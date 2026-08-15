package tagging

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var AutoTagsDB *mongo.Collection

var ErrAutoTagNotFound = errors.New("auto tag not found")

// AutoTagEntry is one element of UserAutoTags.Entries, as stored in Mongo.
type AutoTagEntry struct {
	ID             bson.ObjectID `bson:"id" json:"id"`
	Name           string        `bson:"name" json:"name"`
	SrcTagKeyMatch []string      `bson:"src_tag_key_match" json:"srcTagKeyMatch"` // SourceTagDoc.Key values
	Count          int           `bson:"count" json:"count"`                      // image sets currently carrying this AutoTag
}

// UserAutoTags is the AutoTagsDB document shape: one per user.
type UserAutoTags struct {
	UserID  bson.ObjectID  `bson:"user_id" json:"userId"`
	Entries []AutoTagEntry `bson:"entries" json:"entries"`
}

// AutoTagWithMatches is ListAutoTags' response shape, never persisted: like
// AutoTagEntry but with SrcTagKeyMatch resolved to full SourceTagDocs.
type AutoTagWithMatches struct {
	ID      bson.ObjectID  `json:"id"`
	Name    string         `json:"name"`
	Matches []SourceTagDoc `json:"matches"`
	Count   int            `json:"count"`
}

func CreateAutoTag(userID bson.ObjectID, name string, srcTagKeyMatch []string) (bson.ObjectID, error) {
	entry := AutoTagEntry{ID: bson.NewObjectID(), Name: name, SrcTagKeyMatch: srcTagKeyMatch}

	filter := bson.M{"user_id": userID}
	update := bson.M{"$push": bson.M{"entries": entry}}
	if _, err := AutoTagsDB.UpdateOne(context.Background(), filter, update, options.UpdateOne().SetUpsert(true)); err != nil {
		return bson.ObjectID{}, fmt.Errorf("creating auto tag: %w", err)
	}

	if err := applyAutoTagToSets(userID, entry.ID, nil, srcTagKeyMatch); err != nil {
		return bson.ObjectID{}, fmt.Errorf("applying auto tag to existing sets: %w", err)
	}
	return entry.ID, nil
}

func UpdateAutoTag(userID, autoTagID bson.ObjectID, name *string, srcTagKeyMatch []string) error {
	old, err := findAutoTagEntry(userID, autoTagID)
	if err != nil {
		return err
	}

	set := bson.M{}
	if name != nil {
		set["entries.$.name"] = *name
	}
	newSrcTagKeyMatch := old.SrcTagKeyMatch
	if srcTagKeyMatch != nil {
		set["entries.$.src_tag_key_match"] = srcTagKeyMatch
		newSrcTagKeyMatch = srcTagKeyMatch
	}
	if len(set) > 0 {
		filter := bson.M{"user_id": userID, "entries.id": autoTagID}
		if _, err := AutoTagsDB.UpdateOne(context.Background(), filter, bson.M{"$set": set}); err != nil {
			return fmt.Errorf("updating auto tag: %w", err)
		}
	}

	if err := applyAutoTagToSets(userID, autoTagID, old.SrcTagKeyMatch, newSrcTagKeyMatch); err != nil {
		return fmt.Errorf("applying auto tag to existing sets: %w", err)
	}
	return nil
}

func DeleteAutoTag(userID, autoTagID bson.ObjectID) error {
	old, err := findAutoTagEntry(userID, autoTagID)
	if err != nil {
		return err
	}

	filter := bson.M{"user_id": userID}
	update := bson.M{"$pull": bson.M{"entries": bson.M{"id": autoTagID}}}
	if _, err := AutoTagsDB.UpdateOne(context.Background(), filter, update); err != nil {
		return fmt.Errorf("deleting auto tag: %w", err)
	}

	if err := applyAutoTagToSets(userID, autoTagID, old.SrcTagKeyMatch, nil); err != nil {
		return fmt.Errorf("removing auto tag from existing sets: %w", err)
	}
	return nil
}

// AutoTagSummary is {id, name, count} with no SourceTagDoc resolution - for
// autocomplete and for resolving ImageSetMongo.AutoTags IDs to names, where
// AutoTagWithMatches' resolve step would be pure overhead.
type AutoTagSummary struct {
	ID    bson.ObjectID `json:"id"`
	Name  string        `json:"name"`
	Count int           `json:"count"`
}

// ListAutoTagSummaries returns every AutoTag for userID as {id, name,
// count}, with a single read and no SourceTagDoc resolution.
func ListAutoTagSummaries(userID bson.ObjectID) ([]AutoTagSummary, error) {
	doc, err := fetchUserAutoTags(userID)
	if err != nil {
		return nil, err
	}

	results := make([]AutoTagSummary, 0, len(doc.Entries))
	for _, e := range doc.Entries {
		results = append(results, AutoTagSummary{ID: e.ID, Name: e.Name, Count: e.Count})
	}
	return results, nil
}

