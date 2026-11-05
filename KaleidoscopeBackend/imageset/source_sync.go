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

// ErrSetChangedDuringSync is a set's result error when its copy needs a page the
// caller didn't fetch, as for a set that changed after the pre-lock check read
// it. Nothing is written, so the next sync checks it again.
var ErrSetChangedDuringSync = errors.New("the set changed during the sync")

// SourceWriteKind is the per-set step a SourceWriteResult comes from; for a
// failed result, the write it attempted.
type SourceWriteKind string

const (
	SourceHeldStamp      SourceWriteKind = "held stamp"
	SourceMetadataUpdate SourceWriteKind = "metadata update"
	SourceReorder        SourceWriteKind = "reorder"
	SourceRemoval        SourceWriteKind = "removal"
	SourceAttribution    SourceWriteKind = "attribution"
	SourcePendingFlip    SourceWriteKind = "pending flip"
	SourceMissingMark    SourceWriteKind = "missing mark"
	SourceImageCheck     SourceWriteKind = "image check" //a check that failed or couldn't decide, so nothing was written
	SourceReread         SourceWriteKind = "re-read"     //a failed re-read of the set before its write
)

// SourceWriteResult is one set's outcome from a by-source writer, whose own
// error covers the whole call; each writer says what Changed means for it.
type SourceWriteResult struct {
	SetID   bson.ObjectID
	Kind    SourceWriteKind
	Changed bool
	Held    ImageChangeState //the copy's state when a hold kept the write from applying; none otherwise
	Err     error
}

// writeEachSetHoldingSource holds the user's set lock (lockUserSetsOrPause) and
// calls write on every set holding the source, oldest first, with the source's
// index, carrying on after a set whose write failed. Each set is re-read just
// before its turn: one that's gone or no longer holds the source is skipped, and
// a failed re-read is that set's SourceReread result.
func writeEachSetHoldingSource(ctx context.Context, userID, name, sourceID string, write func(set *ImageSetMongo, index int) SourceWriteResult) ([]SourceWriteResult, error) {

	unlock, err := lockUserSetsOrPause(ctx, userID)
	if err != nil {
		return nil, err
	}
	defer unlock()

	ids, err := findSetIDsWithSource(userID, name, sourceID)
	if err != nil {
		return nil, err
	}

	if len(ids) == 0 {
		return nil, fmt.Errorf("%w: %s/%s", ErrSourceNotStored, name, sourceID)
	}

	key := SourceInfo{Name: name, SourceID: sourceID}
	results := make([]SourceWriteResult, 0, len(ids))
	for _, id := range ids {

		sets, err := loadOwnedSets(userID, []bson.ObjectID{id})
		if err != nil {
			results = append(results, SourceWriteResult{SetID: id, Kind: SourceReread, Err: err})
			continue
		}

		if len(sets) == 0 {
			continue
		}

		index := slices.IndexFunc(sets[0].Sources, func(src SourceInfo) bool { return SameSource(src, key) })
		if index < 0 {
			continue
		}
		results = append(results, write(&sets[0], index))
	}
	return results, nil
}

// stampHeldSourceInSet is a sync update's whole write to a copy whose
// image-change state holds it (pending or declined): only LastChecked is set,
// and a zero one is kept, since it marks a source the next sync must catch up.
// The caller holds the user's set lock.
func stampHeldSourceInSet(set *ImageSetMongo, index int, checkedAt time.Time) SourceWriteResult {

	src := &set.Sources[index]
	result := SourceWriteResult{SetID: set.ID, Kind: SourceHeldStamp, Held: src.ImageChangeState}
	if src.LastChecked.IsZero() {
		return result
	}

	src.LastChecked = checkedAt
	fields := bson.M{fmt.Sprintf("sources.%d.last_checked", index): checkedAt}
	result.Err = updateSourceFields(set, index, result.Held, fields)
	return result
}

// ApplySourceUpdate writes the service's version of a source, newSrc, to every
// set holding it (writeEachSetHoldingSource), each set getting the outcome its
// own copy calls for (applySourceUpdateInSet). Only metadata is read from
// newSrc. pages are the previews and originals the sync fetched; with nil, a
// copy that needs the image check is skipped with ErrSetChangedDuringSync.
func ApplySourceUpdate(ctx context.Context, userID string, newSrc SourceInfo, pages *FetchedPages, checkedAt time.Time) ([]SourceWriteResult, error) {
	return writeEachSetHoldingSource(ctx, userID, newSrc.Name, newSrc.SourceID, func(set *ImageSetMongo, index int) SourceWriteResult {
		return applySourceUpdateInSet(set, index, newSrc, pages, checkedAt)
	})
}

