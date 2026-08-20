package tagging

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var AutoTagsDB *mongo.Collection

var ErrAutoTagNotFound = errors.New("auto tag not found")
var ErrAutoTagNameExists = errors.New("auto tag name already exists")
var ErrAutoTagNameReserved = errors.New("auto tag name is reserved for a system tag")
var ErrSystemAutoTagImmutable = errors.New("system auto tags cannot be edited or deleted")

// Reserved names for system-computed AutoTags (see system_tags.go). No user
// AutoTag may use these names - CreateAutoTag rejects them so the system tag
// can always be lazily created later without colliding with the unique
// {user_id, name} index.
const (
	lostMediaTagName = "Lost Media"
	untrackedTagName = "Untracked"
)

var systemAutoTagNames = []string{lostMediaTagName, untrackedTagName}

// AutoTagDoc is one AutoTag, one document per tag (mirrors SourceTagDoc).
type AutoTagDoc struct {
	ID             bson.ObjectID `bson:"_id" json:"id"`
	UserID         bson.ObjectID `bson:"user_id" json:"userId"`
	Name           string        `bson:"name" json:"name"`
	SrcTagKeyMatch []string      `bson:"src_tag_key_match" json:"srcTagKeyMatch"` // SourceTagDoc.Key values
	Count          int           `bson:"count" json:"count"`                      // image sets currently carrying this AutoTag
	System         bool          `bson:"system,omitempty" json:"system,omitempty"` // true for a computed tag (see system_tags.go), never user-editable
}

// AutoTagWithMatches is ListAutoTags' response shape, never persisted: like
// AutoTagDoc but with SrcTagKeyMatch resolved to full SourceTagDocs.
type AutoTagWithMatches struct {
	ID      bson.ObjectID  `json:"id"`
	Name    string         `json:"name"`
	Matches []SourceTagDoc `json:"matches"`
	Count   int            `json:"count"`
	System  bool           `json:"system,omitempty"`
}

// AutoTagSummary is {id, name, count} with no SourceTagDoc resolution - for
// autocomplete and for resolving ImageSetMongo.AutoTags IDs to names, where
// AutoTagWithMatches' resolve step would be pure overhead.
type AutoTagSummary struct {
	ID     bson.ObjectID `bson:"_id" json:"id"`
	Name   string        `bson:"name" json:"name"`
	Count  int           `bson:"count" json:"count"`
	System bool          `bson:"system,omitempty" json:"system,omitempty"`
}

func CreateAutoTag(userID bson.ObjectID, name string, srcTagKeyMatch []string) (bson.ObjectID, error) {
	for _, reserved := range systemAutoTagNames {
		if imageset.NormalizeTagText(name) == imageset.NormalizeTagText(reserved) {
			return bson.ObjectID{}, ErrAutoTagNameReserved
		}
	}

	doc := AutoTagDoc{ID: bson.NewObjectID(), UserID: userID, Name: name, SrcTagKeyMatch: srcTagKeyMatch}
	if _, err := AutoTagsDB.InsertOne(context.Background(), doc); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return bson.ObjectID{}, ErrAutoTagNameExists
		}
		return bson.ObjectID{}, fmt.Errorf("creating auto tag: %w", err)
	}

	if err := applyAutoTagToSets(userID, doc.ID, nil, srcTagKeyMatch); err != nil {
		return bson.ObjectID{}, fmt.Errorf("applying auto tag to existing sets: %w", err)
	}
	return doc.ID, nil
}

// UpdateAutoTag only changes fields whose pointer/slice is non-nil; a nil
// srcTagKeyMatch leaves it unchanged, a non-nil empty slice clears it.
// Rejects system-computed tags (see system_tags.go): applyAutoTagToSets
// below re-evaluates membership purely from SrcTagKeyMatch, which is always
// empty for a system tag, so letting this through would silently strip the
// tag from every set that currently has it.
func UpdateAutoTag(userID, autoTagID bson.ObjectID, name *string, srcTagKeyMatch []string) error {
	var existing AutoTagDoc
	if err := AutoTagsDB.FindOne(context.Background(), bson.M{"_id": autoTagID, "user_id": userID}).Decode(&existing); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ErrAutoTagNotFound
		}
		return fmt.Errorf("updating auto tag: %w", err)
	}
	if existing.System {
		return ErrSystemAutoTagImmutable
	}

	set := bson.M{}
	if name != nil {
		set["name"] = *name
	}
	if srcTagKeyMatch != nil {
		set["src_tag_key_match"] = srcTagKeyMatch
	}

	filter := bson.M{"_id": autoTagID, "user_id": userID}
	var old AutoTagDoc
	var err error
	if len(set) > 0 {
		opts := options.FindOneAndUpdate().SetReturnDocument(options.Before)
		err = AutoTagsDB.FindOneAndUpdate(context.Background(), filter, bson.M{"$set": set}, opts).Decode(&old)
	} else {
		err = AutoTagsDB.FindOne(context.Background(), filter).Decode(&old)
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrAutoTagNotFound
	}
	if mongo.IsDuplicateKeyError(err) {
		return ErrAutoTagNameExists
	}
	if err != nil {
		return fmt.Errorf("updating auto tag: %w", err)
	}

	newSrcTagKeyMatch := old.SrcTagKeyMatch
	if srcTagKeyMatch != nil {
		newSrcTagKeyMatch = srcTagKeyMatch
	}
	if err := applyAutoTagToSets(userID, autoTagID, old.SrcTagKeyMatch, newSrcTagKeyMatch); err != nil {
		return fmt.Errorf("applying auto tag to existing sets: %w", err)
	}
	return nil
}

