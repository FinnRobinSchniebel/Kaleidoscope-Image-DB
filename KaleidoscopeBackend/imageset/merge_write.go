package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"fmt"
	"log"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// attachIncomingSource attaches incoming.Sources[0] to target, attributed to
// attributed (its page order), and returns its index in target.Sources. The
// same source keeps its other attributed images and takes incoming's metadata
// and LastChecked; a new one is appended with its tags cleared, for
// ProcessSourceTags to record. An empty Itype or Other is filled from incoming.
func attachIncomingSource(target, incoming *ImageSetMongo, attributed []bson.ObjectID) (index int, isNew bool) {

	source := incoming.Sources[0]

	if target.Itype == "" {
		target.Itype = incoming.Itype
	}

	if target.Other == "" {
		target.Other = incoming.Other
	}

	for i := range target.Sources {

		if !SameSource(target.Sources[i], source) {
			continue
		}

		target.Sources[i].AttributedTo = mergeAttribution(target.Sources[i].AttributedTo, attributed)
		applySourceMetadata(target, i, source)
		target.Sources[i].LastChecked = source.LastChecked
		return i, false
	}

	source.AttributedTo = attributed
	source.Tags = nil
	AppendSource(target, source, source.Description)
	DeriveFromSources(target)
	return len(target.Sources) - 1, true
}

// mergeIntoSet writes incoming into match's set (a duplicate, addition or less
// candidate): an addition inserts its new images, and incoming's source is
// attached to the images it covers. Nothing is written before checkAttribution,
// and a failure removes the new files and undoes the recorded tag counts.
// imagesAdded (the set needs a full-set check) and sourceAdded (a source new to
// the set) are only reported for a write that landed. The caller holds
// LockUserSets.
func mergeIntoSet(userID, ref string, incoming *ImageSetMongo, prepared []PreparedImage, match candidateMatch) (item notification.ItemResult, imagesAdded, sourceAdded bool) {

	target := match.set
	before := slices.Clone(target.Image)
	tagsBefore, autoTagsBefore := slices.Clone(target.Tags), slices.Clone(target.AutoTags)
	itypeBefore, otherBefore := target.Itype, target.Other
	sourcesBefore := slices.Clone(target.Sources)
	for i := range sourcesBefore {
		sourcesBefore[i].Tags = slices.Clone(sourcesBefore[i].Tags)
	}

	keeperOf, added := planNewImages(prepared, match)
	if len(added) > 0 {
		target.Image = placeInsertedImages(target.Image, match, keeperOf, added)
	}
	index, isNew := attachIncomingSource(target, incoming, targetImageIDs(match, keeperOf, added))

	if err := target.checkAttribution(); err != nil {
		return notification.FailedItem(userID, ref, target.ID, fmt.Errorf("%w: %w", notification.ErrSaveFailed, err)), false, false
	}

	plannedIDs := make(map[bson.ObjectID]bool, len(added))
	for _, info := range added {
		plannedIDs[info.ID] = true
	}

	// Until the write lands, a failure removes the new files and the counts recorded so far.
	committed := false
	var counted TagCountChange
	defer func() {

		if committed {
			return
		}

		var written []ImageInfo
		for _, img := range target.Image {
			if plannedIDs[img.ID] {
				written = append(written, img)
			}
		}

		if err := DeleteFilesFromInfoList(target.Path, written); err != nil {
			log.Printf("------ Warning: removing files of failed merge into %s: %s ------", target.ID.Hex(), err)
		}

		if err := Tagger.UndoTagCounts(userID, counted); err != nil {
			log.Printf("------ Warning: undoing tag counts of failed merge into %s: %s ------", target.ID.Hex(), err)
		}
	}()

	if len(added) > 0 {

		toWrite := make(map[bson.ObjectID]PreparedImage, len(added))
		for i, info := range added {
			toWrite[info.ID] = prepared[i]
		}

		if err := writePreparedImages(target, toWrite); err != nil {
			return notification.FailedItem(userID, ref, target.ID, fmt.Errorf("%w: %w", notification.ErrSaveFailed, err)), false, false
		}
	}

	counted, err := Tagger.ProcessSourceTags(userID, target, index, incoming.Sources[0].Tags)
	if err != nil {
		return notification.FailedItem(userID, ref, target.ID, fmt.Errorf("%w: processing source tags: %w", notification.ErrSaveFailed, err)), false, false
	}

	source := &target.Sources[index]
	realChange := len(added) > 0 || isNew ||
		!slices.Equal(source.AttributedTo, sourcesBefore[index].AttributedTo) ||
		sourceDataChanged(sourcesBefore[index], *source)
	//recomputed system tags or a filled Itype/Other still need writing, but aren't the source's change
	otherChange := !slices.Equal(target.Tags, tagsBefore) || !slices.Equal(target.AutoTags, autoTagsBefore) ||
		target.Itype != itypeBefore || target.Other != otherBefore

	if !realChange && !otherChange {
		committed = true
		return notification.ItemResult{Kind: notification.ItemUnchanged, Ref: ref, SetID: target.ID}, false, false
	}

	if realChange {
		now := time.Now()
		source.LastAppliedUpdate = now
		if len(added) > 0 || isNew {
			source.LastImageUpdate = now
		}
	}

	oldThumbnail := target.ThumbNail
	thumbnailCleared, err := saveMergedSet(target, before)
	if err != nil {
		return notification.FailedItem(userID, ref, target.ID, fmt.Errorf("%w: %w", notification.ErrSaveFailed, err)), false, false
	}
	committed = true
	refreshUntagged(userID)

	if thumbnailCleared {
		target.ThumbNail = ""
		if err := DeleteLowResFile(target.Path, oldThumbnail); err != nil {
			log.Printf("------ Warning: removing the old thumbnail of %s: %s ------", target.ID.Hex(), err)
		}
	}

	switch {
	case !realChange:
		return notification.ItemResult{Kind: notification.ItemUnchanged, Ref: ref, SetID: target.ID}, false, false
	case len(added) == 1:
		return notification.ItemResult{Kind: notification.ItemUpdated, Ref: ref, SetID: target.ID, Reason: "added 1 new image to an existing set"}, true, isNew
	case len(added) > 1:
		return notification.ItemResult{Kind: notification.ItemUpdated, Ref: ref, SetID: target.ID, Reason: fmt.Sprintf("added %d new images to an existing set", len(added))}, true, isNew
	default:
		return notification.ItemResult{Kind: notification.ItemUpdated, Ref: ref, SetID: target.ID, Reason: "matched an existing set"}, false, isNew
	}
}

