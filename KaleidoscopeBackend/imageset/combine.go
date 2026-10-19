package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// combineSets merges absorbed into survivor, saves it, then deletes absorbed and
// its leftover files. survivor is changed in place, even when the combine fails.
// Nothing is written until the merged set passes checkAttribution, and until
// survivor's write lands a failure removes the new links and undoes the counts.
// A failure deleting absorbed leaves both sets (an interrupted combine).
// imagesChanged reports that survivor gained images. The caller holds
// LockUserSets.
func combineSets(userID, ref string, survivor, absorbed *ImageSetMongo) (item notification.ItemResult, imagesChanged bool) {

	if survivor.KscopeUserId != userID || absorbed.KscopeUserId != userID {
		err := fmt.Errorf("%w: sets %s and %s aren't both the user's", notification.ErrSaveFailed, survivor.ID.Hex(), absorbed.ID.Hex())
		return notification.FailedItem(userID, ref, survivor.ID, err), false
	}

	before := slices.Clone(survivor.Image)
	oldThumbnail := survivor.ThumbNail
	absorbedIDs := make([]bson.ObjectID, len(absorbed.Image))
	for i, img := range absorbed.Image {
		absorbedIDs[i] = img.ID
	}

	merged, idMap, moved, _, err := collapseImages(survivor, absorbed)
	if err != nil {
		return notification.FailedItem(userID, ref, survivor.ID, fmt.Errorf("%w: %w", notification.ErrSaveFailed, err)), false
	}
	survivor.Image = merged
	indexes := mergeSourcesInto(survivor, absorbed.Sources, idMap)
	mergeSetFields(survivor, absorbed)

	if err := survivor.checkAttribution(); err != nil {
		return notification.FailedItem(userID, ref, survivor.ID, fmt.Errorf("%w: %w", notification.ErrSaveFailed, err)), false
	}

	survivorFolder, err := os.Stat(survivor.Path)
	if err != nil {
		return notification.FailedItem(userID, ref, survivor.ID, fmt.Errorf("%w: %w", notification.ErrSaveFailed, err)), false
	}

	absorbedFolder, err := os.Stat(absorbed.Path)
	if err != nil {
		return notification.FailedItem(userID, ref, survivor.ID, fmt.Errorf("%w: %w", notification.ErrSaveFailed, err)), false
	}
	samePath := os.SameFile(survivorFolder, absorbedFolder)

	// Until survivor's write lands, a failure removes the new links and the counts recorded so far.
	committed := false
	var linked []string
	var counted []TagCountChange
	defer func() {

		if committed {
			return
		}

		for _, name := range linked {
			if err := os.Remove(filepath.Join(survivor.Path, name)); err != nil {
				log.Printf("------ Warning: removing link %s of failed combine into %s: %s ------", name, survivor.ID.Hex(), err)
			}
		}

		for i := len(counted) - 1; i >= 0; i-- {
			if err := Tagger.UndoTagCounts(userID, counted[i]); err != nil {
				log.Printf("------ Warning: undoing tag counts of failed combine into %s: %s ------", survivor.ID.Hex(), err)
			}
		}
	}()

	if !samePath {

		movedIDs := make(map[bson.ObjectID]bool, len(moved))
		for _, img := range moved {

			if err := linkIntoFolder(absorbed.Path, survivor.Path, img.Name); err != nil {
				return notification.FailedItem(userID, ref, survivor.ID, fmt.Errorf("%w: %w", notification.ErrSaveFailed, err)), false
			}

			linked = append(linked, img.Name)
			movedIDs[img.ID] = true
		}

		//a moved image's low-res copy isn't linked, so it regenerates in survivor's folder
		for i := range survivor.Image {
			if movedIDs[survivor.Image[i].ID] {
				survivor.Image[i].LowResName = ""
			}
		}
	}

	for a, index := range indexes {

		change, err := Tagger.ProcessSourceTags(userID, survivor, index, absorbed.Sources[a].Tags)
		if err != nil {
			return notification.FailedItem(userID, ref, survivor.ID, fmt.Errorf("%w: processing source tags: %w", notification.ErrSaveFailed, err)), false
		}
		counted = append(counted, change)
	}

	change, err := Tagger.CombineTagOverrides(userID, survivor, absorbed.TagRuleOverrides)
	if err != nil {
		return notification.FailedItem(userID, ref, survivor.ID, fmt.Errorf("%w: combining tag overrides: %w", notification.ErrSaveFailed, err)), false
	}
	counted = append(counted, change)

	thumbnailCleared, err := saveMergedSet(survivor, before)
	if err != nil {
		return notification.FailedItem(userID, ref, survivor.ID, fmt.Errorf("%w: %w", notification.ErrSaveFailed, err)), false
	}
	committed = true
	refreshUntagged(userID)

	if err := deleteCombinedSet(absorbed, absorbedIDs); err != nil {
		log.Printf("------ Warning: interrupted combine: %s now holds the images and sources of %s, which wasn't deleted ------", survivor.ID.Hex(), absorbed.ID.Hex())
		return notification.FailedItem(userID, ref, survivor.ID, fmt.Errorf("%w: interrupted combine: %w", notification.ErrSaveFailed, err)), false
	}

	if err := Tagger.RecordDeletion(userID, absorbed.Sources, absorbed.Tags); err != nil {
		log.Printf("------ Warning: undoing tag counts of combined set %s: %s ------", absorbed.ID.Hex(), err)
	}
	refreshUntagged(userID)

	survivorIDs := make(map[bson.ObjectID]bool, len(survivor.Image))
	for _, img := range survivor.Image {
		survivorIDs[img.ID] = true
	}

	if err := DeleteFilesFromInfoList(absorbed.Path, filesToRemove(absorbed.Image, survivorIDs, samePath)); err != nil {
		log.Printf("------ Warning: removing files of combined set %s: %s ------", absorbed.ID.Hex(), err)
	}

	//the same name in the same folder would be survivor's own thumbnail
	if !samePath || absorbed.ThumbNail != survivor.ThumbNail {
		if err := DeleteLowResFile(absorbed.Path, absorbed.ThumbNail); err != nil {
			log.Printf("------ Warning: removing the thumbnail of combined set %s: %s ------", absorbed.ID.Hex(), err)
		}
	}

	if thumbnailCleared {
		survivor.ThumbNail = ""
		if err := DeleteLowResFile(survivor.Path, oldThumbnail); err != nil {
			log.Printf("------ Warning: removing the old thumbnail of %s: %s ------", survivor.ID.Hex(), err)
		}
	}

	reason := fmt.Sprintf("combined with set %s", absorbed.ID.Hex())
	return notification.ItemResult{Kind: notification.ItemUpdated, Ref: ref, SetID: survivor.ID, Reason: reason}, len(survivor.Image) > len(before)
}

