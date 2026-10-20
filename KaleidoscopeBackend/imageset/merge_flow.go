package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"fmt"
	"slices"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// targetOutcome is what one merge round did to one of its targets.
type targetOutcome struct {
	setID       bson.ObjectID
	imagesAdded bool         //the target joins the check queue
	received    []SourceInfo //sources it now holds whose other sets get looked up
}

// mergeResult is what mergeFlow did.
type mergeResult struct {
	items   []notification.ItemResult
	targets []bson.ObjectID //the first entry's automatic targets, landed or not; none means an import becomes a new set
}

// mergeFlow merges set into userID's library breadth first: each entry goes
// into all of its automatic matches, which survive (a stored entry is then
// removed), and a match that gained images is checked in turn. Suggested merges
// are published here and nowhere else; no set is created. set is an import not
// stored yet when isNew, otherwise a stored set that changed, and prepared holds
// its images (asPrepared for a stored set). err means the first lookup failed
// and nothing was decided. The caller holds LockUserSets.
func mergeFlow(userID, ref string, set *ImageSetMongo, prepared []PreparedImage, isNew bool) (mergeResult, error) {

	var result mergeResult
	var excluded []bson.ObjectID
	queue := []bson.ObjectID{set.ID}
	queued := map[bson.ObjectID]bool{set.ID: true}
	removed := make(map[bson.ObjectID]bool)

	//sources each set received but hasn't looked up yet; pendingOrder keeps the suggestions' order stable
	var pendingOrder []bson.ObjectID
	pending := make(map[bson.ObjectID][]SourceInfo)
	receive := func(id bson.ObjectID, sources []SourceInfo) {

		if _, waiting := pending[id]; !waiting {
			pendingOrder = append(pendingOrder, id)
		}

		for _, source := range sources {
			if !slices.ContainsFunc(pending[id], func(held SourceInfo) bool { return SameSource(held, source) }) {
				pending[id] = append(pending[id], source)
			}
		}
	}
	receive(set.ID, set.Sources)

	first := true
	for len(queue) > 0 {

		id := queue[0]
		queue = queue[1:]
		delete(queued, id)

		entry, entryPrepared, entryIsNew := set, prepared, isNew
		if !first {

			sets, err := loadOwnedSets(userID, []bson.ObjectID{id})
			if err != nil {
				result.items = append(result.items, notification.FailedItem(userID, ref, id, fmt.Errorf("%w: %w", notification.ErrLibraryAccess, err)))
				continue
			}

			if len(sets) == 0 {
				continue //merged into another set since it was queued
			}

			entry, entryIsNew = &sets[0], false
			if entryPrepared, err = asPrepared(entry.Image); err != nil {
				result.items = append(result.items, notification.FailedItem(userID, ref, id, fmt.Errorf("%w: %w", notification.ErrLibraryAccess, err)))
				continue
			}
		}

		automatic, suggested, err := findMergeCandidates(userID, entryPrepared, entry.ID)
		if err != nil && first {
			return mergeResult{}, err
		}

		if err != nil {
			result.items = append(result.items, notification.FailedItem(userID, ref, id, fmt.Errorf("%w: %w", notification.ErrLibraryAccess, err)))
			continue
		}
		wasFirst := first
		first = false

		sources := pending[entry.ID]
		delete(pending, entry.ID) //used for its own suggestion, or reaching its targets as received sources
		if len(automatic) == 0 {
			result.items = append(result.items, suggestMerge(userID, ref, entry.ID, suggested, sources, excluded)...)
			continue
		}

		var items []notification.ItemResult
		var outcomes []targetOutcome
		if entryIsNew {
			items, outcomes = mergeImportIntoTargets(userID, ref, entry, entryPrepared, automatic)
		} else {

			var gone bool
			items, outcomes, gone = mergeStoredSetIntoTargets(userID, ref, entry, automatic)
			if gone {
				removed[entry.ID] = true
			} else {
				outcomes = nil //an interrupted round queues and records nothing
			}
		}
		result.items = append(result.items, items...)

		if wasFirst {
			for _, target := range automatic {
				result.targets = append(result.targets, target.set.ID)
			}
			excluded = result.targets
		}

		for _, outcome := range outcomes {

			if outcome.received != nil {
				receive(outcome.setID, outcome.received)
			}

			if outcome.imagesAdded && !queued[outcome.setID] {
				queue = append(queue, outcome.setID)
				queued[outcome.setID] = true
			}
		}
	}

	for _, id := range pendingOrder {

		sources, waiting := pending[id]
		if !waiting || removed[id] {
			continue
		}

		delete(pending, id)
		result.items = append(result.items, suggestMerge(userID, ref, id, nil, sources, excluded)...)
	}
	return result, nil
}

// findMergeCandidates looks images up in userID's library, ignoring hits on
// ownID, and returns the matched sets classified as automatic merges or
// suggestions. An error means the library couldn't be read.
func findMergeCandidates(userID string, images []PreparedImage, ownID bson.ObjectID) (automatic, suggested []candidateMatch, err error) {

	matches, err := findImageMatches(userID, images)
	if err != nil {
		return nil, nil, err
	}

	for i := range matches {
		matches[i] = slices.DeleteFunc(matches[i], func(hit HashMatch) bool { return hit.SetID == ownID })
	}

	candidateSets, err := loadOwnedSets(userID, findCandidates(matches, len(images)))
	if err != nil {
		return nil, nil, err
	}

	candidates := make([]candidateMatch, len(candidateSets))
	for i := range candidateSets {
		candidates[i] = classifyCandidate(&candidateSets[i], images, matches)
	}

	automatic, suggested = splitCandidates(candidates)
	return automatic, suggested, nil
}

// mergeImportIntoTargets writes an import into each of its automatic targets,
// each on its own, so a failed write leaves the others. The caller holds
// LockUserSets.
func mergeImportIntoTargets(userID, ref string, set *ImageSetMongo, prepared []PreparedImage, targets []candidateMatch) (items []notification.ItemResult, outcomes []targetOutcome) {

	for _, target := range targets {

		item, imagesAdded, sourceAdded := mergeIntoSet(userID, ref, set, prepared, target)
		items = append(items, item)

		outcome := targetOutcome{setID: target.set.ID, imagesAdded: imagesAdded}
		if imagesAdded || sourceAdded {
			outcome.received = set.Sources
		}
		outcomes = append(outcomes, outcome)
	}
	return items, outcomes
}
