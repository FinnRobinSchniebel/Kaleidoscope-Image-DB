package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"bytes"
	"context"
	"fmt"
	"log"
	"slices"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// mergeTargetOutcome is what one merge round did to one of its targets.
type mergeTargetOutcome struct {
	setID       bson.ObjectID
	imagesAdded bool             //the target joins the check queue
	heldCopies  []heldCopyChange //folded into carriedNotifications
}

// heldCopyChange is what one merge write did to one source's held copies
// (pending or declined), from the copies as they were before it.
type heldCopyChange struct {
	service, sourceID string
	survivorLost      bool //the survivor's held copy was reset, or replaced by the absorbed copy's state
	absorbedKept      bool //the absorbed set's held copy's state now sits on the survivor
}

// makeKey returns the pending-update key of this source's copy on setID.
func (change heldCopyChange) makeKey(setID bson.ObjectID) notification.PendingUpdateKey {
	return notification.PendingUpdateKey{SetID: setID, Service: change.service, SourceID: change.sourceID}
}

// carriedNotifications follows held copies' pending-update notifications through
// one mergeFlow. locations maps a held copy's key to its notification's origin,
// the key it's stored under; a held copy not in it carries its own.
type carriedNotifications struct {
	locations map[notification.PendingUpdateKey]notification.PendingUpdateKey
	origins   map[notification.PendingUpdateKey]bool //every origin touched, settled at the end
}

// trackOrigin returns the origin of the notification the held copy at key
// carries, recording an untouched copy as carrying its own.
func (c *carriedNotifications) trackOrigin(key notification.PendingUpdateKey) notification.PendingUpdateKey {

	if origin, carried := c.locations[key]; carried {
		return origin
	}

	if c.locations == nil {
		c.locations = make(map[notification.PendingUpdateKey]notification.PendingUpdateKey)
		c.origins = make(map[notification.PendingUpdateKey]bool)
	}

	c.locations[key] = key
	c.origins[key] = true
	return key
}

// dropLocation records that the held copy at key lost its hold or its set. Its
// origin stays tracked, so with no location left its notification is
// invalidated.
func (c *carriedNotifications) dropLocation(key notification.PendingUpdateKey) {

	c.trackOrigin(key)
	delete(c.locations, key)
}

// recordMergeWrite folds one landed write of a round over entryID into the map:
// a lost hold drops the target's location, and a kept absorbed hold makes the
// target carry the entry's notification. It must run before recordSetRemoved
// for the same entry, or the entry's dropped key would read as untouched.
func (c *carriedNotifications) recordMergeWrite(entryID bson.ObjectID, outcome mergeTargetOutcome) {

	for _, change := range outcome.heldCopies {

		target := change.makeKey(outcome.setID)
		if change.survivorLost {
			c.dropLocation(target)
		}

		if change.absorbedKept {
			origin := c.trackOrigin(change.makeKey(entryID))
			c.locations[target] = origin
		}
	}
}

// recordSetRemoved drops the location of every held copy on set, which its
// round removed.
func (c *carriedNotifications) recordSetRemoved(set *ImageSetMongo) {

	for i := range set.Sources {
		if set.Sources[i].ImageChangeState != ImageChangeNone {
			c.dropLocation(newPendingImageUpdate(set, i).PendingUpdateKey)
		}
	}
}

// collectOriginLocations returns every tracked origin with the IDs of the sets
// its copies are held on now, in ID order. An origin with none is to be
// invalidated.
func (c *carriedNotifications) collectOriginLocations() map[notification.PendingUpdateKey][]bson.ObjectID {

	sets := make(map[notification.PendingUpdateKey][]bson.ObjectID, len(c.origins))
	for origin := range c.origins {
		sets[origin] = nil
	}

	for location, origin := range c.locations {
		sets[origin] = append(sets[origin], location.SetID)
	}

	for _, ids := range sets {
		slices.SortFunc(ids, func(a, b bson.ObjectID) int { return bytes.Compare(a[:], b[:]) })
	}
	return sets
}

// settle makes every tracked origin's notifications follow its copies
// (notification.SettlePendingImageUpdates). A failure is only logged, leaving
// the rest to the lazy check and the startup repair. The caller must still hold
// LockUserSets: the settlement takes every live notification of an origin,
// which is only right while none can be published.
func (c *carriedNotifications) settle(userID string) {

	if len(c.origins) == 0 {
		return
	}

	if err := notification.SettlePendingImageUpdates(context.Background(), userID, c.collectOriginLocations()); err != nil {
		log.Printf("------ Warning: settling the notifications of held copies a merge changed: %s ------", err)
	}
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
// its images (storedSetAsPreparedImages for a stored set). Once the queue is
// empty, the pending-update notifications of the held copies its writes moved
// or reset follow them (carriedNotifications). err means the first lookup
// failed and nothing was decided. The caller holds LockUserSets.
func mergeFlow(userID, ref string, set *ImageSetMongo, prepared []PreparedImage, isNew bool) (mergeResult, error) {

	var result mergeResult
	var excluded []bson.ObjectID
	var carried carriedNotifications
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
		var gone bool
		if entryIsNew {
			items, outcomes = mergeImportIntoTargets(userID, ref, entry, entryPrepared, automatic)
		} else {
			items, outcomes, gone = mergeStoredSetIntoTargets(userID, ref, entry, automatic)
		}
		result.items = append(result.items, items...)

		if wasFirst {
			for _, target := range automatic {
				result.targets = append(result.targets, target.set.ID)
			}
			excluded = result.targets
		}

		for _, outcome := range outcomes {

			carried.recordMergeWrite(entry.ID, outcome)

			//an interrupted round queues nothing
			if outcome.imagesAdded && (entryIsNew || gone) && !queued[outcome.setID] {
				queue = append(queue, outcome.setID)
				queued[outcome.setID] = true
			}
		}

		//after its writes, which can carry its holds to the targets
		if gone {
			carried.recordSetRemoved(entry)
		}
	}

	carried.settle(userID)
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

		item, outcome := mergeIntoSet(userID, ref, set, prepared, target)
		items = append(items, item)
		outcomes = append(outcomes, outcome)
	}
	return items, outcomes
}