// DeleteAutoTag rejects system-computed tags (see system_tags.go): deleting
// the doc would leave its id dangling in every set's AutoTags/Tags, and the
// next recompute would just mint a replacement with a fresh id anyway.
func DeleteAutoTag(userID, autoTagID bson.ObjectID) error {
	var existing AutoTagDoc
	if err := AutoTagsDB.FindOne(context.Background(), bson.M{"_id": autoTagID, "user_id": userID}).Decode(&existing); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ErrAutoTagNotFound
		}
		return fmt.Errorf("deleting auto tag: %w", err)
	}
	if existing.System {
		return ErrSystemAutoTagImmutable
	}

	filter := bson.M{"_id": autoTagID, "user_id": userID}
	var old AutoTagDoc
	err := AutoTagsDB.FindOneAndDelete(context.Background(), filter).Decode(&old)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrAutoTagNotFound
	}
	if err != nil {
		return fmt.Errorf("deleting auto tag: %w", err)
	}

	if err := applyAutoTagToSets(userID, autoTagID, old.SrcTagKeyMatch, nil); err != nil {
		return fmt.Errorf("removing auto tag from existing sets: %w", err)
	}
	return nil
}

// ensureAutoTagIndexes creates the AutoTagsDB indexes. Owned here since
// AutoTagsDB is this file's collection - callers outside this file must not
// touch AutoTagsDB's indexes directly.
func ensureAutoTagIndexes(ctx context.Context) error {
	_, err := AutoTagsDB.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "name", Value: 1}},
			Options: options.Index().SetUnique(true).SetCollation(&options.Collation{Locale: "en", Strength: 2}),
		},
		{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "src_tag_key_match", Value: 1}}},
	})
	return err
}

// findAutoTagsByNames returns existing AutoTagDocs for userID matching any
// of names, keyed by Name. Missing names are simply absent from the result.
func findAutoTagsByNames(userID bson.ObjectID, names []string) (map[string]AutoTagDoc, error) {
	cursor, err := AutoTagsDB.Find(context.Background(), bson.M{"user_id": userID, "name": bson.M{"$in": names}})
	if err != nil {
		return nil, fmt.Errorf("finding auto tags by name: %w", err)
	}
	defer cursor.Close(context.Background())

	var docs []AutoTagDoc
	if err := cursor.All(context.Background(), &docs); err != nil {
		return nil, fmt.Errorf("finding auto tags by name: %w", err)
	}
	result := make(map[string]AutoTagDoc, len(docs))
	for _, d := range docs {
		result[d.Name] = d
	}
	return result, nil
}

// insertAutoTag inserts doc as-is (caller sets ID/UserID/Name/System, etc.).
// Returns ErrAutoTagNameExists on a name collision, same translation
// CreateAutoTag already does.
func insertAutoTag(doc AutoTagDoc) error {
	if _, err := AutoTagsDB.InsertOne(context.Background(), doc); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrAutoTagNameExists
		}
		return fmt.Errorf("inserting auto tag: %w", err)
	}
	return nil
}

// findAutoTagByName returns the single AutoTagDoc for userID with the given
// name. Returns ErrAutoTagNotFound if none exists.
func findAutoTagByName(userID bson.ObjectID, name string) (AutoTagDoc, error) {
	var doc AutoTagDoc
	err := AutoTagsDB.FindOne(context.Background(), bson.M{"user_id": userID, "name": name}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return AutoTagDoc{}, ErrAutoTagNotFound
	}
	if err != nil {
		return AutoTagDoc{}, fmt.Errorf("finding auto tag by name: %w", err)
	}
	return doc, nil
}