// asPrepared wraps stored images as an incoming list for the lookup and
// classification. Media is left empty, so the result can't be written.
func asPrepared(images []ImageInfo) ([]PreparedImage, error) {

	prepared := make([]PreparedImage, len(images))
	for i, img := range images {

		plain, err := isPlain(img.PDQ)
		if err != nil {
			return nil, err
		}

		prepared[i] = PreparedImage{Info: img, Plain: plain}
	}
	return prepared, nil
}

// exactMatches returns each image's pixel-identical images in set, in set
// order. A missing or malformed pixel hash never matches.
func exactMatches(images []PreparedImage, set *ImageSetMongo) CollisionMap {

	matches := make(CollisionMap)
	for i, preparedImage := range images {

		if len(preparedImage.Info.Pixel) != pixelHashBytes {
			continue
		}

		for _, stored := range set.Image {
			if bytes.Equal(preparedImage.Info.Pixel, stored.Pixel) {
				matches[i] = append(matches[i], HashMatch{SetID: set.ID, ImageID: stored.ID, Exact: true})
			}
		}
	}
	return matches
}

// collapseImages merges absorbed's images into a new list after survivor's. An
// absorbed image the survivor already has collapses into the survivor's copy
// and repeats within absorbed into the first (dropped); a kept copy is active if
// any of its copies was. The rest keep their IDs (moved) and are placed after
// the nearest earlier matched image. idMap maps every absorbed ID to the ID it
// is kept under.
func collapseImages(survivor, absorbed *ImageSetMongo) (merged []ImageInfo, idMap map[bson.ObjectID]bson.ObjectID, moved, dropped []ImageInfo, err error) {

	incoming, err := asPrepared(absorbed.Image)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	match := classifyCandidate(survivor, incoming, exactMatches(incoming, survivor))
	keeperOf, added := planNewImages(incoming, match)

	idMap = make(map[bson.ObjectID]bson.ObjectID, len(absorbed.Image))
	activeByID := make(map[bson.ObjectID]bool)
	for i, img := range absorbed.Image {

		switch keeper := keeperOf[i]; {
		case keeper == -1:
			idMap[img.ID] = match.exactOf[i][0]
			dropped = append(dropped, img)
		case keeper == i:
			idMap[img.ID] = img.ID
			added[i] = img
			moved = append(moved, img)
		default:
			idMap[img.ID] = absorbed.Image[keeper].ID
			dropped = append(dropped, img)
		}

		if img.IsImageActive {
			activeByID[idMap[img.ID]] = true
		}
	}

	merged = placeInsertedImages(survivor.Image, match, keeperOf, added)
	for i := range merged {
		if activeByID[merged[i].ID] {
			merged[i].IsImageActive = true
		}
	}
	return merged, idMap, moved, dropped, nil
}