// suggestMerge publishes one suggested merge for setID and returns its Pending
// item, or nothing when there is nobody to suggest. It names each candidate
// with its image pairs (their image IDs must be setID's own), then every set
// holding one of sources that has an ID, without pairs. setID and the excluded
// sets are never named, and no set twice. A failed publish or lookup is only
// logged.
func suggestMerge(userID, ref string, setID bson.ObjectID, candidates []candidateMatch, sources []SourceInfo, excluded []bson.ObjectID) []notification.ItemResult {

	skip := map[bson.ObjectID]bool{setID: true}
	for _, id := range excluded {
		skip[id] = true
	}

	suggestion := notification.SuggestedMerge{SetID: setID}
	for _, candidate := range candidates {
		if !skip[candidate.set.ID] {
			suggestion.Candidates = append(suggestion.Candidates, notification.MergeCandidate{SetID: candidate.set.ID, Pairs: candidate.pairs})
			skip[candidate.set.ID] = true
		}
	}

	for _, source := range sources {

		if source.SourceID == "" {
			continue //ID-less sources never match across sets
		}

		found, err := findSetIDsWithSource(userID, source.Name, source.SourceID)
		if err != nil {
			log.Printf("------ Warning: import [%s] %s: same-source lookup: %s ------", userID, ref, err)
			continue
		}

		for _, id := range found {
			if !skip[id] {
				suggestion.Candidates = append(suggestion.Candidates, notification.MergeCandidate{SetID: id})
				skip[id] = true
			}
		}
	}

	if len(suggestion.Candidates) == 0 {
		return nil
	}

	if err := notification.Publish(userID, suggestion); err != nil {
		log.Printf("------ Warning: import [%s] %s: %s ------", userID, ref, err)
		return nil
	}

	reason := fmt.Sprintf("suggested a merge with %d sets", len(suggestion.Candidates))
	if len(suggestion.Candidates) == 1 {
		reason = "suggested a merge with 1 set"
	}
	return []notification.ItemResult{{Kind: notification.ItemPending, Ref: ref, SetID: setID, Reason: reason}}
}
