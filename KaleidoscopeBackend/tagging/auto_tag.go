package tagging

import (
	"context"
	"fmt"
	"log"
	"slices"

	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// findAutoTags returns AutoTags matched by sourceTags that aren't already in currentAutoTags.
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
	return newlyAssigned, nil
}

// autoTag matches newTags, appends new AutoTag ids to set.AutoTags, and rebuilds set.Tags.
func autoTag(userID bson.ObjectID, set *imageset.ImageSetMongo, sourceName string, newTags []imageset.SourceTag) error {
	newIDs, err := findAutoTags(userID, sourceName, newTags, set.AutoTags)
	if err != nil {
		return err
	}
	if len(newIDs) == 0 {
		return nil
	}
	set.AutoTags = append(set.AutoTags, newIDs...)
	return rebuildTagsAndAdjustCounts(userID, set)
}

// ProcessSourceTags is the imageset.AutoTagger.ProcessSourceTags implementation.
// It also recomputes system-computed tags (Lost Media, Untracked - see
// system_tags.go) against set's current Sources on every call, not just
// when fetched contains something new, so callers never need a separate
// call to keep those in sync after a source-tag fetch.
//
// On error, the count writes that already succeeded are reversed and set is
// restored to its state before the call. Translation pushes and system tag
// creation are kept on purpose; a step that fails partway can still leave
// part of its own write applied.
func ProcessSourceTags(userID string, set *imageset.ImageSetMongo, sourceIdx int, fetched []imageset.SourceTag) (err error) {
	defer lockUserTags(userID)()

	uid, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return fmt.Errorf("parsing user id: %w", err)
	}
	src := &set.Sources[sourceIdx]
	srcTags, autoTags, tags := slices.Clone(src.Tags), slices.Clone(set.AutoTags), slices.Clone(set.Tags)

	var added []imageset.SourceTag // non-empty once source tag usage was recorded
	var tagsAfterAutoTag []string   // non-nil once autoTag's count change landed
	defer func() {
		if err == nil {
			return
		}
		if tagsAfterAutoTag != nil {
			if uerr := adjustAutoTagCounts(uid, tagCountDeltas(tagsAfterAutoTag, tags)); uerr != nil {
				log.Printf("------ Warning: undoing auto tag counts for user [%s]: %s ------", userID, uerr)
			}
		}
		if len(added) > 0 {
			if uerr := decrementSourceTagUsage(uid, []imageset.SourceInfo{{Name: src.Name, Tags: added}}); uerr != nil {
				log.Printf("------ Warning: undoing source tag counts for user [%s]: %s ------", userID, uerr)
			}
		}
		src.Tags, set.AutoTags, set.Tags = srcTags, autoTags, tags
	}()

	added, err = reconcileSourceTags(uid, src, fetched)
	if err != nil {
		return err
	}
	if len(added) > 0 {
		if err = autoTag(uid, set, src.Name, added); err != nil {
			return err
		}
		tagsAfterAutoTag = slices.Clone(set.Tags)
		if tagsAfterAutoTag == nil {
			tagsAfterAutoTag = []string{}
		}
	}
	// last count write, so nothing after it can need undoing
	return RecomputeSystemTags(userID, set)
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
// source tags from sources, and AutoTags from tags.
func RecordDeletion(userID string, sources []imageset.SourceInfo, tags []string) error {
	uid, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return fmt.Errorf("parsing user id: %w", err)
	}
	if err := decrementSourceTagUsage(uid, sources); err != nil {
		return err
	}
	return adjustAutoTagCounts(uid, tagCountDeltas(tags, nil))
}

// AutoTagFunc implements imageset.AutoTagger by delegating to this package's free functions.
type AutoTagFunc struct{}

func (AutoTagFunc) ProcessSourceTags(userID string, set *imageset.ImageSetMongo, sourceIdx int, fetched []imageset.SourceTag) error {
	return ProcessSourceTags(userID, set, sourceIdx, fetched)
}

func (AutoTagFunc) RecordDeletion(userID string, sources []imageset.SourceInfo, tags []string) error {
	return RecordDeletion(userID, sources, tags)
}

func (AutoTagFunc) RecomputeSystemTags(userID string, set *imageset.ImageSetMongo) error {
	return RecomputeSystemTags(userID, set)
}

func (AutoTagFunc) RefreshUntaggedCount(userID string) error {
	uid, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return fmt.Errorf("parsing user id: %w", err)
	}
	return refreshUntaggedCount(uid)
}

func (AutoTagFunc) ResolveTagTerm(userID string, term string) ([]string, bool, error) {
	return ResolveTagTerm(userID, term)
}
