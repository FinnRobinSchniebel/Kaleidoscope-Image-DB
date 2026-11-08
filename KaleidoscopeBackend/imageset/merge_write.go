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
// same source keeps its other attributed images, with the winning copy's order
// first (doesIncomingSourceInfoWin), and takes incoming's metadata and
// LastChecked; a held copy keeps its Date. A new one is appended with its tags
// cleared, for ProcessSourceTags to record. An empty Itype or Other is filled
// from incoming.
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

		stored := target.Sources[i] //read before the writes below, whose zero LastChecked would make a zip win
		leading, rest := attributed, stored.AttributedTo
		if !doesIncomingSourceInfoWin(stored, source) {
			leading, rest = rest, leading
		}

		target.Sources[i].AttributedTo = mergeAttribution(rest, leading)
		applySourceMetadata(target, i, source)

		if stored.ImageChangeState != ImageChangeNone {
			target.Sources[i].Date = stored.Date
		}

		target.Sources[i].LastChecked = source.LastChecked
		return i, false
	}

	source.AttributedTo = attributed
	source.Tags = nil
	AppendSource(target, source, source.Description)
	DeriveFromSources(target)
	return len(target.Sources) - 1, true
}

// resetHeldSourceCopies sets every pending or declined copy on set back to none,
// for a set that gained images: a hold only covers the images its check saw.
// Each copy's held Date is left alone, so the next sync checks it again.
func resetHeldSourceCopies(set *ImageSetMongo) {

	for i := range set.Sources {
		set.Sources[i].ImageChangeState = ImageChangeNone
	}
}

// mergeIntoSet writes incoming into match's set (a duplicate, addition or less
// candidate): an addition inserts its new images and resets the set's held
// copies (resetHeldSourceCopies), and incoming's source is attached to the
// images it covers. Nothing is written before checkAttribution,
// and a failure removes the new files and undoes the recorded tag counts.
// imagesAdded (the set needs checking in turn) is only reported for a write
// that landed. The caller holds LockUserSets.
func mergeIntoSet(userID, ref string, incoming *ImageSetMongo, prepared []PreparedImage, match mergeCandidate) (item notification.ItemResult, imagesAdded bool) {

	target := match.set
	before := slices.Clone(target.Image)
	tagsBefore, autoTagsBefore := slices.Clone(target.Tags), slices.Clone(target.AutoTags)
	itypeBefore, otherBefore := target.Itype, target.Other
	sourcesBefore := slices.Clone(target.Sources)
	for i := range sourcesBefore {
		sourcesBefore[i].Tags = slices.Clone(sourcesBefore[i].Tags)
	}

	keeperOf, added := planImagesToMerge(prepared, match)
	if len(added) > 0 {
		target.Image = placeMergedImages(target.Image, match, keeperOf, added)
	}
	index, isNew := attachIncomingSource(target, incoming, findMergedAttributionImageIDs(match, keeperOf, added))

	//after the attach, which keeps a held copy's Date
	if len(added) > 0 {
		resetHeldSourceCopies(target)
	}

	if err := target.checkAttribution(); err != nil {
		return notification.FailedItem(userID, ref, target.ID, fmt.Errorf("%w: %w", notification.ErrSaveFailed, err)), false
	}

	// Until the write lands, a failure removes the new files and the counts recorded so far.
	committed := false
	var counted TagCountChange
	defer func() {
		if !committed {
			undoImageInsert(target, added, counted)
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

	if err := saveMergedSet(target, before); err != nil {
		return notification.FailedItem(userID, ref, target.ID, fmt.Errorf("%w: %w", notification.ErrSaveFailed, err)), false
	}
	committed = true
	refreshUntagged(userID)

	switch {
	case !realChange:
		return notification.ItemResult{Kind: notification.ItemUnchanged, Ref: ref, SetID: target.ID}, false
	case len(added) == 1:
		return notification.ItemResult{Kind: notification.ItemUpdated, Ref: ref, SetID: target.ID, Reason: "added 1 new image to an existing set"}, true
	case len(added) > 1:
		return notification.ItemResult{Kind: notification.ItemUpdated, Ref: ref, SetID: target.ID, Reason: fmt.Sprintf("added %d new images to an existing set", len(added))}, true
	default:
		return notification.ItemResult{Kind: notification.ItemUpdated, Ref: ref, SetID: target.ID, Reason: "matched an existing set"}, false
	}
}

// undoImageInsert reverses an insert into set whose write didn't land: it removes
// the files written for added (named on set.Image) and undoes counted. Failures
// are only logged.
func undoImageInsert(set *ImageSetMongo, added map[int]ImageInfo, counted TagCountChange) {

	planned := make(map[bson.ObjectID]bool, len(added))
	for _, info := range added {
		planned[info.ID] = true
	}

	var written []ImageInfo
	for _, img := range set.Image {
		if planned[img.ID] {
			written = append(written, img)
		}
	}

	if err := DeleteFilesFromInfoList(set.Path, written); err != nil {
		log.Printf("------ Warning: removing files of failed insert into %s: %s ------", set.ID.Hex(), err)
	}

	if err := Tagger.UndoTagCounts(set.KscopeUserId, counted); err != nil {
		log.Printf("------ Warning: undoing tag counts of failed insert into %s: %s ------", set.ID.Hex(), err)
	}
}

// suggestMerge publishes one suggested merge for setID and returns its Pending
// item, or nothing when there is nobody to suggest. It names each candidate
// with its image pairs (their image IDs must be setID's own). setID and the
// excluded sets are never named, and no set twice. A failed publish is a Failed
// item for setID.
func suggestMerge(userID, ref string, setID bson.ObjectID, candidates []mergeCandidate, excluded []bson.ObjectID) []notification.ItemResult {

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

	if len(suggestion.Candidates) == 0 {
		return nil
	}

	if err := notification.Publish(userID, suggestion); err != nil {
		return []notification.ItemResult{notification.FailedItem(userID, ref, setID, fmt.Errorf("%w: %w", notification.ErrSuggestionSaveFailed, err))}
	}

	reason := fmt.Sprintf("suggested a merge with %d sets", len(suggestion.Candidates))
	if len(suggestion.Candidates) == 1 {
		reason = "suggested a merge with 1 set"
	}
	return []notification.ItemResult{{Kind: notification.ItemPending, Ref: ref, SetID: setID, Reason: reason}}
}
