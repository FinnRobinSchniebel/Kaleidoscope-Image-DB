package tagging

import (
	"context"
	"fmt"
	"slices"

	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
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

// matchAutoTags returns the IDs of every AutoTag whose SrcTagKeyMatch
// intersects the given source tags' computed keys, via an indexed query
// (userId + src_tag_key_match). No side effects.
func matchAutoTags(userID bson.ObjectID, source string, sourceTags []imageset.SourceTag) ([]bson.ObjectID, error) {
	keys := make([]string, len(sourceTags))
	for i, t := range sourceTags {
		keys[i] = sourceTagKey(userID, source, t.Default)
	}

	cursor, err := AutoTagsDB.Find(context.Background(),
		bson.M{"user_id": userID, "src_tag_key_match": bson.M{"$in": keys}},
		options.Find().SetProjection(bson.M{"_id": 1}),
	)
	if err != nil {
		return nil, fmt.Errorf("matching auto tags: %w", err)
	}
	defer cursor.Close(context.Background())

	var docs []struct {
		ID bson.ObjectID `bson:"_id"`
	}
	if err := cursor.All(context.Background(), &docs); err != nil {
		return nil, fmt.Errorf("matching auto tags: %w", err)
	}

	matched := make([]bson.ObjectID, len(docs))
	for i, d := range docs {
		matched[i] = d.ID
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

func (AutoTagFunc) ReconcileTranslations(userID, sourceName string, sourceTags []imageset.SourceTag) ([]imageset.SourceTag, error) {
	return ReconcileTranslations(userID, sourceName, sourceTags)
}
