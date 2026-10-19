package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"fmt"
	"slices"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// checkFullSet combines setID, which just gained images, with any set of
// userID's it duplicates, extends or is a subset of (the older set survives),
// checking the survivor again each time. The partial and similar matches left
// get one suggested merge, also naming the sets holding lookupSources (plus the
// set's own sources once a combine added images to it); protected sets are never
// suggested. A set that no longer exists returns nothing. The caller holds
// LockUserSets.
func checkFullSet(userID, ref string, setID bson.ObjectID, lookupSources []SourceInfo, protected []bson.ObjectID) (items []notification.ItemResult, finalID bson.ObjectID, absorbed []bson.ObjectID) {

	finalID = setID
	ownSourcesChanged := false
	for {

		sets, err := loadOwnedSets(userID, []bson.ObjectID{finalID})
		if err != nil {
			return append(items, notification.FailedItem(userID, ref, finalID, fmt.Errorf("%w: %w", notification.ErrLibraryAccess, err))), finalID, absorbed
		}

		if len(sets) == 0 {
			return items, finalID, absorbed
		}
		set := &sets[0]

		incoming, err := asPrepared(set.Image)
		if err != nil {
			return append(items, notification.FailedItem(userID, ref, set.ID, fmt.Errorf("%w: %w", notification.ErrLibraryAccess, err))), finalID, absorbed
		}

		matches, err := findImageMatches(userID, incoming)
		if err != nil {
			return append(items, notification.FailedItem(userID, ref, set.ID, fmt.Errorf("%w: %w", notification.ErrLibraryAccess, err))), finalID, absorbed
		}

		for i := range matches {
			matches[i] = slices.DeleteFunc(matches[i], func(hit HashMatch) bool { return hit.SetID == set.ID })
		}

		candidateSets, err := loadOwnedSets(userID, findCandidates(matches, len(incoming)))
		if err != nil {
			return append(items, notification.FailedItem(userID, ref, set.ID, fmt.Errorf("%w: %w", notification.ErrLibraryAccess, err))), finalID, absorbed
		}

		var automatic *ImageSetMongo
		var suggested []candidateMatch
		for i := range candidateSets {

			candidate := classifyCandidate(&candidateSets[i], incoming, matches)
			switch candidate.scenario {
			case scenarioDuplicate, scenarioAddition, scenarioLess:
				if automatic == nil {
					automatic = candidate.set
				}
			default:
				suggested = append(suggested, candidate)
			}
		}

		if automatic == nil {
			sources := lookupSources
			if ownSourcesChanged {
				sources = slices.Concat(lookupSources, set.Sources)
			}
			return append(items, suggestMerge(userID, ref, set.ID, suggested, sources, protected)...), finalID, absorbed
		}

		survivor, other := set, automatic
		if automatic.DateAdded.Before(set.DateAdded) || (automatic.DateAdded.Equal(set.DateAdded) && automatic.ID.Hex() < set.ID.Hex()) {
			survivor, other = automatic, set
		}

		item, imagesChanged := combineSets(userID, ref, survivor, other)
		items = append(items, item)
		if item.Kind == notification.ItemFailed {
			return items, finalID, absorbed
		}
		absorbed = append(absorbed, other.ID)
		finalID = survivor.ID

		switch {
		case survivor == set:
			//more sets may relate to it: check it again
			ownSourcesChanged = ownSourcesChanged || imagesChanged
		case imagesChanged:
			ownSourcesChanged = true
		default:
			//it fully collapsed into an older set, whose relations were already found; only the import's source is new there
			return append(items, suggestMerge(userID, ref, finalID, nil, lookupSources, protected)...), finalID, absorbed
		}
	}
}
