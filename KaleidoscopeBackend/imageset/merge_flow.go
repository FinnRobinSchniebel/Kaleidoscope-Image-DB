package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"fmt"
	"slices"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// mergeTargetOutcome is what one merge round did to one of its targets.
type mergeTargetOutcome struct {
	setID       bson.ObjectID
	imagesAdded bool //the target joins the check queue
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
// its images (storedSetAsPreparedImages for a stored set). err means the first
// lookup failed and nothing was decided. The caller holds LockUserSets.
func mergeFlow(userID, ref string, set *ImageSetMongo, prepared []PreparedImage, isNew bool) (mergeResult, error) {

	var result mergeResult
	var excluded []bson.ObjectID
	queue := []bson.ObjectID{set.ID}
	queued := map[bson.ObjectID]bool{set.ID: true}

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
			if entryPrepared, err = storedSetAsPreparedImages(entry.Image); err != nil {
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

		if len(automatic) == 0 {
			result.items = append(result.items, suggestMerge(userID, ref, entry.ID, suggested, excluded)...)
			continue
		}

		var items []notification.ItemResult
		var outcomes []mergeTargetOutcome
		if entryIsNew {
			items, outcomes = mergeImportIntoTargets(userID, ref, entry, entryPrepared, automatic)
		} else {

			var gone bool
			items, outcomes, gone = mergeStoredSetIntoTargets(userID, ref, entry, automatic)
			if !gone {
				outcomes = nil //an interrupted round queues nothing
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
			if outcome.imagesAdded && !queued[outcome.setID] {
				queue = append(queue, outcome.setID)
				queued[outcome.setID] = true
			}
		}
	}
	return result, nil
}

// findMergeCandidates looks images up in userID's library, ignoring hits on
// ownID, and returns the matched sets classified as automatic merges or
// suggestions. An error means the library couldn't be read.
func findMergeCandidates(userID string, images []PreparedImage, ownID bson.ObjectID) (automatic, suggested []mergeCandidate, err error) {

	matches, err := findImageMatches(userID, images)
	if err != nil {
		return nil, nil, err
	}

	for i := range matches {
		matches[i] = slices.DeleteFunc(matches[i], func(hit HashMatch) bool { return hit.SetID == ownID })
	}

	candidateSets, err := loadOwnedSets(userID, findMergeCandidateIDs(matches, len(images)))
	if err != nil {
		return nil, nil, err
	}

	candidates := make([]mergeCandidate, len(candidateSets))
	for i := range candidateSets {
		candidates[i] = classifyMergeCandidate(&candidateSets[i], images, matches)
	}

	automatic, suggested = splitMergeCandidates(candidates)
	return automatic, suggested, nil
}

// mergeImportIntoTargets writes an import into each of its automatic targets,
// each on its own, so a failed write leaves the others. The caller holds
// LockUserSets.
func mergeImportIntoTargets(userID, ref string, set *ImageSetMongo, prepared []PreparedImage, targets []mergeCandidate) (items []notification.ItemResult, outcomes []mergeTargetOutcome) {

	for _, target := range targets {

		item, imagesAdded := mergeIntoSet(userID, ref, set, prepared, target)
		items = append(items, item)
		outcomes = append(outcomes, mergeTargetOutcome{setID: target.set.ID, imagesAdded: imagesAdded})
	}
	return items, outcomes
}
