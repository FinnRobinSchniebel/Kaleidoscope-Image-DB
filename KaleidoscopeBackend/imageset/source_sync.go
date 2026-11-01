package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ErrSourceNotStored is returned by the source writers below when none of the
// user's sets holds the source.
var ErrSourceNotStored = errors.New("source not stored")

// SourceWriteResult is one set's outcome from a by-source writer, whose own
// error covers the whole call; each writer says what Changed means for it.
type SourceWriteResult struct {
	SetID   bson.ObjectID
	Changed bool
	Held    ImageChangeState //the copy's state when a hold kept the write from applying; none otherwise
	Err     error
}

// writeEachSetHoldingSource holds the user's set lock (lockUserSetsOrPause),
// re-reads every set holding the source and calls write on each in turn with
// the source's index, carrying on after a set whose write failed.
func writeEachSetHoldingSource(ctx context.Context, userID, name, sourceID string, write func(set *ImageSetMongo, index int) SourceWriteResult) ([]SourceWriteResult, error) {

	unlock, err := lockUserSetsOrPause(ctx, userID)
	if err != nil {
		return nil, err
	}
	defer unlock()

	found, err := GetImageSetsBySourceIDs(userID, name, []string{sourceID})
	if err != nil {
		return nil, err
	}

	sets := found[sourceID]
	if len(sets) == 0 {
		return nil, fmt.Errorf("%w: %s/%s", ErrSourceNotStored, name, sourceID)
	}

	key := SourceInfo{Name: name, SourceID: sourceID}
	results := make([]SourceWriteResult, 0, len(sets))
	for _, set := range sets {

		index := slices.IndexFunc(set.Sources, func(src SourceInfo) bool { return SameSource(src, key) })
		results = append(results, write(set, index))
	}
	return results, nil
}

// stampHeldSourceInSet is a sync update's whole write to a copy whose
// image-change state holds it (pending or declined): only LastChecked is set,
// and a zero one is kept, since it marks a source the next sync must catch up.
// The caller holds the user's set lock.
func stampHeldSourceInSet(set *ImageSetMongo, index int, checkedAt time.Time) SourceWriteResult {

	src := &set.Sources[index]
	result := SourceWriteResult{SetID: set.ID, Held: src.ImageChangeState}
	if src.LastChecked.IsZero() {
		return result
	}

	src.LastChecked = checkedAt
	fields := bson.M{fmt.Sprintf("sources.%d.last_checked", index): checkedAt}
	result.Err = updateSourceFields(set, index, result.Held, fields)
	return result
}

// ApplySourceMetadataUpdate updates every stored copy of the source matching
// newSrc's Name and SourceID with its metadata and tags (see
// applySourceMetadata for which fields and when), through
// writeEachSetHoldingSource. Only metadata is read from newSrc; each copy keeps
// its ID and attribution. Callers must confirm the source's images are
// unchanged first; images are never touched here. A result's Changed reports
// whether data from the source changed in that set, which also stamps its
// LastAppliedUpdate. A copy held by its image-change state is only stamped
// (stampHeldSourceInSet), with Held set.
func ApplySourceMetadataUpdate(ctx context.Context, userID string, newSrc SourceInfo, checkedAt time.Time) ([]SourceWriteResult, error) {
	return writeEachSetHoldingSource(ctx, userID, newSrc.Name, newSrc.SourceID, func(set *ImageSetMongo, index int) SourceWriteResult {
		return applySourceMetadataUpdateInSet(set, index, newSrc, checkedAt)
	})
}

// applySourceMetadataUpdateInSet is ApplySourceMetadataUpdate for the copy at
// set.Sources[index]. The caller holds the user's set lock.
func applySourceMetadataUpdateInSet(set *ImageSetMongo, index int, newSrc SourceInfo, checkedAt time.Time) SourceWriteResult {

	if set.Sources[index].ImageChangeState != ImageChangeNone {
		return stampHeldSourceInSet(set, index, checkedAt)
	}

	userID := set.KscopeUserId
	before := set.Sources[index]
	// ProcessSourceTags edits tags in place, so the copy needs its own slice.
	before.Tags = slices.Clone(before.Tags)
	fields := applySourceMetadata(set, index, newSrc)

	// Must precede ProcessSourceTags: its system-tag recompute reads SourceMissing.
	set.Sources[index].LastChecked = checkedAt
	set.Sources[index].SourceMissing = false

	counted, err := Tagger.ProcessSourceTags(userID, set, index, newSrc.Tags)
	if err != nil {
		return SourceWriteResult{SetID: set.ID, Err: fmt.Errorf("processing source tags: %w", err)}
	}

	changed := sourceDataChanged(before, set.Sources[index])
	if changed {
		set.Sources[index].LastAppliedUpdate = checkedAt
	}

	fields[fmt.Sprintf("sources.%d", index)] = set.Sources[index]
	maps.Copy(fields, tagFields(set))

	if err := updateSourceFields(set, index, ImageChangeNone, fields); err != nil {
		return SourceWriteResult{SetID: set.ID, Err: errors.Join(err, Tagger.UndoTagCounts(userID, counted))}
	}
	return SourceWriteResult{SetID: set.ID, Changed: changed}
}

