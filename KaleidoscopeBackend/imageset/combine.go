package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"bytes"
	"fmt"
	"log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// mergePlan is what a merge decides before anything is written. It names image
// IDs only; applyMergePlan takes each image's entry from the two sets.
type mergePlan struct {
	survivor, absorbed *ImageSetMongo
	order              []bson.ObjectID                 //survivor's final image IDs, in order
	inactive           map[bson.ObjectID]bool          //IDs in order to mark inactive; the rest are active
	idMap              map[bson.ObjectID]bson.ObjectID //absorbed image ID to the ID it's kept under
	copiedFrom         map[bson.ObjectID]bson.ObjectID //a fresh-ID copy to the absorbed image it copies
	description        string
}

// orderedImages returns the survivor's final image entries in plan.order, with
// flags from plan.inactive. A copy is its absorbed entry under the fresh ID and
// a new file name, with no low-res yet. An ID in neither set is an error.
func (plan mergePlan) orderedImages() ([]ImageInfo, error) {

	absorbedEntries := make(map[bson.ObjectID]ImageInfo, len(plan.absorbed.Image))
	for _, img := range plan.absorbed.Image {
		absorbedEntries[img.ID] = img
	}

	entries := maps.Clone(absorbedEntries)
	for _, img := range plan.survivor.Image {
		entries[img.ID] = img //an image both sets hold keeps the survivor's file
	}

	images := make([]ImageInfo, len(plan.order))
	for i, id := range plan.order {

		img, found := entries[id]
		if sourceID, copied := plan.copiedFrom[id]; copied {
			img, found = absorbedEntries[sourceID]
			img.ID = id
			img.Name = ImageFileName(plan.survivor.Title, id, strings.TrimPrefix(filepath.Ext(img.Name), "."))
			img.LowResName = ""
		}

		if !found {
			return nil, fmt.Errorf("image %s is in neither set", id.Hex())
		}

		img.IsImageActive = !plan.inactive[id]
		images[i] = img
	}
	return images, nil
}

// mergeStoredSetIntoTargets writes set into each of its automatic targets, which
// survive, then removes set once every write has landed. Targets that gain
// nothing are written first and gaining ones last, stopping at the first
// failure, so a kept-ID move into set's own folder is always the final write. A
// failed write or removal keeps set: an interrupted merge, with removed false.
// The caller holds LockUserSets.
func mergeStoredSetIntoTargets(userID, ref string, set *ImageSetMongo, targets []mergeCandidate) (items []notification.ItemResult, outcomes []mergeTargetOutcome, removed bool) {

	freshIDs := doesMergeNeedNewImageIDs(targets)
	gains := func(target mergeCandidate) int {
		if target.scenario == scenarioAddition {
			return 1
		}
		return 0
	}
	ordered := slices.Clone(targets)
	slices.SortStableFunc(ordered, func(a, b mergeCandidate) int { return gains(a) - gains(b) })

	survivors := make([]*ImageSetMongo, 0, len(ordered))
	for _, target := range ordered {

		var imagesAdded, sourcesAdded bool
		plan, err := makeAutoMergePlan(target.set, set, freshIDs)
		if err == nil {
			imagesAdded, sourcesAdded, err = applyMergePlan(userID, plan)
		}

		if err != nil {
			return append(items, notification.FailedItem(userID, ref, target.set.ID, fmt.Errorf("%w: %w", notification.ErrSaveFailed, err))), outcomes, false
		}

		reason := fmt.Sprintf("combined with set %s", set.ID.Hex())
		items = append(items, notification.ItemResult{Kind: notification.ItemUpdated, Ref: ref, SetID: target.set.ID, Reason: reason})

		outcome := mergeTargetOutcome{setID: target.set.ID, imagesAdded: imagesAdded}
		if imagesAdded || sourcesAdded {
			outcome.received = set.Sources
		}
		outcomes = append(outcomes, outcome)
		survivors = append(survivors, target.set)
	}

	if err := removeMergedSet(userID, set, survivors); err != nil {
		return append(items, notification.FailedItem(userID, ref, set.ID, fmt.Errorf("%w: %w", notification.ErrSaveFailed, err))), outcomes, false
	}
	return items, outcomes, true
}