// ListAutoTags returns every AutoTag for userID with its SrcTagKeyMatch
// resolved to full SourceTagDocs, for the edit-page display. Prefer
// ListAutoTagSummaries wherever the resolved matches aren't actually needed
// - this runs an extra query per call and returns a much larger payload.
func ListAutoTags(userID bson.ObjectID) ([]AutoTagWithMatches, error) {
	doc, err := fetchUserAutoTags(userID)
	if err != nil {
		return nil, err
	}

	var allKeys []string
	for _, e := range doc.Entries {
		allKeys = append(allKeys, e.SrcTagKeyMatch...)
	}
	tagsByKey, err := sourceTagsByKey(allKeys)
	if err != nil {
		return nil, fmt.Errorf("resolving auto tag matches: %w", err)
	}

	results := make([]AutoTagWithMatches, 0, len(doc.Entries))
	for _, e := range doc.Entries {
		matches := make([]SourceTagDoc, 0, len(e.SrcTagKeyMatch))
		for _, key := range e.SrcTagKeyMatch {
			if t, ok := tagsByKey[key]; ok {
				matches = append(matches, t)
			}
		}
		results = append(results, AutoTagWithMatches{ID: e.ID, Name: e.Name, Matches: matches, Count: e.Count})
	}
	return results, nil
}

func fetchUserAutoTags(userID bson.ObjectID) (UserAutoTags, error) {
	var doc UserAutoTags
	err := AutoTagsDB.FindOne(context.Background(), bson.M{"user_id": userID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return UserAutoTags{}, nil
	}
	if err != nil {
		return UserAutoTags{}, fmt.Errorf("fetching auto tags: %w", err)
	}
	return doc, nil
}

func findAutoTagEntry(userID, autoTagID bson.ObjectID) (AutoTagEntry, error) {
	doc, err := fetchUserAutoTags(userID)
	if err != nil {
		return AutoTagEntry{}, err
	}
	for _, e := range doc.Entries {
		if e.ID == autoTagID {
			return e, nil
		}
	}
	return AutoTagEntry{}, ErrAutoTagNotFound
}

// adjustAutoTagCounts applies each entry's delta to its stored Count, in one
// bulk write. Entries with a zero delta are skipped.
func adjustAutoTagCounts(userID bson.ObjectID, deltas map[bson.ObjectID]int) error {
	models := make([]mongo.WriteModel, 0, len(deltas))
	for id, delta := range deltas {
		if delta == 0 {
			continue
		}
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"user_id": userID, "entries.id": id}).
			SetUpdate(bson.M{"$inc": bson.M{"entries.$.count": delta}}))
	}
	if len(models) == 0 {
		return nil
	}
	_, err := AutoTagsDB.BulkWrite(context.Background(), models)
	return err
}

func incrementAutoTagCounts(userID bson.ObjectID, autoTagIDs []bson.ObjectID) error {
	return adjustAutoTagCounts(userID, deltasFor(autoTagIDs, 1))
}

func decrementAutoTagCounts(userID bson.ObjectID, autoTagIDs []bson.ObjectID) error {
	return adjustAutoTagCounts(userID, deltasFor(autoTagIDs, -1))
}

func deltasFor(ids []bson.ObjectID, delta int) map[bson.ObjectID]int {
	deltas := make(map[bson.ObjectID]int, len(ids))
	for _, id := range ids {
		deltas[id] += delta
	}
	return deltas
}

// applyAutoTagToSets re-evaluates autoTagID's membership on every image set
// that could be affected by the change from oldSrcTagKeyMatch to
// newSrcTagKeyMatch (either list may be nil), and applies the resulting
// additions/removals in one bulk write.
func applyAutoTagToSets(userID, autoTagID bson.ObjectID, oldSrcTagKeyMatch, newSrcTagKeyMatch []string) error {
	touchedKeys := union(oldSrcTagKeyMatch, newSrcTagKeyMatch)
	touchedTags, err := sourceTagsByKey(touchedKeys)
	if err != nil {
		return err
	}
	candidateDefaults := make([]string, 0, len(touchedTags))
	for _, t := range touchedTags {
		candidateDefaults = append(candidateDefaults, t.Tag.Default)
	}

	filter := bson.M{
		"kscope_userid": userID.Hex(),
		"$or": bson.A{
			bson.M{"autotags": autoTagID},
			bson.M{"sources.tags.default": bson.M{"$in": candidateDefaults}},
		},
	}
	cursor, err := imageset.Collection.Find(context.Background(), filter)
	if err != nil {
		return err
	}
	defer cursor.Close(context.Background())

	var sets []imageset.ImageSetMongo
	if err := cursor.All(context.Background(), &sets); err != nil {
		return err
	}

	newKeySet := toSet(newSrcTagKeyMatch)
	var models []mongo.WriteModel
	delta := 0
	for _, set := range sets {
		shouldHave := setHasMatch(userID, set.Sources, newKeySet)
		hasIt := slices.Contains(set.AutoTags, autoTagID)
		if shouldHave == hasIt {
			continue
		}
		op := "$pull"
		if shouldHave {
			op = "$addToSet"
			delta++
		} else {
			delta--
		}
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": set.ID}).
			SetUpdate(bson.M{op: bson.M{"autotags": autoTagID}}))
	}
	if len(models) == 0 {
		return nil
	}
	if _, err := imageset.Collection.BulkWrite(context.Background(), models); err != nil {
		return err
	}
	return adjustAutoTagCounts(userID, map[bson.ObjectID]int{autoTagID: delta})
}

func setHasMatch(userID bson.ObjectID, sources []imageset.SourceInfo, srcTagKeySet map[string]bool) bool {
	for _, src := range sources {
		for _, t := range src.Tags {
			if srcTagKeySet[sourceTagKey(userID, src.Name, t.Default)] {
				return true
			}
		}
	}
	return false
}

func union(a, b []string) []string {
	set := make(map[string]struct{}, len(a)+len(b))
	for _, s := range a {
		set[s] = struct{}{}
	}
	for _, s := range b {
		set[s] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for s := range set {
		result = append(result, s)
	}
	return result
}

func toSet(items []string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, s := range items {
		set[s] = true
	}
	return set
}
