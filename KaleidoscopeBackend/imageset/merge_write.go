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
// changedForLookup reports images inserted or a source new to the set, which
// is when the same-source lookup runs. The caller holds LockUserSets.
func mergeIntoSet(userID, ref string, incoming *ImageSetMongo, prepared []PreparedImage, match candidateMatch) (item notification.ItemResult, changedForLookup bool) {

	target := match.set
	beforeIDs := make([]bson.ObjectID, len(target.Image))
	for i, img := range target.Image {
		beforeIDs[i] = img.ID
	}
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
		return notification.FailedItem(userID, ref, target.ID, fmt.Errorf("%w: %w", notification.ErrSaveFailed, err)), false
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
			return notification.FailedItem(userID, ref, target.ID, fmt.Errorf("%w: %w", notification.ErrSaveFailed, err)), false
		}
	}

	counted, err := Tagger.ProcessSourceTags(userID, target, index, incoming.Sources[0].Tags)
	if err != nil {
		return notification.FailedItem(userID, ref, target.ID, fmt.Errorf("%w: processing source tags: %w", notification.ErrSaveFailed, err)), false
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
		return notification.ItemResult{Kind: notification.ItemUnchanged, Ref: ref, SetID: target.ID}, false
	}

	if realChange {
		now := time.Now()
		source.LastAppliedUpdate = now
		if len(added) > 0 || isNew {
			source.LastImageUpdate = now
		}
	}

	oldThumbnail := target.ThumbNail
	thumbnailCleared, err := saveMergedSet(target, beforeIDs)
	if err != nil {
		return notification.FailedItem(userID, ref, target.ID, fmt.Errorf("%w: %w", notification.ErrSaveFailed, err)), false
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
		return notification.ItemResult{Kind: notification.ItemUnchanged, Ref: ref, SetID: target.ID}, false
	case len(added) == 1:
		return notification.ItemResult{Kind: notification.ItemUpdated, Ref: ref, SetID: target.ID, Reason: "added 1 new image to an existing set"}, true
	case len(added) > 1:
		return notification.ItemResult{Kind: notification.ItemUpdated, Ref: ref, SetID: target.ID, Reason: fmt.Sprintf("added %d new images to an existing set", len(added))}, true
	default:
		return notification.ItemResult{Kind: notification.ItemUpdated, Ref: ref, SetID: target.ID, Reason: "matched an existing set"}, isNew
	}
}

// suggestMerges publishes the import's suggested merges and returns a Pending
// item for each. newSetID (zero when no set was made) is suggested with every
// candidate, with its image pairs. When source has an ID and the import made a
// new set or changed a target (changedTargets), the sets holding the same
// source are suggested too, without pairs. The new set and every set in written
// are never suggested. A failed publish is only logged.
func suggestMerges(userID, ref string, source SourceInfo, newSetID bson.ObjectID, changedTargets, written []bson.ObjectID, candidates []candidateMatch) []notification.ItemResult {

	excluded := make(map[bson.ObjectID]bool)
	for _, id := range written {
		excluded[id] = true
	}
	if !newSetID.IsZero() {
		excluded[newSetID] = true
	}

	var sameSource []bson.ObjectID
	if source.SourceID != "" && (!newSetID.IsZero() || len(changedTargets) > 0) {

		found, err := findSetIDsWithSource(userID, source.Name, source.SourceID)
		if err != nil {
			log.Printf("------ Warning: import [%s] %s: same-source lookup: %s ------", userID, ref, err)
		}
		sameSource = found
	}

	var suggestions []notification.SuggestedMerge
	if !newSetID.IsZero() {

		suggestion := notification.SuggestedMerge{SetID: newSetID}
		listed := make(map[bson.ObjectID]bool)
		for _, candidate := range candidates {
			if !excluded[candidate.set.ID] {
				suggestion.Candidates = append(suggestion.Candidates, notification.MergeCandidate{SetID: candidate.set.ID, Pairs: candidate.pairs})
				listed[candidate.set.ID] = true
			}
		}

		for _, id := range sameSource {
			if !excluded[id] && !listed[id] {
				suggestion.Candidates = append(suggestion.Candidates, notification.MergeCandidate{SetID: id})
			}
		}
		suggestions = append(suggestions, suggestion)
	}

	for _, target := range changedTargets {

		suggestion := notification.SuggestedMerge{SetID: target}
		for _, id := range sameSource {
			if !excluded[id] {
				suggestion.Candidates = append(suggestion.Candidates, notification.MergeCandidate{SetID: id})
			}
		}
		suggestions = append(suggestions, suggestion)
	}

	var results []notification.ItemResult
	for _, suggestion := range suggestions {

		if len(suggestion.Candidates) == 0 {
			continue
		}

		if err := notification.Publish(userID, suggestion); err != nil {
			log.Printf("------ Warning: import [%s] %s: %s ------", userID, ref, err)
			continue
		}

		reason := fmt.Sprintf("suggested a merge with %d sets", len(suggestion.Candidates))
		if len(suggestion.Candidates) == 1 {
			reason = "suggested a merge with 1 set"
		}
		results = append(results, notification.ItemResult{Kind: notification.ItemPending, Ref: ref, SetID: suggestion.SetID, Reason: reason})
	}
	return results
}
