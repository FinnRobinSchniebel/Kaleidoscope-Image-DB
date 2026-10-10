package imageset

import (
	"fmt"
	"maps"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ApplySourceMetadataUpdate updates Sources[index]'s metadata and tags from newSrc
// and saves them (see applySourceMetadata for which fields and when). Callers must
// confirm the source's images are unchanged first; images are never touched here.
// changed reports whether data from the source changed, which also stamps
// LastAppliedUpdate.
func ApplySourceMetadataUpdate(ISet *ImageSetMongo, index int, newSrc SourceInfo, checkedAt time.Time, userId string) (changed bool, err error) {
	before := ISet.Sources[index]
	// ProcessSourceTags edits tags in place, so the copy needs its own slice.
	before.Tags = slices.Clone(before.Tags)
	fields := applySourceMetadata(ISet, index, newSrc)

	// Must precede ProcessSourceTags: its system-tag recompute reads SourceMissing.
	ISet.Sources[index].LastChecked = checkedAt
	ISet.Sources[index].ImageChangeState = ImageChangeNone
	ISet.Sources[index].SourceMissing = false

	if err := Tagger.ProcessSourceTags(userId, ISet, index, newSrc.Tags); err != nil {
		return false, fmt.Errorf("processing source tags: %w", err)
	}
	changed = sourceDataChanged(before, ISet.Sources[index])
	if changed {
		ISet.Sources[index].LastAppliedUpdate = checkedAt
	}

	fields[fmt.Sprintf("sources.%d", index)] = ISet.Sources[index]
	maps.Copy(fields, tagFields(ISet))
	return changed, updateSourceFields(ISet, index, fields)
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

// MarkSourcePendingImageChange records that Sources[index]'s images no longer match
// what's stored, without writing any image or metadata change. sourceDate is the
// source's own Date as of this check; storing it lets a later sync tell whether
// the source has moved on again since this still-unresolved change was detected.
func MarkSourcePendingImageChange(ISet *ImageSetMongo, index int, sourceDate, checkedAt time.Time) error {
	wasMissing := ISet.Sources[index].SourceMissing
	ISet.Sources[index].ImageChangeState = ImageChangePending
	ISet.Sources[index].SourceMissing = false
	ISet.Sources[index].Date = sourceDate
	ISet.Sources[index].LastChecked = checkedAt

	p := fmt.Sprintf("sources.%d.", index)
	fields := bson.M{
		p + "image_change_state": ImageChangePending,
		p + "source_missing":     false,
		p + "date":               sourceDate,
		p + "last_checked":       checkedAt,
	}
	if wasMissing {
		if err := Tagger.RecomputeSystemTags(ISet.KscopeUserId, ISet); err != nil {
			return fmt.Errorf("computing system tags: %w", err)
		}
		maps.Copy(fields, tagFields(ISet))
	}
	return updateSourceFields(ISet, index, fields)
}

// MarkSourceMissing records that Sources[index] could no longer be fetched. Any prior
// image-change state is reset to none along with it: there's no source left to
// update the images from, so an unresolved change can no longer be completed.
// newlyMissing reports whether this call flipped SourceMissing.
func MarkSourceMissing(ISet *ImageSetMongo, index int, checkedAt time.Time) (newlyMissing bool, err error) {
	wasMissing := ISet.Sources[index].SourceMissing
	ISet.Sources[index].SourceMissing = true
	ISet.Sources[index].ImageChangeState = ImageChangeNone
	ISet.Sources[index].LastChecked = checkedAt

	p := fmt.Sprintf("sources.%d.", index)
	fields := bson.M{
		p + "source_missing":     true,
		p + "image_change_state": ImageChangeNone,
		p + "last_checked":       checkedAt,
	}
	if !wasMissing {
		if err := Tagger.RecomputeSystemTags(ISet.KscopeUserId, ISet); err != nil {
			return false, fmt.Errorf("computing system tags: %w", err)
		}
		maps.Copy(fields, tagFields(ISet))
	}
	return !wasMissing, updateSourceFields(ISet, index, fields)
}

// MarkSourceRecovered records that Sources[index] is reachable again with nothing
// else to report. Metadata or image changes go through ApplySourceMetadataUpdate /
// MarkSourcePendingImageChange instead, which clear SourceMissing themselves.
func MarkSourceRecovered(ISet *ImageSetMongo, index int, checkedAt time.Time) error {
	wasMissing := ISet.Sources[index].SourceMissing
	ISet.Sources[index].SourceMissing = false
	ISet.Sources[index].LastChecked = checkedAt

	p := fmt.Sprintf("sources.%d.", index)
	fields := bson.M{
		p + "source_missing": false,
		p + "last_checked":   checkedAt,
	}
	if wasMissing {
		if err := Tagger.RecomputeSystemTags(ISet.KscopeUserId, ISet); err != nil {
			return fmt.Errorf("computing system tags: %w", err)
		}
		maps.Copy(fields, tagFields(ISet))
	}
	return updateSourceFields(ISet, index, fields)
}