// applySourceMetadata copies newSrc's non-empty title, description, known author
// and author id onto set.Sources[index]; an empty value never overwrites. The date
// follows mergeSourceDate. The set's Title follows only while it still equals the
// source's old title, Authors are re-derived from the sources, and the set's
// Description is never touched. Returns the set-level fields it changed.
func applySourceMetadata(set *ImageSetMongo, index int, newSrc SourceInfo) bson.M {
	old := set.Sources[index]
	src := &set.Sources[index]
	fields := bson.M{}

	if newSrc.Title != "" && newSrc.Title != old.Title {
		if set.Title == old.Title {
			set.Title = newSrc.Title
			fields["title"] = set.Title
		}
		src.Title = newSrc.Title
	}
	if newSrc.Description != "" && newSrc.Description != old.Description {
		UpdateSourceDescription(set, index, newSrc.Description)
	}
	if authorKnown(newSrc.SourceAuthor) {
		src.SourceAuthor = newSrc.SourceAuthor
	}
	if deriveAuthors(set) {
		fields["authors"] = set.Authors
	}
	if newSrc.AuthorID != "" {
		src.AuthorID = newSrc.AuthorID
	}
	src.Date = mergeSourceDate(old.Date, newSrc.Date)
	return fields
}

// sourceDataChanged reports whether after differs from before in a field the
// source supplies. Bookkeeping such as LastChecked and SourceMissing is ignored.
func sourceDataChanged(before, after SourceInfo) bool {
	return before.Title != after.Title ||
		before.Description != after.Description ||
		before.SourceAuthor != after.SourceAuthor ||
		before.AuthorID != after.AuthorID ||
		!before.Date.Equal(after.Date) ||
		!slices.Equal(before.Tags, after.Tags)
}

// MarkSourcePendingImageChange records, in every set holding the source (through
// writeEachSetHoldingSource), that the source's images no longer match what's
// stored, writing no image or metadata change. Date is left alone, so a later
// sync still sees the source's move. A copy already pending or declined is only
// stamped (stampHeldSourceInSet), with Held set. A result's Changed reports
// whether that set's copy flipped to pending.
func MarkSourcePendingImageChange(ctx context.Context, userID, name, sourceID string, checkedAt time.Time) ([]SourceWriteResult, error) {
	return writeEachSetHoldingSource(ctx, userID, name, sourceID, func(set *ImageSetMongo, index int) SourceWriteResult {
		return markSourcePendingImageChangeInSet(set, index, checkedAt)
	})
}

// markSourcePendingImageChangeInSet is MarkSourcePendingImageChange for the copy
// at set.Sources[index]. The pending-update notification is published only once
// the flip's write has landed; a failed publish is only logged, since
// PublishMissingPendingUpdates sends it at the next startup. The caller holds
// the user's set lock.
func markSourcePendingImageChangeInSet(set *ImageSetMongo, index int, checkedAt time.Time) SourceWriteResult {

	if set.Sources[index].ImageChangeState != ImageChangeNone {
		return stampHeldSourceInSet(set, index, checkedAt)
	}

	userID := set.KscopeUserId
	wasMissing := set.Sources[index].SourceMissing
	set.Sources[index].ImageChangeState = ImageChangePending
	set.Sources[index].SourceMissing = false
	set.Sources[index].LastChecked = checkedAt

	p := fmt.Sprintf("sources.%d.", index)
	fields := bson.M{
		p + "image_change_state": ImageChangePending,
		p + "source_missing":     false,
		p + "last_checked":       checkedAt,
	}

	var counted TagCountChange
	if wasMissing {
		var err error
		if counted, err = Tagger.RecomputeSystemTags(userID, set); err != nil {
			return SourceWriteResult{SetID: set.ID, Err: fmt.Errorf("computing system tags: %w", err)}
		}
		maps.Copy(fields, tagFields(set))
	}

	if err := updateSourceFields(set, index, ImageChangeNone, fields); err != nil {
		return SourceWriteResult{SetID: set.ID, Err: errors.Join(err, Tagger.UndoTagCounts(userID, counted))}
	}

	src := set.Sources[index]
	pending := notification.PendingImageUpdate{SetID: set.ID, Service: src.Name, SourceID: src.SourceID}
	if err := notification.Publish(userID, pending); err != nil {
		log.Printf("------ Warning: %s/%s in set %s is pending, but its notification wasn't published (sent at the next startup): %s ------", src.Name, src.SourceID, set.ID.Hex(), err)
	}
	return SourceWriteResult{SetID: set.ID, Changed: true}
}