// applySourceUpdateInSet gives the copy at set.Sources[index] its outcome. A
// copy that needs no image check (NeedsImageCheck) gets the metadata update,
// which only stamps a held one; any other is checked (checkSourceImages) and
// then flipped to pending, or has its attribution rewritten along with the
// metadata. A rewrite the set's attribution check rejects writes nothing. The
// caller holds the user's set lock.
func applySourceUpdateInSet(set *ImageSetMongo, index int, newSrc SourceInfo, pages *FetchedPages, checkedAt time.Time) SourceWriteResult {

	if !NeedsImageCheck(set.Sources[index], newSrc.Date) {
		return applySourceMetadataUpdateInSet(set, index, newSrc, checkedAt)
	}

	if pages == nil {
		return SourceWriteResult{SetID: set.ID, Kind: SourceImageCheck, Err: ErrSetChangedDuringSync}
	}

	check, err := checkSourceImages(set, index, *pages)
	switch {
	case err != nil:
		return SourceWriteResult{SetID: set.ID, Kind: SourceImageCheck, Err: err}
	case len(check.needsOriginals) > 0:
		return SourceWriteResult{SetID: set.ID, Kind: SourceImageCheck, Err: ErrSetChangedDuringSync}
	case check.kind == SourcePendingFlip:
		return markSourcePendingImageChangeInSet(set, index, checkedAt)
	}

	set.Sources[index].AttributedTo = check.attribution
	if err := set.checkAttribution(); err != nil {
		return SourceWriteResult{SetID: set.ID, Kind: check.kind, Err: err}
	}

	result := applySourceMetadataUpdateInSet(set, index, newSrc, checkedAt)
	result.Kind, result.Changed = check.kind, result.Err == nil
	return result
}

// applySourceMetadataUpdateInSet updates the copy at set.Sources[index] with
// newSrc's metadata and tags (see applySourceMetadata for which fields and
// when), writing the copy as the caller left it, AttributedTo included. Images
// are never touched, so the caller must have found them unchanged. A copy held
// by its image-change state is only stamped (stampHeldSourceInSet). Changed
// reports whether data from the source changed, which also stamps
// LastAppliedUpdate, or whether the copy was marked missing, which doesn't. The
// caller holds the user's set lock.
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
		return SourceWriteResult{SetID: set.ID, Kind: SourceMetadataUpdate, Err: fmt.Errorf("processing source tags: %w", err)}
	}

	changed := sourceDataChanged(before, set.Sources[index])
	if changed {
		set.Sources[index].LastAppliedUpdate = checkedAt
	}

	fields[fmt.Sprintf("sources.%d", index)] = set.Sources[index]
	maps.Copy(fields, tagFields(set))

	if err := updateSourceFields(set, index, ImageChangeNone, fields); err != nil {
		return SourceWriteResult{SetID: set.ID, Kind: SourceMetadataUpdate, Err: errors.Join(err, Tagger.UndoTagCounts(userID, counted))}
	}
	return SourceWriteResult{SetID: set.ID, Kind: SourceMetadataUpdate, Changed: changed || before.SourceMissing}
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

// markSourcePendingImageChangeInSet records that the source's images no longer
// match the copy at set.Sources[index], writing no image or metadata change:
// Date is left alone, so a later sync still sees the source's move. A copy
// already pending or declined is only stamped (stampHeldSourceInSet). The
// pending-update notification is published only once the flip's write has
// landed; a failed publish is only logged, since PublishMissingPendingUpdates
// sends it at the next startup. The caller holds the user's set lock.
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

	if err := updateSourceFieldsWithTags(set, index, ImageChangeNone, fields, wasMissing); err != nil {
		return SourceWriteResult{SetID: set.ID, Kind: SourcePendingFlip, Err: err}
	}

	pending := newPendingImageUpdate(set, index)
	if err := notification.Publish(userID, pending); err != nil {
		log.Printf("------ Warning: %s/%s in set %s is pending, but its notification wasn't published (sent at the next startup): %s ------", pending.Service, pending.SourceID, set.ID.Hex(), err)
	}
	return SourceWriteResult{SetID: set.ID, Kind: SourcePendingFlip, Changed: true}
}

// newPendingImageUpdate is the pending-update notification for the copy at
// set.Sources[index].
func newPendingImageUpdate(set *ImageSetMongo, index int) notification.PendingImageUpdate {
	src := set.Sources[index]
	return notification.PendingImageUpdate{PendingUpdateKey: notification.PendingUpdateKey{SetID: set.ID, Service: src.Name, SourceID: src.SourceID}}
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

	if err := updateSourceFieldsWithTags(set, index, stateRead, fields, !wasMissing); err != nil {
		return SourceWriteResult{SetID: set.ID, Kind: SourceMissingMark, Err: err}
	}
	return SourceWriteResult{SetID: set.ID, Kind: SourceMissingMark, Changed: !wasMissing}
}

// updateSourceFieldsWithTags writes fields to the copy at set.Sources[index]
// through updateSourceFields, guarded on stateRead. With recomputeTags, the set's
// system tags are recomputed first and written in the same update, and a failed
// write undoes their counts. The caller holds the user's set lock.
func updateSourceFieldsWithTags(set *ImageSetMongo, index int, stateRead ImageChangeState, fields bson.M, recomputeTags bool) error {

	var counted TagCountChange
	if recomputeTags {
		var err error
		if counted, err = Tagger.RecomputeSystemTags(set.KscopeUserId, set); err != nil {
			return fmt.Errorf("computing system tags: %w", err)
		}
		maps.Copy(fields, tagFields(set))
	}

	if err := updateSourceFields(set, index, stateRead, fields); err != nil {
		return errors.Join(err, Tagger.UndoTagCounts(set.KscopeUserId, counted))
	}
	return nil
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