// doesMergeNeedNewImageIDs reports that more than one target would gain images,
// so each needs its own copies of them.
func doesMergeNeedNewImageIDs(targets []mergeCandidate) bool {

	gaining := 0
	for _, target := range targets {
		if target.scenario == scenarioAddition {
			gaining++
		}
	}
	return gaining > 1
}

// makeAutoMergePlan makes the automatic merge's decisions:
// collapseMergedSetImages' order (fresh-ID copies when freshIDs is set), an
// image inactive only when every copy of it was, and the descriptions joined
// when they differ. Changes neither set.
func makeAutoMergePlan(survivor, absorbed *ImageSetMongo, freshIDs bool) (mergePlan, error) {

	images, idMap, err := collapseMergedSetImages(survivor, absorbed, freshIDs)
	if err != nil {
		return mergePlan{}, err
	}

	existing := make(map[bson.ObjectID]bool, len(survivor.Image)+len(absorbed.Image))
	for _, img := range slices.Concat(survivor.Image, absorbed.Image) {
		existing[img.ID] = true
	}

	activeCopy := make(map[bson.ObjectID]bool, len(absorbed.Image))
	copiedFrom := make(map[bson.ObjectID]bson.ObjectID)
	for _, img := range absorbed.Image {

		kept := idMap[img.ID]
		if img.IsImageActive {
			activeCopy[kept] = true
		}

		//a copy's ID is in neither set; absorbed order records a keeper before its repeats
		if _, recorded := copiedFrom[kept]; !existing[kept] && !recorded {
			copiedFrom[kept] = img.ID
		}
	}

	plan := mergePlan{
		survivor:    survivor,
		absorbed:    absorbed,
		order:       make([]bson.ObjectID, len(images)),
		inactive:    make(map[bson.ObjectID]bool),
		idMap:       idMap,
		copiedFrom:  copiedFrom,
		description: survivor.Description,
	}
	for i, img := range images {

		plan.order[i] = img.ID
		if !img.IsImageActive && !activeCopy[img.ID] {
			plan.inactive[img.ID] = true
		}
	}

	if absorbed.Description != survivor.Description {
		plan.description = JoinDescriptions(survivor.Description, absorbed.Description)
	}
	return plan, nil
}

