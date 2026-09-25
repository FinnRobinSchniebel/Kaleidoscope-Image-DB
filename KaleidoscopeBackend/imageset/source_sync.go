package imageset

import (
	"fmt"
	"time"
)

// ApplySourceMetadataUpdate updates Sources[index]'s title, description and tags from
// newSrc and saves the set. The set's own Title only changes if it still matched
// the source's old title, so a user's custom title is never overwritten. Callers
// must confirm the source's images are unchanged before calling this; images are
// never touched here.
func ApplySourceMetadataUpdate(ISet *ImageSetMongo, index int, newSrc SourceInfo, checkedAt time.Time, userId string) error {
	old := ISet.Sources[index]

	if newSrc.Title != old.Title {
		if ISet.Title == old.Title {
			ISet.Title = newSrc.Title
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

	return UpdateImageSet(ISet)
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
	if wasMissing {
		if err := Tagger.RecomputeSystemTags(ISet.KscopeUserId, ISet); err != nil {
			return fmt.Errorf("computing system tags: %w", err)
		}
	}
	return UpdateImageSet(ISet)
}

// MarkSourceMissing records that Sources[index] could no longer be fetched. Any prior
// PendingImageChange is cleared along with it: there's no source left to update
// the images from, so an unresolved change can no longer be completed.
// See MarkSourceRecovered for the opposite transition.
func MarkSourceMissing(ISet *ImageSetMongo, index int, checkedAt time.Time) error {
	wasMissing := ISet.Sources[index].SourceMissing
	ISet.Sources[index].SourceMissing = true
	ISet.Sources[index].PendingImageChange = false
	ISet.Sources[index].LastChecked = checkedAt
	if !wasMissing {
		if err := Tagger.RecomputeSystemTags(ISet.KscopeUserId, ISet); err != nil {
			return fmt.Errorf("computing system tags: %w", err)
		}
	}
	return UpdateImageSet(ISet)
}

// MarkSourceRecovered records that Sources[index] is reachable again with nothing
// else to report - no metadata change, no image change (those cases go
// through ApplySourceMetadataUpdate / MarkSourcePendingImageChange instead,
// which already clear SourceMissing themselves).
func MarkSourceRecovered(ISet *ImageSetMongo, index int, checkedAt time.Time) error {
	wasMissing := ISet.Sources[index].SourceMissing
	ISet.Sources[index].SourceMissing = false
	ISet.Sources[index].LastChecked = checkedAt
	if wasMissing {
		if err := Tagger.RecomputeSystemTags(ISet.KscopeUserId, ISet); err != nil {
			return fmt.Errorf("computing system tags: %w", err)
		}
	}
	return UpdateImageSet(ISet)
}
