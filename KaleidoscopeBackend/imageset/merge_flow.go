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
// one merge flow. locations maps a held copy's key to its notification's origin,
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
			c.dropLocation(makePendingUpdateKey(set, i))
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

// mergeResult is what a merge flow did; importMergeFlow returns it.
type mergeResult struct {
	items   []notification.ItemResult
	targets []bson.ObjectID //the import's automatic targets, landed or not, which later suggestions never name; none means it becomes a new set
}

// mergeFlowState is one merge flow's progress. A flow merges a set into its
// user's library breadth first: each entry goes into all of its automatic
// matches, which survive (a stored entry is then removed), and a match that
// gained images gets its own round in turn. Only a flow publishes suggested
// merges, and it never creates a set.
type mergeFlowState struct {
	userID, ref string
	result      mergeResult
	queue       []bson.ObjectID //sets waiting for their round
	carried     carriedNotifications
}

// recordRound adds a round's items to the report and folds its landed writes
// into carried. With queueGains, each target that gained images is queued
// unless it's already waiting; an interrupted stored round passes false.
func (f *mergeFlowState) recordRound(entryID bson.ObjectID, items []notification.ItemResult, outcomes []mergeTargetOutcome, queueGains bool) {

	f.result.items = append(f.result.items, items...)
	for _, outcome := range outcomes {

		f.carried.recordMergeWrite(entryID, outcome)
		if queueGains && outcome.imagesAdded && !slices.Contains(f.queue, outcome.setID) {
			f.queue = append(f.queue, outcome.setID)
		}
	}
}

// runStoredRound writes the stored entry into each of its automatic matches and
// removes it, or with none gives it its suggested merge, never naming the
// import's targets.
func (f *mergeFlowState) runStoredRound(entry *ImageSetMongo, automatic, suggested []mergeCandidate) {

	if len(automatic) == 0 {
		f.result.items = append(f.result.items, suggestMerge(f.userID, f.ref, entry.ID, suggested, f.result.targets)...)
		return
	}

	items, outcomes, removed := mergeStoredSetIntoTargets(f.userID, f.ref, entry, automatic)
	f.recordRound(entry.ID, items, outcomes, removed)

	//after its writes, which can carry its holds to the targets
	if removed {
		f.carried.recordSetRemoved(entry)
	}
}

// runQueuedRounds gives each queued set its stored round, breadth first, until
// the queue is empty. A set merged away since it was queued is skipped, and a
// failed read is a Failed item for that set.
func (f *mergeFlowState) runQueuedRounds() {

	for len(f.queue) > 0 {

		id := f.queue[0]
		f.queue = f.queue[1:]

		sets, err := loadOwnedSets(f.userID, []bson.ObjectID{id})
		if err != nil {
			f.result.items = append(f.result.items, notification.FailedItem(f.userID, f.ref, id, fmt.Errorf("%w: %w", notification.ErrLibraryAccess, err)))
			continue
		}

		if len(sets) == 0 {
			continue //merged into another set since it was queued
		}

		automatic, suggested, err := findMergeCandidates(f.userID, storedSetAsPreparedImages(sets[0].Image), id)
		if err != nil {
			f.result.items = append(f.result.items, notification.FailedItem(f.userID, f.ref, id, fmt.Errorf("%w: %w", notification.ErrLibraryAccess, err)))
			continue
		}
		f.runStoredRound(&sets[0], automatic, suggested)
	}
}

// importMergeFlow merges set, an import not stored yet, into userID's library
// (see mergeFlowState), then settles the notifications of the held copies its
// writes moved or reset. err means the lookup failed and nothing was decided.
// The caller holds LockUserSets.
func importMergeFlow(userID, ref string, set *ImageSetMongo, prepared []PreparedImage) (mergeResult, error) {

	automatic, suggested, err := findMergeCandidates(userID, prepared, set.ID)
	if err != nil {
		return mergeResult{}, err
	}

	if len(automatic) == 0 {
		return mergeResult{items: suggestMerge(userID, ref, set.ID, suggested, nil)}, nil
	}

	flow := &mergeFlowState{userID: userID, ref: ref}
	for _, target := range automatic {
		flow.result.targets = append(flow.result.targets, target.set.ID)
	}

	items, outcomes := mergeImportIntoTargets(userID, ref, set, prepared, automatic)
	flow.recordRound(set.ID, items, outcomes, true)

	flow.runQueuedRounds()
	flow.carried.settle(userID)
	return flow.result, nil
}

// localMergeFlow merges the stored set setID into the rest of userID's library
// (see mergeFlowState), then settles the notifications of the held copies its
// writes moved or reset. A failed read is a Failed item. The caller holds
// LockUserSets.
func localMergeFlow(userID, ref string, setID bson.ObjectID) []notification.ItemResult {

	flow := &mergeFlowState{userID: userID, ref: ref, queue: []bson.ObjectID{setID}}
	flow.runQueuedRounds()
	flow.carried.settle(userID)
	return flow.result.items
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
