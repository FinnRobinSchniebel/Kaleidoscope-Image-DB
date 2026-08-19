package tagging

import (
	"context"
	"fmt"
	"slices"

	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// findAutoTags returns AutoTags matched by sourceTags that aren't already
// in currentAutoTags, incrementing each returned AutoTag's count once.
func findAutoTags(userID bson.ObjectID, source string, sourceTags []imageset.SourceTag, currentAutoTags []bson.ObjectID) ([]bson.ObjectID, error) {
	matched, err := matchAutoTags(userID, source, sourceTags)
	if err != nil {
		return nil, err
	}

	newlyAssigned := make([]bson.ObjectID, 0, len(matched))
	for _, id := range matched {
		if !slices.Contains(currentAutoTags, id) {
			newlyAssigned = append(newlyAssigned, id)
		}
	}
	if err := incrementAutoTagCounts(userID, newlyAssigned); err != nil {
		return nil, fmt.Errorf("recording auto tag usage: %w", err)
	}
	return newlyAssigned, nil
}

// applyAutoTags matches newTags, appends new AutoTag ids to set.AutoTags,
// and rebuilds set.Tags.
func applyAutoTags(userID bson.ObjectID, set *imageset.ImageSetMongo, sourceName string, newTags []imageset.SourceTag) error {
	newIDs, err := findAutoTags(userID, sourceName, newTags, set.AutoTags)
	if err != nil {
		return err
	}
	if len(newIDs) == 0 {
		return nil
	}
	set.AutoTags = append(set.AutoTags, newIDs...)
	buildTags(set)
	return nil
}

// ProcessSourceTags is the imageset.AutoTagger.ProcessSourceTags implementation.
func ProcessSourceTags(userID string, set *imageset.ImageSetMongo, sourceIdx int, fetched []imageset.SourceTag) error {
	uid, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return fmt.Errorf("parsing user id: %w", err)
	}
	src := &set.Sources[sourceIdx]
	added, err := reconcileSourceTags(uid, src, fetched)
	if err != nil {
		return err
	}
	if len(added) == 0 {
		return nil
	}
	return applyAutoTags(uid, set, src.Name, added)
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

func (AutoTagFunc) ProcessSourceTags(userID string, set *imageset.ImageSetMongo, sourceIdx int, fetched []imageset.SourceTag) error {
	return ProcessSourceTags(userID, set, sourceIdx, fetched)
}

func (AutoTagFunc) RecordDeletion(userID string, sources []imageset.SourceInfo, autoTags []bson.ObjectID) error {
	return RecordDeletion(userID, sources, autoTags)
}