// ListAutoTagSummaries returns AutoTags for userID as {id, name, count},
// sorted by count descending. prefix filters by name when non-empty; limit
// truncates the result when > 0, 0 means unlimited. The query projects out
// src_tag_key_match, so it's never pulled over the wire here.
func ListAutoTagSummaries(userID bson.ObjectID, prefix string, limit int) ([]AutoTagSummary, error) {
	filter := bson.M{"user_id": userID}
	if prefix != "" {
		filter["name"] = bson.M{"$regex": "^" + regexp.QuoteMeta(strings.TrimSpace(prefix)), "$options": "i"}
	}
	opts := options.Find().SetProjection(bson.M{"name": 1, "count": 1, "system": 1}).SetSort(bson.D{{Key: "count", Value: -1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}

	cursor, err := AutoTagsDB.Find(context.Background(), filter, opts)
	if err != nil {
		return nil, fmt.Errorf("listing auto tags: %w", err)
	}
	defer cursor.Close(context.Background())

	var results []AutoTagSummary
	if err := cursor.All(context.Background(), &results); err != nil {
		return nil, fmt.Errorf("listing auto tags: %w", err)
	}
	return results, nil
}

// AutoTagSummariesByID resolves specific AutoTag IDs to {id, name, count}.
// IDs that don't exist (or belong to another user) are silently omitted.
func AutoTagSummariesByID(userID bson.ObjectID, ids []bson.ObjectID) ([]AutoTagSummary, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	filter := bson.M{"user_id": userID, "_id": bson.M{"$in": ids}}
	opts := options.Find().SetProjection(bson.M{"name": 1, "count": 1, "system": 1})

	cursor, err := AutoTagsDB.Find(context.Background(), filter, opts)
	if err != nil {
		return nil, fmt.Errorf("fetching auto tags: %w", err)
	}
	defer cursor.Close(context.Background())

	var results []AutoTagSummary
	if err := cursor.All(context.Background(), &results); err != nil {
		return nil, fmt.Errorf("fetching auto tags: %w", err)
	}
	return results, nil
}

// ListAutoTags returns every AutoTag for userID with its SrcTagKeyMatch
// resolved to full SourceTagDocs, for the edit-page display. Prefer
// ListAutoTagSummaries wherever the resolved matches aren't actually needed
// - this runs an extra query per call and returns a much larger payload.
func ListAutoTags(userID bson.ObjectID) ([]AutoTagWithMatches, error) {
	cursor, err := AutoTagsDB.Find(context.Background(), bson.M{"user_id": userID})
	if err != nil {
		return nil, fmt.Errorf("fetching auto tags: %w", err)
	}
	defer cursor.Close(context.Background())

	var docs []AutoTagDoc
	if err := cursor.All(context.Background(), &docs); err != nil {
		return nil, fmt.Errorf("fetching auto tags: %w", err)
	}

	var allKeys []string
	for _, d := range docs {
		allKeys = append(allKeys, d.SrcTagKeyMatch...)
	}
	tagsByKey, err := sourceTagsByKey(userID, allKeys)
	if err != nil {
		return nil, fmt.Errorf("resolving auto tag matches: %w", err)
	}

	results := make([]AutoTagWithMatches, 0, len(docs))
	for _, d := range docs {
		matches := make([]SourceTagDoc, 0, len(d.SrcTagKeyMatch))
		for _, key := range d.SrcTagKeyMatch {
			if t, ok := tagsByKey[key]; ok {
				matches = append(matches, t)
			}
		}
		results = append(results, AutoTagWithMatches{ID: d.ID, Name: d.Name, Matches: matches, Count: d.Count, System: d.System})
	}
	return results, nil
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
			SetFilter(bson.M{"_id": id, "user_id": userID}).
			SetUpdate(bson.M{"$inc": bson.M{"count": delta}}))
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
	touchedTags, err := sourceTagsByKey(userID, touchedKeys)
	if err != nil {
		return err
	}

	// case/whitespace-insensitive: normalizeTag treats those as the same tag
	orClauses := bson.A{bson.M{"autotags": autoTagID}}
	for _, t := range touchedTags {
		pattern := "^" + regexp.QuoteMeta(strings.TrimSpace(t.Tag.Default)) + "$"
		orClauses = append(orClauses, bson.M{"sources.tags.default": bson.M{"$regex": pattern, "$options": "i"}})
	}

	filter := bson.M{
		"kscope_userid": userID.Hex(),
		"$or":           orClauses,
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
		newAutoTags := slices.Clone(set.AutoTags)
		if shouldHave {
			op = "$addToSet"
			newAutoTags = append(newAutoTags, autoTagID)
			delta++
		} else {
			newAutoTags = slices.DeleteFunc(newAutoTags, func(id bson.ObjectID) bool { return id == autoTagID })
			delta--
		}
		// TODO: tags is computed from this set's state as of the Find above, so a
		// concurrent write to this set's autotags/tag_rule_overrides between that
		// Find and this BulkWrite can get overwritten with a stale value here,
		// unlike the atomic $addToSet/$pull alongside it. Narrow window, self-heals
		// on the next relevant write; low priority, see read-modify-write-race-review skill.
		tags := ApplyTagRuleOverrides(newAutoTags, set.TagRuleOverrides)
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": set.ID}).
			SetUpdate(bson.M{op: bson.M{"autotags": autoTagID}, "$set": bson.M{"tags": tags}}))
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
