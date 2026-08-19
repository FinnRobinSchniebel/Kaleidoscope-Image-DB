package imageset

import (
	"fmt"
	"time"
)

// ApplySourceMetadataUpdate updates source i's title, description and tags from
// newSrc and saves the set. The set's own Title only changes if it still matched
// the source's old title, so a user's custom title is never overwritten. Callers
// must confirm the source's images are unchanged before calling this; images are
// never touched here.
func ApplySourceMetadataUpdate(a *ImageSetMongo, i int, newSrc SourceInfo, checkedAt time.Time, userId string) error {
	old := a.Sources[i]

	if newSrc.Title != old.Title {
		if a.Title == old.Title {
			a.Title = newSrc.Title
		}
		a.Sources[i].Title = newSrc.Title
	}

	if newSrc.Description != old.Description {
		UpdateSourceDescription(a, i, newSrc.Description)
	}

	if err := Tagger.ProcessSourceTags(userId, a, i, newSrc.Tags); err != nil {
		return fmt.Errorf("processing source tags: %w", err)
	}

	a.Sources[i].Date = newSrc.Date
	a.Sources[i].LastChecked = checkedAt
	a.Sources[i].LastImageUpdate = newSrc.Date
	a.Sources[i].PendingImageChange = false
	a.Sources[i].SourceMissing = false

	return UpdateImageSet(a)
}

// MarkSourcePendingImageChange records that source i's images no longer match
// what's stored, without writing any image or metadata change. sourceDate is the
// source's own Date as of this check; storing it lets a later sync tell whether
// the source has moved on again since this still-unresolved change was detected.
func MarkSourcePendingImageChange(a *ImageSetMongo, i int, sourceDate, checkedAt time.Time) error {
	a.Sources[i].PendingImageChange = true
	a.Sources[i].SourceMissing = false
	a.Sources[i].LastImageUpdate = sourceDate
	a.Sources[i].Date = sourceDate
	a.Sources[i].LastChecked = checkedAt
	return UpdateImageSet(a)
}

// MarkSourceMissing records that source i could no longer be fetched. Any prior
// PendingImageChange is cleared along with it: there's no source left to update
// the images from, so an unresolved change can no longer be completed.
func MarkSourceMissing(a *ImageSetMongo, i int, checkedAt time.Time) error {
	a.Sources[i].SourceMissing = true
	a.Sources[i].PendingImageChange = false
	a.Sources[i].LastChecked = checkedAt
	return UpdateImageSet(a)
}