// applyMergePlan writes plan.absorbed into plan.survivor and saves the survivor;
// the absorbed set stays until removeMergedSet. The survivor is changed in
// place, even when this fails. Nothing is written until the merged set passes
// checkAttribution, and until the save lands a failure removes the new links and
// undoes the counts. The caller holds LockUserSets.
func applyMergePlan(userID string, plan mergePlan) (imagesAdded, sourcesAdded bool, err error) {

	survivor, absorbed := plan.survivor, plan.absorbed
	if survivor.KscopeUserId != userID || absorbed.KscopeUserId != userID {
		return false, false, fmt.Errorf("sets %s and %s aren't both the user's", survivor.ID.Hex(), absorbed.ID.Hex())
	}

	if survivor.ID == absorbed.ID {
		return false, false, fmt.Errorf("set %s can't be merged with itself", survivor.ID.Hex())
	}

	before := slices.Clone(survivor.Image)
	sourcesBefore := len(survivor.Sources)

	images, err := plan.orderedImages()
	if err != nil {
		return false, false, err
	}
	survivor.Image = images
	survivor.Description = plan.description
	indexes := mergeSourcesInto(survivor, absorbed.Sources, plan.idMap)
	mergeSetFields(survivor, absorbed)

	if err := survivor.checkAttribution(); err != nil {
		return false, false, err
	}

	survivorFolder, err := os.Stat(survivor.Path)
	if err != nil {
		return false, false, err
	}

	absorbedFolder, err := os.Stat(absorbed.Path)
	if err != nil {
		return false, false, err
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
				log.Printf("------ Warning: removing link %s of failed merge into %s: %s ------", name, survivor.ID.Hex(), err)
			}
		}

		for i := len(counted) - 1; i >= 0; i-- {
			if err := Tagger.UndoTagCounts(userID, counted[i]); err != nil {
				log.Printf("------ Warning: undoing tag counts of failed merge into %s: %s ------", survivor.ID.Hex(), err)
			}
		}
	}()

	held := make(map[bson.ObjectID]bool, len(before))
	for _, img := range before {
		held[img.ID] = true
	}

	absorbedNames := make(map[bson.ObjectID]string, len(absorbed.Image))
	for _, img := range absorbed.Image {
		absorbedNames[img.ID] = img.Name
	}

	for i := range survivor.Image {

		gained := &survivor.Image[i]
		sourceID, copied := plan.copiedFrom[gained.ID]
		switch {
		case held[gained.ID]:
			continue
		case copied:
			//linked under its new name even within one folder
			if err := linkIntoFolder(absorbed.Path, absorbedNames[sourceID], survivor.Path, gained.Name); err != nil {
				return false, false, err
			}
		case samePath:
			continue //a moved image in the same folder: the survivor now names its file and low-res
		default:
			if err := linkIntoFolder(absorbed.Path, gained.Name, survivor.Path, gained.Name); err != nil {
				return false, false, err
			}
			gained.LowResName = "" //its low-res copy isn't linked, so it regenerates in survivor's folder
		}
		linked = append(linked, gained.Name)
	}

	for a, index := range indexes {

		change, err := Tagger.ProcessSourceTags(userID, survivor, index, absorbed.Sources[a].Tags)
		if err != nil {
			return false, false, fmt.Errorf("processing source tags: %w", err)
		}
		counted = append(counted, change)
	}

	change, err := Tagger.CombineTagOverrides(userID, survivor, absorbed.TagRuleOverrides)
	if err != nil {
		return false, false, fmt.Errorf("combining tag overrides: %w", err)
	}
	counted = append(counted, change)

	if err := saveMergedSet(survivor, before); err != nil {
		return false, false, err
	}
	committed = true
	refreshUntagged(userID)

	return len(survivor.Image) > len(before), len(survivor.Sources) > sourcesBefore, nil
}

// removeMergedSet removes absorbed once its content has been written into
// survivors: its leftover files first, then its document, then its counts, so a
// failure leaves the files listed for a retry rather than orphaned (an
// interrupted merge). The caller holds LockUserSets.
func removeMergedSet(userID string, absorbed *ImageSetMongo, survivors []*ImageSetMongo) error {

	leftovers, err := mergedSetFilesToRemove(absorbed, survivors)
	if err == nil {
		err = DeleteFilesFromInfoList(absorbed.Path, leftovers)
	}

	if err != nil {
		log.Printf("------ Warning: interrupted merge: %s was merged, but its files couldn't all be removed ------", absorbed.ID.Hex())
		return fmt.Errorf("interrupted merge: removing files of %s: %w", absorbed.ID.Hex(), err)
	}

	thumbnailShared := slices.ContainsFunc(survivors, func(survivor *ImageSetMongo) bool {
		return absorbed.ThumbNail != "" && survivor.ThumbNail == absorbed.ThumbNail
	})
	if !thumbnailShared {
		if err := DeleteLowResFile(absorbed.Path, absorbed.ThumbNail); err != nil {
			log.Printf("------ Warning: interrupted merge: %s was merged, but its thumbnail couldn't be removed ------", absorbed.ID.Hex())
			return fmt.Errorf("interrupted merge: removing the thumbnail of %s: %w", absorbed.ID.Hex(), err)
		}
	}

	absorbedIDs := make([]bson.ObjectID, len(absorbed.Image))
	for i, img := range absorbed.Image {
		absorbedIDs[i] = img.ID
	}

	if err := deleteCombinedSet(absorbed, absorbedIDs); err != nil {
		log.Printf("------ Warning: interrupted merge: %s was merged, but wasn't deleted ------", absorbed.ID.Hex())
		return fmt.Errorf("interrupted merge: %w", err)
	}

	if err := Tagger.RecordDeletion(userID, absorbed.Sources, absorbed.Tags); err != nil {
		log.Printf("------ Warning: undoing tag counts of merged set %s: %s ------", absorbed.ID.Hex(), err)
	}
	refreshUntagged(userID)
	return nil
}

