package imageset

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ErrSourceNotStored is returned by the source writers below when none of the
// user's sets holds the source.
var ErrSourceNotStored = errors.New("source not stored")

// findStoredSource re-reads the set holding the source and finds its index.
// The caller holds LockUserSets, so the set stays current until its write.
func findStoredSource(userID, name, sourceID string) (*ImageSetMongo, int, error) {
	set, ok, err := GetImageSetBySourceID(userID, name, sourceID)
	if err != nil {
		return nil, -1, err
	}
	if ok {
		key := SourceInfo{Name: name, SourceID: sourceID}
		for index, src := range set.Sources {
			if SameSource(src, key) {
				return set, index, nil
			}
		}
	}
	return nil, -1, fmt.Errorf("%w: %s/%s", ErrSourceNotStored, name, sourceID)
}

// ApplySourceMetadataUpdate updates the stored source matching newSrc's Name and
// SourceID with its metadata and tags, holding LockUserSets (see
// applySourceMetadata for which fields and when). Only metadata is read from newSrc; the stored source keeps its ID and
// attribution. Callers must confirm the source's images are unchanged first;
// images are never touched here. changed reports whether data from the source
// changed, which also stamps LastAppliedUpdate.
func ApplySourceMetadataUpdate(userID string, newSrc SourceInfo, checkedAt time.Time) (setID bson.ObjectID, changed bool, err error) {
	defer LockUserSets(userID)()
	set, index, err := findStoredSource(userID, newSrc.Name, newSrc.SourceID)
	if err != nil {
		return bson.NilObjectID, false, err
	}

	before := set.Sources[index]
	// ProcessSourceTags edits tags in place, so the copy needs its own slice.
	before.Tags = slices.Clone(before.Tags)
	fields := applySourceMetadata(set, index, newSrc)

	// Must precede ProcessSourceTags: its system-tag recompute reads SourceMissing.
	set.Sources[index].LastChecked = checkedAt
	set.Sources[index].ImageChangeState = ImageChangeNone
	set.Sources[index].SourceMissing = false

	counted, err := Tagger.ProcessSourceTags(userID, set, index, newSrc.Tags)
	if err != nil {
		return set.ID, false, fmt.Errorf("processing source tags: %w", err)
	}

	changed = sourceDataChanged(before, set.Sources[index])
	if changed {
		set.Sources[index].LastAppliedUpdate = checkedAt
	}

	fields[fmt.Sprintf("sources.%d", index)] = set.Sources[index]
	maps.Copy(fields, tagFields(set))

	if err := updateSourceFields(set, index, fields); err != nil {
		return set.ID, false, errors.Join(err, Tagger.UndoTagCounts(userID, counted))
	}
	return set.ID, changed, nil
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

// MarkSourcePendingImageChange records that the stored source's images no longer
// match what's stored, holding LockUserSets and writing no image or metadata
// change. sourceDate is
// the source's own Date as of this check; storing it lets a later sync tell whether
// the source has moved on again since this still-unresolved change was detected.
func MarkSourcePendingImageChange(userID, name, sourceID string, sourceDate, checkedAt time.Time) (setID bson.ObjectID, err error) {
	defer LockUserSets(userID)()
	set, index, err := findStoredSource(userID, name, sourceID)
	if err != nil {
		return bson.NilObjectID, err
	}

	wasMissing := set.Sources[index].SourceMissing
	set.Sources[index].ImageChangeState = ImageChangePending
	set.Sources[index].SourceMissing = false
	set.Sources[index].Date = sourceDate
	set.Sources[index].LastChecked = checkedAt

	p := fmt.Sprintf("sources.%d.", index)
	fields := bson.M{
		p + "image_change_state": ImageChangePending,
		p + "source_missing":     false,
		p + "date":               sourceDate,
		p + "last_checked":       checkedAt,
	}

	var counted TagCountChange
	if wasMissing {
		if counted, err = Tagger.RecomputeSystemTags(userID, set); err != nil {
			return set.ID, fmt.Errorf("computing system tags: %w", err)
		}
		maps.Copy(fields, tagFields(set))
	}

	if err := updateSourceFields(set, index, fields); err != nil {
		return set.ID, errors.Join(err, Tagger.UndoTagCounts(userID, counted))
	}
	return set.ID, nil
}

// MarkSourceMissing records, holding LockUserSets, that the stored source could no
// longer be fetched. Any
// prior image-change state is reset to none along with it: there's no source left
// to update the images from, so an unresolved change can no longer be completed.
// newlyMissing reports whether this call flipped SourceMissing.
func MarkSourceMissing(userID, name, sourceID string, checkedAt time.Time) (setID bson.ObjectID, newlyMissing bool, err error) {
	defer LockUserSets(userID)()
	set, index, err := findStoredSource(userID, name, sourceID)
	if err != nil {
		return bson.NilObjectID, false, err
	}

	wasMissing := set.Sources[index].SourceMissing
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
		if counted, err = Tagger.RecomputeSystemTags(userID, set); err != nil {
			return set.ID, false, fmt.Errorf("computing system tags: %w", err)
		}
		maps.Copy(fields, tagFields(set))
	}

	if err := updateSourceFields(set, index, fields); err != nil {
		return set.ID, false, errors.Join(err, Tagger.UndoTagCounts(userID, counted))
	}
	return set.ID, !wasMissing, nil
}

// MarkSourceRecovered records, holding LockUserSets, that the stored source is
// reachable again with nothing else to report. Metadata or image changes go through
// ApplySourceMetadataUpdate / MarkSourcePendingImageChange instead, which clear
// SourceMissing themselves.
func MarkSourceRecovered(userID, name, sourceID string, checkedAt time.Time) (setID bson.ObjectID, err error) {
	defer LockUserSets(userID)()
	set, index, err := findStoredSource(userID, name, sourceID)
	if err != nil {
		return bson.NilObjectID, err
	}

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
		if counted, err = Tagger.RecomputeSystemTags(userID, set); err != nil {
			return set.ID, fmt.Errorf("computing system tags: %w", err)
		}
		maps.Copy(fields, tagFields(set))
	}

	if err := updateSourceFields(set, index, fields); err != nil {
		return set.ID, errors.Join(err, Tagger.UndoTagCounts(userID, counted))
	}
	return set.ID, nil
}