// MarkSourceMissing records, in every set holding the source (through
// writeEachSetHoldingSource), that the source could no longer be fetched. Any
// prior image-change state is reset to none along with it: there's no source
// left to update the images from, so an unresolved change can no longer be
// completed. A result's Changed reports whether this call flipped that set's
// SourceMissing.
func MarkSourceMissing(ctx context.Context, userID, name, sourceID string, checkedAt time.Time) ([]SourceWriteResult, error) {
	return writeEachSetHoldingSource(ctx, userID, name, sourceID, func(set *ImageSetMongo, index int) SourceWriteResult {
		return markSourceMissingInSet(set, index, checkedAt)
	})
}

// markSourceMissingInSet is MarkSourceMissing for the copy at
// set.Sources[index]. The caller holds the user's set lock.
func markSourceMissingInSet(set *ImageSetMongo, index int, checkedAt time.Time) SourceWriteResult {

	userID := set.KscopeUserId
	wasMissing := set.Sources[index].SourceMissing
	stateRead := set.Sources[index].ImageChangeState
	set.Sources[index].SourceMissing = true
	set.Sources[index].ImageChangeState = ImageChangeNone
	set.Sources[index].LastChecked = checkedAt

	p := fmt.Sprintf("sources.%d.", index)
	fields := bson.M{
		p + "source_missing":     true,
		p + "image_change_state": ImageChangeNone,
		p + "last_checked":       checkedAt,
	}

	var counted TagCountChange
	if !wasMissing {
		var err error
		if counted, err = Tagger.RecomputeSystemTags(userID, set); err != nil {
			return SourceWriteResult{SetID: set.ID, Err: fmt.Errorf("computing system tags: %w", err)}
		}
		maps.Copy(fields, tagFields(set))
	}

	if err := updateSourceFields(set, index, stateRead, fields); err != nil {
		return SourceWriteResult{SetID: set.ID, Err: errors.Join(err, Tagger.UndoTagCounts(userID, counted))}
	}
	return SourceWriteResult{SetID: set.ID, Changed: !wasMissing}
}

// MarkSourceRecovered records, in every set holding the source (through
// writeEachSetHoldingSource), that the source is reachable again with nothing
// else to report. Metadata or image changes go through ApplySourceMetadataUpdate /
// MarkSourcePendingImageChange instead, which clear SourceMissing themselves. A
// result's Changed reports whether that set's copy was missing.
func MarkSourceRecovered(ctx context.Context, userID, name, sourceID string, checkedAt time.Time) ([]SourceWriteResult, error) {
	return writeEachSetHoldingSource(ctx, userID, name, sourceID, func(set *ImageSetMongo, index int) SourceWriteResult {
		return markSourceRecoveredInSet(set, index, checkedAt)
	})
}

// markSourceRecoveredInSet is MarkSourceRecovered for the copy at
// set.Sources[index]. The caller holds the user's set lock.
func markSourceRecoveredInSet(set *ImageSetMongo, index int, checkedAt time.Time) SourceWriteResult {

	userID := set.KscopeUserId
	wasMissing := set.Sources[index].SourceMissing
	set.Sources[index].SourceMissing = false
	set.Sources[index].LastChecked = checkedAt

	p := fmt.Sprintf("sources.%d.", index)
	fields := bson.M{
		p + "source_missing": false,
		p + "last_checked":   checkedAt,
	}

	var counted TagCountChange
	if wasMissing {
		var err error
		if counted, err = Tagger.RecomputeSystemTags(userID, set); err != nil {
			return SourceWriteResult{SetID: set.ID, Err: fmt.Errorf("computing system tags: %w", err)}
		}
		maps.Copy(fields, tagFields(set))
	}

	if err := updateSourceFields(set, index, set.Sources[index].ImageChangeState, fields); err != nil {
		return SourceWriteResult{SetID: set.ID, Err: errors.Join(err, Tagger.UndoTagCounts(userID, counted))}
	}
	return SourceWriteResult{SetID: set.ID, Changed: wasMissing}
}

// MarkSourcesChecked sets LastChecked to checkedAt on every stored copy of the
// sources named name with sourceIDs, holding the user's set lock
// (lockUserSetsOrPause), for sources a sync pass looked at and had nothing to
// write for. A copy whose LastChecked is still zero keeps it.
func MarkSourcesChecked(ctx context.Context, userID, name string, sourceIDs []string, checkedAt time.Time) error {

	if len(sourceIDs) == 0 {
		return nil
	}

	unlock, err := lockUserSetsOrPause(ctx, userID)
	if err != nil {
		return err
	}
	defer unlock()
	return bumpSourcesLastChecked(userID, name, sourceIDs, checkedAt)
}