// storedSetAsPreparedImages wraps a stored set's images as an incoming list for
// the lookup and classification. Media is left empty, so the result can't be
// written.
func storedSetAsPreparedImages(images []ImageInfo) ([]PreparedImage, error) {

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

// mergedSetExactMatches returns each image's pixel-identical images in set, in
// set order. A missing or malformed pixel hash never matches.
func mergedSetExactMatches(images []PreparedImage, set *ImageSetMongo) CollisionMap {

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

// collapseMergedSetImages merges absorbed's images into a new list after
// survivor's. An absorbed image the survivor already has collapses into the
// survivor's copy, and repeats within absorbed into the first. The rest keep
// their entries, under a fresh ID when freshIDs is set, and are placed after the
// nearest earlier matched image (the automatic order). Every image keeps its
// stored flag. idMap maps every absorbed ID to the ID it is kept under.
func collapseMergedSetImages(survivor, absorbed *ImageSetMongo, freshIDs bool) (images []ImageInfo, idMap map[bson.ObjectID]bson.ObjectID, err error) {

	incoming, err := storedSetAsPreparedImages(absorbed.Image)
	if err != nil {
		return nil, nil, err
	}

	match := classifyMergeCandidate(survivor, incoming, mergedSetExactMatches(incoming, survivor))
	keeperOf, added := planImagesToMerge(incoming, match)

	idMap = make(map[bson.ObjectID]bson.ObjectID, len(absorbed.Image))
	for i, img := range absorbed.Image {

		switch keeper := keeperOf[i]; {
		case keeper == -1:
			idMap[img.ID] = match.exactOf[i][0]
		case keeper == i:
			kept := img
			if freshIDs {
				kept.ID = added[i].ID
			}
			idMap[img.ID] = kept.ID
			added[i] = kept
		default:
			idMap[img.ID] = idMap[absorbed.Image[keeper].ID] //a keeper always comes before its repeats
		}
	}
	return placeMergedImages(survivor.Image, match, keeperOf, added), idMap, nil
}

// mergeSourcesInto merges absorbed sources into survivor in memory, with their
// attribution remapped through idMap and deduped. The same source collapses
// into survivor's copy (attribution unioned, metadata from the absorbed copy,
// bookkeeping by collapseMergedSourceBookkeeping); any other is appended with no
// tags. Returns each absorbed source's index in survivor.Sources, where the
// caller applies its tags with ProcessSourceTags.
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
		collapseMergedSourceBookkeeping(&survivor.Sources[index], survivorBefore, source)
		indexes[a] = index
	}

	deriveAuthors(survivor)
	return indexes
}

// collapseMergedSourceBookkeeping sets kept's bookkeeping from two copies of one
// source: the copy checked last wins (survivorBefore on a tie), except that each
// update timestamp takes the later of the two. Call after applySourceMetadata,
// which also sets Date.
func collapseMergedSourceBookkeeping(kept *SourceInfo, survivorBefore, other SourceInfo) {

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

// mergeSetFields takes absorbed's set-level fields into survivor: an empty Itype
// or Other filled, and the earlier date_added kept.
func mergeSetFields(survivor, absorbed *ImageSetMongo) {

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

// mergedSetFilesToRemove returns the absorbed images whose files can go once
// it's merged. An image a survivor in the same folder holds under the same ID
// is that survivor's file now, so it stays.
func mergedSetFilesToRemove(absorbed *ImageSetMongo, survivors []*ImageSetMongo) ([]ImageInfo, error) {

	absorbedFolder, err := os.Stat(absorbed.Path)
	if err != nil {
		return nil, err
	}

	keep := make(map[bson.ObjectID]bool)
	for _, survivor := range survivors {

		survivorFolder, err := os.Stat(survivor.Path)
		if err != nil {
			return nil, err
		}

		if !os.SameFile(absorbedFolder, survivorFolder) {
			continue
		}

		for _, img := range survivor.Image {
			keep[img.ID] = true
		}
	}

	var leftovers []ImageInfo
	for _, img := range absorbed.Image {
		if !keep[img.ID] {
			leftovers = append(leftovers, img)
		}
	}
	return leftovers, nil
}
