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
func ApplySourceMetadataUpdate(ISet *ImageSetMongo, index int, newSrc SourceInfo, checkedAt time.Time, userId string) error {
	fields := applySourceMetadata(ISet, index, newSrc)

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

// applySourceMetadata copies newSrc's non-empty title, description, author and
// author id onto set.Sources[index]; an empty value never overwrites. The set's
// own Title and Authors follow only while they still carry the old source value,
// so a user's edits are kept, and the set's Description is never touched.
// Returns the set-level fields it changed.
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
	if newSrc.SourceAuthor != "" && newSrc.SourceAuthor != old.SourceAuthor {
		if followAuthor(set, old.SourceAuthor, newSrc.SourceAuthor) {
			fields["authors"] = set.Authors
		}
		src.SourceAuthor = newSrc.SourceAuthor
	}
	if newSrc.AuthorID != "" {
		src.AuthorID = newSrc.AuthorID
	}
	return fields
}

// followAuthor moves set.Authors from a source's old author name to newName:
// in place while the list still has oldName (just removing oldName if newName
// is already listed), or, when the source had no author, by adding newName
// (replacing a lone unknownAuthor placeholder). A list that no longer has
// oldName was edited and is left alone. Reports whether it changed.
func followAuthor(set *ImageSetMongo, oldName, newName string) bool {
	if slices.Contains(set.Authors, newName) {
		i := slices.Index(set.Authors, oldName)
		if oldName == "" || i < 0 {
			return false
		}
		set.Authors = slices.Delete(set.Authors, i, i+1)
		return true
	}
	if oldName == "" {
		if len(set.Authors) == 1 && set.Authors[0] == unknownAuthor {
			set.Authors[0] = newName
		} else {
			set.Authors = append(set.Authors, newName)
		}
		return true
	}
	i := slices.Index(set.Authors, oldName)
	if i < 0 {
		return false
	}
	set.Authors[i] = newName
	return true
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
