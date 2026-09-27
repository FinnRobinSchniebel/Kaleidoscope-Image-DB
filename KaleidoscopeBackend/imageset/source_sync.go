package imageset

import (
	"fmt"
	"maps"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ApplySourceMetadataUpdate updates Sources[index]'s title, description and tags from
// newSrc and saves them. The set's own Title only follows a non-empty new source title,
// and only if it still matched the old one, so a user's custom title is never
// overwritten. Callers must confirm the source's images are unchanged first; images
// are never touched here.
func ApplySourceMetadataUpdate(ISet *ImageSetMongo, index int, newSrc SourceInfo, checkedAt time.Time, userId string) error {
	old := ISet.Sources[index]
	fields := bson.M{}

	if newSrc.Title != old.Title {
		if ISet.Title == old.Title && newSrc.Title != "" {
			ISet.Title = newSrc.Title
			fields["title"] = ISet.Title
		}
		ISet.Sources[index].Title = newSrc.Title
	}

	if newSrc.Description != old.Description {
		UpdateSourceDescription(ISet, index, newSrc.Description)
	}

	// Must precede ProcessSourceTags: its system-tag recompute reads SourceMissing.
	ISet.Sources[index].Date = newSrc.Date
	ISet.Sources[index].LastChecked = checkedAt
	ISet.Sources[index].LastImageUpdate = newSrc.Date
	ISet.Sources[index].PendingImageChange = false
	ISet.Sources[index].SourceMissing = false

	if err := Tagger.ProcessSourceTags(userId, ISet, index, newSrc.Tags); err != nil {
		return fmt.Errorf("processing source tags: %w", err)
	}

	fields[fmt.Sprintf("sources.%d", index)] = ISet.Sources[index]
	maps.Copy(fields, tagFields(ISet))
	return updateSourceFields(ISet, index, fields)
}

// MarkSourcePendingImageChange records that Sources[index]'s images no longer match
// what's stored, without writing any image or metadata change. sourceDate is the
// source's own Date as of this check; storing it lets a later sync tell whether
// the source has moved on again since this still-unresolved change was detected.
func MarkSourcePendingImageChange(ISet *ImageSetMongo, index int, sourceDate, checkedAt time.Time) error {
	wasMissing := ISet.Sources[index].SourceMissing
	ISet.Sources[index].PendingImageChange = true
	ISet.Sources[index].SourceMissing = false
	ISet.Sources[index].LastImageUpdate = sourceDate
	ISet.Sources[index].Date = sourceDate
	ISet.Sources[index].LastChecked = checkedAt

	p := fmt.Sprintf("sources.%d.", index)
	fields := bson.M{
		p + "pending_image_change": true,
		p + "source_missing":       false,
		p + "last_image_update":    sourceDate,
		p + "date":                 sourceDate,
		p + "last_checked":         checkedAt,
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
// PendingImageChange is cleared along with it: there's no source left to update
// the images from, so an unresolved change can no longer be completed.
func MarkSourceMissing(ISet *ImageSetMongo, index int, checkedAt time.Time) error {
	wasMissing := ISet.Sources[index].SourceMissing
	ISet.Sources[index].SourceMissing = true
	ISet.Sources[index].PendingImageChange = false
	ISet.Sources[index].LastChecked = checkedAt

	p := fmt.Sprintf("sources.%d.", index)
	fields := bson.M{
		p + "source_missing":       true,
		p + "pending_image_change": false,
		p + "last_checked":         checkedAt,
	}
	if !wasMissing {
		if err := Tagger.RecomputeSystemTags(ISet.KscopeUserId, ISet); err != nil {
			return fmt.Errorf("computing system tags: %w", err)
		}
		maps.Copy(fields, tagFields(ISet))
	}
	return updateSourceFields(ISet, index, fields)
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
