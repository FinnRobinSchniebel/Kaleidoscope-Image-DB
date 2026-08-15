package tagging

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// AutoTag is the import-time entry point: records source-tag usage, matches
// against the user's AutoTags, then returns only the matches not already in
// currentAutoTags. A single source tag can match several AutoTags, and a
// single AutoTag can be reached by several source tags (possibly across more
// than one call, e.g. a set with multiple sources) - filtering against
// currentAutoTags is what keeps each AutoTag's usage count incrementing by
// exactly one per set no matter how many of its matches were involved.
func AutoTag(userID, sourceName string, sourceTags []imageset.SourceTag, currentAutoTags []bson.ObjectID) ([]bson.ObjectID, error) {
	uid, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return nil, fmt.Errorf("parsing user id: %w", err)
	}
	//source tag increment
	if err := recordSourceTagUsage(uid, sourceName, sourceTags); err != nil {
		return nil, fmt.Errorf("recording source tag usage: %w", err)
	}
	matched, err := matchAutoTags(uid, sourceName, sourceTags)
	if err != nil {
		return nil, err
	}

	newlyAssigned := make([]bson.ObjectID, 0, len(matched))
	for _, id := range matched {
		if !slices.Contains(currentAutoTags, id) {
			newlyAssigned = append(newlyAssigned, id)
		}
	}
	if err := incrementAutoTagCounts(uid, newlyAssigned); err != nil {
		return nil, fmt.Errorf("recording auto tag usage: %w", err)
	}
	return newlyAssigned, nil
}

// matchAutoTags fetches userID's AutoTags and returns every entry whose
// SrcTagKeyMatch intersects the given source tags' computed keys. No side
// effects.
func matchAutoTags(userID bson.ObjectID, source string, sourceTags []imageset.SourceTag) ([]bson.ObjectID, error) {
	var doc UserAutoTags
	err := AutoTagsDB.FindOne(context.Background(), bson.M{"user_id": userID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetching auto tags: %w", err)
	}

	incoming := make(map[string]bool, len(sourceTags))
	for _, t := range sourceTags {
		incoming[sourceTagKey(userID, source, t.Default)] = true
	}

	var matched []bson.ObjectID
	for _, e := range doc.Entries {
		for _, key := range e.SrcTagKeyMatch {
			if incoming[key] {
				matched = append(matched, e.ID)
				break
			}
		}
	}
	return matched, nil
}

// RecordDeletion undoes the usage counts recorded for a deleted image set:
// source tags from sources, and AutoTags from autoTags.
func RecordDeletion(userID string, sources []imageset.SourceInfo, autoTags []bson.ObjectID) error {
	uid, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return fmt.Errorf("parsing user id: %w", err)
	}
	if err := decrementSourceTagUsage(uid, sources); err != nil {
		return err
	}
	return decrementAutoTagCounts(uid, autoTags)
}

// AutoTagFunc implements imageset.AutoTagger by delegating to this package's free functions.
type AutoTagFunc struct{}

func (AutoTagFunc) AutoTag(userID, sourceName string, sourceTags []imageset.SourceTag, currentAutoTags []bson.ObjectID) ([]bson.ObjectID, error) {
	return AutoTag(userID, sourceName, sourceTags, currentAutoTags)
}

func (AutoTagFunc) RecordDeletion(userID string, sources []imageset.SourceInfo, autoTags []bson.ObjectID) error {
	return RecordDeletion(userID, sources, autoTags)
}
