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
	ID      bson.ObjectID `bson:"id" json:"id"`
	Name    string        `bson:"name" json:"name"`
	Matches []string      `bson:"matches" json:"matches"` // SourceTagDoc._id refs
	Count   int           `bson:"count" json:"count"`     // image sets currently carrying this AutoTag
}

// UserAutoTags is the AutoTagsDB document shape: one per user.
type UserAutoTags struct {
	UserID  bson.ObjectID  `bson:"user_id" json:"userId"`
	Entries []AutoTagEntry `bson:"entries" json:"entries"`
}

// AutoTagWithMatches is ListAutoTags' response shape, never persisted: like
// AutoTagEntry but with Matches resolved to full SourceTagDocs.
type AutoTagWithMatches struct {
	ID      bson.ObjectID  `json:"id"`
	Name    string         `json:"name"`
	Matches []SourceTagDoc `json:"matches"`
	Count   int            `json:"count"`
}

func CreateAutoTag(userID bson.ObjectID, name string, matches []string) (bson.ObjectID, error) {
	entry := AutoTagEntry{ID: bson.NewObjectID(), Name: name, Matches: matches}

	filter := bson.M{"user_id": userID}
	update := bson.M{"$push": bson.M{"entries": entry}}
	if _, err := AutoTagsDB.UpdateOne(context.Background(), filter, update, options.UpdateOne().SetUpsert(true)); err != nil {
		return bson.ObjectID{}, fmt.Errorf("creating auto tag: %w", err)
	}

	if err := applyAutoTagToSets(userID, entry.ID, nil, matches); err != nil {
		return bson.ObjectID{}, fmt.Errorf("applying auto tag to existing sets: %w", err)
	}
	return entry.ID, nil
}

func UpdateAutoTag(userID, autoTagID bson.ObjectID, name *string, matches []string) error {
	old, err := findAutoTagEntry(userID, autoTagID)
	if err != nil {
		return err
	}

	set := bson.M{}
	if name != nil {
		set["entries.$.name"] = *name
	}
	newMatches := old.Matches
	if matches != nil {
		set["entries.$.matches"] = matches
		newMatches = matches
	}
	if len(set) > 0 {
		filter := bson.M{"user_id": userID, "entries.id": autoTagID}
		if _, err := AutoTagsDB.UpdateOne(context.Background(), filter, bson.M{"$set": set}); err != nil {
			return fmt.Errorf("updating auto tag: %w", err)
		}
	}

	if err := applyAutoTagToSets(userID, autoTagID, old.Matches, newMatches); err != nil {
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

	if err := applyAutoTagToSets(userID, autoTagID, old.Matches, nil); err != nil {
		return fmt.Errorf("removing auto tag from existing sets: %w", err)
	}
	return nil
}

// ListAutoTags returns every AutoTag for userID with its matches resolved to
// full SourceTagDocs, for the edit-page display.
func ListAutoTags(userID bson.ObjectID) ([]AutoTagWithMatches, error) {
	var doc UserAutoTags
	err := AutoTagsDB.FindOne(context.Background(), bson.M{"user_id": userID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetching auto tags: %w", err)
	}

	var allRefs []string
	for _, e := range doc.Entries {
		allRefs = append(allRefs, e.Matches...)
	}
	tagsByID, err := sourceTagsByID(allRefs)
	if err != nil {
		return nil, fmt.Errorf("resolving auto tag matches: %w", err)
	}

	results := make([]AutoTagWithMatches, 0, len(doc.Entries))
	for _, e := range doc.Entries {
		matches := make([]SourceTagDoc, 0, len(e.Matches))
		for _, ref := range e.Matches {
			if t, ok := tagsByID[ref]; ok {
				matches = append(matches, t)
			}
		}
		results = append(results, AutoTagWithMatches{ID: e.ID, Name: e.Name, Matches: matches, Count: e.Count})
	}
	return results, nil
}

func findAutoTagEntry(userID, autoTagID bson.ObjectID) (AutoTagEntry, error) {
	var doc UserAutoTags
	err := AutoTagsDB.FindOne(context.Background(), bson.M{"user_id": userID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return AutoTagEntry{}, ErrAutoTagNotFound
	}
	if err != nil {
		return AutoTagEntry{}, fmt.Errorf("fetching auto tags: %w", err)
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

func sourceTagsByID(ids []string) (map[string]SourceTagDoc, error) {
	result := make(map[string]SourceTagDoc, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	cursor, err := SourceTagsDB.Find(context.Background(), bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())

	var docs []SourceTagDoc
	if err := cursor.All(context.Background(), &docs); err != nil {
		return nil, err
	}
	for _, d := range docs {
		result[d.ID] = d
	}
	return result, nil
}

// applyAutoTagToSets re-evaluates autoTagID's membership on every image set
// that could be affected by the change from oldMatches to newMatches (either
// list may be nil), and applies the resulting additions/removals in one bulk
// write.
func applyAutoTagToSets(userID, autoTagID bson.ObjectID, oldMatches, newMatches []string) error {
	touchedRefs := union(oldMatches, newMatches)
	touchedTags, err := sourceTagsByID(touchedRefs)
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

	newMatchSet := toSet(newMatches)
	var models []mongo.WriteModel
	delta := 0
	for _, set := range sets {
		shouldHave := setHasMatch(userID, set.Sources, newMatchSet)
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

func setHasMatch(userID bson.ObjectID, sources []imageset.SourceInfo, matchSet map[string]bool) bool {
	for _, src := range sources {
		for _, t := range src.Tags {
			if matchSet[sourceTagID(userID, src.Name, t.Default)] {
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