// mergeSourcesInto merges absorbed sources into survivor in memory, with their
// attribution remapped through idMap and deduped. The same source collapses
// into survivor's copy (attribution unioned, metadata from the absorbed copy,
// bookkeeping by collapseSourceBookkeeping); any other is appended with no tags.
// Returns each absorbed source's index in survivor.Sources, where the caller
// applies its tags with ProcessSourceTags.
func mergeSourcesInto(survivor *ImageSetMongo, absorbed []SourceInfo, idMap map[bson.ObjectID]bson.ObjectID) []int {

	indexes := make([]int, len(absorbed))
	for a, source := range absorbed {

		remapped := make([]bson.ObjectID, 0, len(source.AttributedTo))
		seen := make(map[bson.ObjectID]bool, len(source.AttributedTo))
		for _, id := range source.AttributedTo {

			kept, mapped := idMap[id]
			if !mapped {
				kept = id //not an absorbed image; checkAttribution rejects the result
			}

			if !seen[kept] {
				seen[kept] = true
				remapped = append(remapped, kept)
			}
		}
		source.AttributedTo = remapped

		index := -1
		for i := range survivor.Sources {
			if SameSource(survivor.Sources[i], source) {
				index = i
				break
			}
		}

		if index == -1 {
			source.Tags = nil
			survivor.Sources = append(survivor.Sources, source)
			indexes[a] = len(survivor.Sources) - 1
			continue
		}

		survivorBefore := survivor.Sources[index]
		survivor.Sources[index].AttributedTo = mergeAttribution(survivorBefore.AttributedTo, source.AttributedTo)
		applySourceMetadata(survivor, index, source)
		collapseSourceBookkeeping(&survivor.Sources[index], survivorBefore, source)
		indexes[a] = index
	}

	deriveAuthors(survivor)
	return indexes
}

// collapseSourceBookkeeping sets kept's bookkeeping from two copies of one
// source: the copy checked last wins (survivorBefore on a tie), except that each
// update timestamp takes the later of the two. Call after applySourceMetadata,
// which also sets Date.
func collapseSourceBookkeeping(kept *SourceInfo, survivorBefore, other SourceInfo) {

	winner, loser := survivorBefore, other
	if other.LastChecked.After(survivorBefore.LastChecked) {
		winner, loser = other, survivorBefore
	}

	kept.LastChecked = winner.LastChecked
	kept.SourceMissing = winner.SourceMissing
	kept.ImageChangeState = winner.ImageChangeState
	kept.Date = mergeSourceDate(loser.Date, winner.Date)

	kept.LastImageUpdate, kept.LastAppliedUpdate = survivorBefore.LastImageUpdate, survivorBefore.LastAppliedUpdate
	if other.LastImageUpdate.After(kept.LastImageUpdate) {
		kept.LastImageUpdate = other.LastImageUpdate
	}

	if other.LastAppliedUpdate.After(kept.LastAppliedUpdate) {
		kept.LastAppliedUpdate = other.LastAppliedUpdate
	}
}

// mergeSetFields takes absorbed's set-level fields into survivor: descriptions
// joined when they differ, an empty Itype or Other filled, and the earlier
// date_added kept.
func mergeSetFields(survivor, absorbed *ImageSetMongo) {

	if absorbed.Description != survivor.Description {
		survivor.Description = JoinDescriptions(survivor.Description, absorbed.Description)
	}

	if survivor.Itype == "" {
		survivor.Itype = absorbed.Itype
	}

	if survivor.Other == "" {
		survivor.Other = absorbed.Other
	}

	if !absorbed.DateAdded.IsZero() && absorbed.DateAdded.Before(survivor.DateAdded) {
		survivor.DateAdded = absorbed.DateAdded
	}
}

// filesToRemove returns the absorbed images whose files can go once the combine
// is saved. In the same folder, images the survivor kept under their own ID are
// its files now, so they stay.
func filesToRemove(absorbed []ImageInfo, survivorIDs map[bson.ObjectID]bool, samePath bool) []ImageInfo {

	if !samePath {
		return slices.Clone(absorbed)
	}

	var leftovers []ImageInfo
	for _, img := range absorbed {
		if !survivorIDs[img.ID] {
			leftovers = append(leftovers, img)
		}
	}
	return leftovers
}
