package tagging

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"

	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var SourceTagsDB *mongo.Collection

type SourceTagDoc struct {
	Key    string             `bson:"_id" json:"key"`
	UserID bson.ObjectID      `bson:"userId" json:"userId"`
	Source string             `bson:"source" json:"source"`
	Tag    imageset.SourceTag `bson:"tag" json:"tag"`
	Count  int                `bson:"count" json:"count"`
}

func normalizeTag(tag string) string {
	return imageset.NormalizeTagText(tag)
}

// sourceTagKey must be used everywhere a SourceTag is written or looked up;
// it is the doc's _id, so a mismatched key here silently creates a duplicate
// rather than finding the existing tag. A plain deterministic string rather
// than a bson.ObjectID, deliberately: unlike AutoTagDoc.ID, nothing ever
// needs to look this up by anything other than (userID, source, default).
func sourceTagKey(userID bson.ObjectID, source, tagDefault string) string {
	return userID.Hex() + "::" + source + "::" + normalizeTag(tagDefault)
}

// userTagLocks guards SourceTagsDB writes against RegatherSourceTags: its
// scan-then-$set isn't atomic like the incremental $inc/$set paths, so a
// concurrent write could otherwise be silently overwritten by a stale scan.
var userTagLocks sync.Map // userID hex -> *sync.Mutex

// lockUserTags acquires the per-user lock, returning the func to release it.
func lockUserTags(userID string) func() {
	v, _ := userTagLocks.LoadOrStore(userID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func EnsureIndexes(ctx context.Context) error {
	if _, err := SourceTagsDB.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "userId", Value: 1}, {Key: "tag.default", Value: 1}}},
		{Keys: bson.D{{Key: "userId", Value: 1}, {Key: "tag.en", Value: 1}}},
	}); err != nil {
		return err
	}
	return ensureAutoTagIndexes(ctx)
}

// recordSourceTagUsage upserts one SourceTagDoc per unique tag, incrementing
// Count once each even if sourceTags repeats the same tag.
func recordSourceTagUsage(userID bson.ObjectID, source string, sourceTags []imageset.SourceTag) error {
	if len(sourceTags) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(sourceTags))
	models := make([]mongo.WriteModel, 0, len(sourceTags))
	for _, t := range sourceTags {
		key := sourceTagKey(userID, source, t.Default)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": key}).
			SetUpdate(bson.M{
				"$inc":         bson.M{"count": 1},
				"$setOnInsert": bson.M{"userId": userID, "source": source, "tag": t},
			}).
			SetUpsert(true))
	}
	_, err := SourceTagsDB.BulkWrite(context.Background(), models)
	return err
}

// decrementSourceTagUsage lowers Count for every tag across sources, clamped
// at 0. Docs are kept at 0 rather than deleted so AutoTag matches referencing
// them never dangle.
func decrementSourceTagUsage(userID bson.ObjectID, sources []imageset.SourceInfo) error {
	var models []mongo.WriteModel
	for _, src := range sources {
		for _, t := range src.Tags {
			pipeline := mongo.Pipeline{
				bson.D{{Key: "$set", Value: bson.D{{Key: "count", Value: bson.D{{Key: "$max", Value: bson.A{
					bson.D{{Key: "$subtract", Value: bson.A{"$count", 1}}}, 0,
				}}}}}}},
			}
			models = append(models, mongo.NewUpdateOneModel().
				SetFilter(bson.M{"_id": sourceTagKey(userID, src.Name, t.Default)}).
				SetUpdate(pipeline))
		}
	}
	if len(models) == 0 {
		return nil
	}
	_, err := SourceTagsDB.BulkWrite(context.Background(), models)
	return err
}

// SearchSourceTags returns up to limit tags whose tag.default or tag.<lang>
// starts with prefix, sorted by Count descending. source and lang filter
// when non-empty.
func SearchSourceTags(userID bson.ObjectID, source, lang, prefix string, limit int) ([]SourceTagDoc, error) {
	pattern := "^" + regexp.QuoteMeta(strings.TrimSpace(prefix))
	fields := bson.A{"tag.default"}
	if lang != "" && lang != "default" {
		fields = append(fields, "tag."+lang)
	}
	or := make(bson.A, 0, len(fields))
	for _, f := range fields {
		or = append(or, bson.M{f.(string): bson.M{"$regex": pattern, "$options": "i"}})
	}

	filter := bson.M{"userId": userID, "$or": or}
	if source != "" {
		filter["source"] = source
	}

	opts := options.Find().SetSort(bson.D{{Key: "count", Value: -1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}

	cursor, err := SourceTagsDB.Find(context.Background(), filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())

	var results []SourceTagDoc
	if err := cursor.All(context.Background(), &results); err != nil {
		return nil, err
	}
	return results, nil
}

// reconcileTranslation resolves one language's value against what's
// already known, and reports whether fetched should be pushed as an
// update. Adding a second language field later means one more call to
// this plus one more field copy in ReconcileTranslations - this decision
// logic itself doesn't change.
func reconcileTranslation(fetched, known string) (resolved string, needsUpdate bool) {
	switch {
	case fetched == "":
		return known, false
	case fetched == known:
		return fetched, false
	default:
		return fetched, true
	}
}

// reconcileTranslations pushes a changed EN to SourceTagsDB and every
// image set carrying the tag. Returns fetched with EN replaced by the
// resolved value.
func reconcileTranslations(userID bson.ObjectID, sourceName string, fetched []imageset.SourceTag) ([]imageset.SourceTag, error) {
	if len(fetched) == 0 {
		return fetched, nil
	}

	keys := make([]string, len(fetched))
	for i, t := range fetched {
		keys[i] = sourceTagKey(userID, sourceName, t.Default)
	}
	known, err := sourceTagsByKey(userID, keys)
	if err != nil {
		return nil, fmt.Errorf("looking up known translations: %w", err)
	}

	resolved := make([]imageset.SourceTag, len(fetched))
	var toUpdate []imageset.SourceTag
	for i, t := range fetched {
		resolvedEN, needsUpdate := reconcileTranslation(t.EN, known[keys[i]].Tag.EN)
		resolved[i] = t
		resolved[i].EN = resolvedEN
		if needsUpdate {
			toUpdate = append(toUpdate, resolved[i])
		}
	}

	if len(toUpdate) > 0 {
		if err := updateSourceTagTranslation(userID, sourceName, toUpdate); err != nil {
			return nil, fmt.Errorf("updating source tag translation: %w", err)
		}
		if err := imageset.UpdateTagTranslations(userID.Hex(), sourceName, toUpdate); err != nil {
			return nil, fmt.Errorf("propagating source tag translation: %w", err)
		}
	}
	return resolved, nil
}

// reconcileSourceTags merges fetched's reconciled translations into
// src.Tags and records usage for tags new to src.Tags as it stood before
// the call. Returns just the newly-added tags.
func reconcileSourceTags(userID bson.ObjectID, src *imageset.SourceInfo, fetched []imageset.SourceTag) (added []imageset.SourceTag, err error) {
	reconciled, err := reconcileTranslations(userID, src.Name, fetched)
	if err != nil {
		return nil, fmt.Errorf("reconciling translations: %w", err)
	}

	existingIdx := make(map[string]int, len(src.Tags))
	for idx, t := range src.Tags {
		existingIdx[imageset.NormalizeTagText(t.Default)] = idx
	}
	for _, t := range reconciled {
		if idx, ok := existingIdx[imageset.NormalizeTagText(t.Default)]; ok {
			src.Tags[idx].EN = t.EN
		} else {
			added = append(added, t)
		}
	}

	if len(added) > 0 {
		src.Tags = append(src.Tags, added...)
		if err := recordSourceTagUsage(userID, src.Name, added); err != nil {
			return nil, fmt.Errorf("recording source tag usage: %w", err)
		}
	}
	return added, nil
}

// updateSourceTagTranslation sets tag.en where it differs from the given value; never touches Count.
func updateSourceTagTranslation(userID bson.ObjectID, source string, sourceTags []imageset.SourceTag) error {
	seen := make(map[string]struct{}, len(sourceTags))
	models := make([]mongo.WriteModel, 0, len(sourceTags))
	for _, t := range sourceTags {
		key := sourceTagKey(userID, source, t.Default)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": key, "tag.en": bson.M{"$ne": t.EN}}).
			SetUpdate(bson.M{"$set": bson.M{"tag.en": t.EN}}))
	}
	if len(models) == 0 {
		return nil
	}
	_, err := SourceTagsDB.BulkWrite(context.Background(), models)
	return err
}

// sourceTagsByKey looks up SourceTagDocs by _id (their Key), keyed by the
// same Key in the result map. Scoped to userID even though the key already
// encodes its owner, so a key string of unknown provenance (e.g. a client-
// supplied AutoTag.SrcTagKeyMatch entry) can never resolve another user's
// SourceTagDoc - it simply comes back absent, same as an unknown key.
func sourceTagsByKey(userID bson.ObjectID, keys []string) (map[string]SourceTagDoc, error) {
	result := make(map[string]SourceTagDoc, len(keys))
	if len(keys) == 0 {
		return result, nil
	}
	cursor, err := SourceTagsDB.Find(context.Background(), bson.M{"_id": bson.M{"$in": keys}, "userId": userID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())

	var docs []SourceTagDoc
	if err := cursor.All(context.Background(), &docs); err != nil {
		return nil, err
	}
	for _, d := range docs {
		result[d.Key] = d
	}
	return result, nil
}

// ListSourceTags keyset-paginates tags ordered by tag.default; cursor is the
// last tag.default seen, empty for the first page.
func ListSourceTags(userID bson.ObjectID, source, cursor string, limit int) ([]SourceTagDoc, error) {
	filter := bson.M{"userId": userID}
	if source != "" {
		filter["source"] = source
	}
	if cursor != "" {
		filter["tag.default"] = bson.M{"$gt": cursor}
	}

	opts := options.Find().SetSort(bson.D{{Key: "tag.default", Value: 1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}
	findCursor, err := SourceTagsDB.Find(context.Background(), filter, opts)
	if err != nil {
		return nil, err
	}
	defer findCursor.Close(context.Background())

	var results []SourceTagDoc
	if err := findCursor.All(context.Background(), &results); err != nil {
		return nil, err
	}
	return results, nil
}

// RegatherSummary reports what a RegatherSourceTags run found and changed,
// so a caller can distinguish "nothing was wrong" from "it did nothing".
type RegatherSummary struct {
	ScannedSets          int                 `json:"scanned_sets"`
	ScannedSources       int                 `json:"scanned_sources"`
	DistinctTags         int                 `json:"distinct_tags"`
	Created              []RegatherCreated   `json:"created"`
	CountCorrected       []RegatherCountDiff `json:"count_corrected"`
	TranslationCorrected []RegatherEnDiff    `json:"translation_corrected"`
}

type RegatherCreated struct {
	Key     string `json:"key"`
	Source  string `json:"source"`
	Default string `json:"default"`
	EN      string `json:"en"`
	Count   int    `json:"count"`
}

type RegatherCountDiff struct {
	Key      string `json:"key"`
	Source   string `json:"source"`
	Default  string `json:"default"`
	OldCount int    `json:"old_count"`
	NewCount int    `json:"new_count"`
}

type RegatherEnDiff struct {
	Key     string `json:"key"`
	Source  string `json:"source"`
	Default string `json:"default"`
	OldEN   string `json:"old_en"`
	NewEN   string `json:"new_en"`
}

// regatheredTag is one (source, normalized tag) group's aggregation result.
type regatheredTag struct {
	ID struct {
		Source string `bson:"source"`
		Tag    string `bson:"tag"`
	} `bson:"_id"`
	Count   int    `bson:"count"`
	Default string `bson:"default"`
	EN      string `bson:"en"`
}

// RegatherSourceTags recomputes every SourceTagDoc for userID from scratch,
// backfilling missing entries and correcting count/translation drift. Blocks
// for the duration on the same per-user lock ProcessSourceTags uses.
func RegatherSourceTags(userID bson.ObjectID) (summary *RegatherSummary, err error) {
	defer lockUserTags(userID.Hex())()

	log.Printf("tagging regather [%s]: started", userID.Hex())
	defer func() {
		if err != nil {
			log.Printf("tagging regather [%s]: failed: %v", userID.Hex(), err)
			return
		}
		log.Printf("tagging regather [%s]: complete - scanned %d sets/%d sources, %d distinct tags, %d created, %d count corrected, %d translations corrected",
			userID.Hex(), summary.ScannedSets, summary.ScannedSources, summary.DistinctTags,
			len(summary.Created), len(summary.CountCorrected), len(summary.TranslationCorrected))
	}()

	ctx := context.Background()

	scannedSets, err := imageset.Collection.CountDocuments(ctx, bson.M{"kscope_userid": userID.Hex()})
	if err != nil {
		return nil, fmt.Errorf("counting image sets: %w", err)
	}

	scannedSources, err := countSources(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("counting sources: %w", err)
	}

	computed, err := aggregateSourceTags(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("aggregating source tags: %w", err)
	}

	existing, err := allSourceTagsForUser(userID)
	if err != nil {
		return nil, fmt.Errorf("loading existing source tags: %w", err)
	}

	summary = &RegatherSummary{
		ScannedSets:    int(scannedSets),
		ScannedSources: scannedSources,
		DistinctTags:   len(computed),
	}

	var models []mongo.WriteModel
	for key, r := range computed {
		old, ok := existing[key]
		if !ok {
			models = append(models, mongo.NewUpdateOneModel().
				SetFilter(bson.M{"_id": key}).
				SetUpdate(bson.M{"$set": bson.M{
					"userId": userID,
					"source": r.ID.Source,
					"tag":    imageset.SourceTag{Default: r.Default, EN: r.EN},
					"count":  r.Count,
				}}).
				SetUpsert(true))
			summary.Created = append(summary.Created, RegatherCreated{
				Key: key, Source: r.ID.Source, Default: r.Default, EN: r.EN, Count: r.Count,
			})
			continue
		}

		set := bson.M{}
		if old.Count != r.Count {
			set["count"] = r.Count
			summary.CountCorrected = append(summary.CountCorrected, RegatherCountDiff{
				Key: key, Source: r.ID.Source, Default: r.Default, OldCount: old.Count, NewCount: r.Count,
			})
		}
		if old.Tag.EN != r.EN {
			set["tag.en"] = r.EN
			summary.TranslationCorrected = append(summary.TranslationCorrected, RegatherEnDiff{
				Key: key, Source: r.ID.Source, Default: r.Default, OldEN: old.Tag.EN, NewEN: r.EN,
			})
		}
		if old.Tag.Default != r.Default {
			set["tag.default"] = r.Default
		}
		if len(set) > 0 {
			models = append(models, mongo.NewUpdateOneModel().SetFilter(bson.M{"_id": key}).SetUpdate(bson.M{"$set": set}))
		}
	}

	// Tags no longer found anywhere decay to count 0 rather than being
	// deleted, so an AutoTag's SrcTagKeyMatch never dangles.
	for key, old := range existing {
		if _, ok := computed[key]; ok {
			continue
		}
		if old.Count != 0 {
			models = append(models, mongo.NewUpdateOneModel().
				SetFilter(bson.M{"_id": key}).
				SetUpdate(bson.M{"$set": bson.M{"count": 0}}))
			summary.CountCorrected = append(summary.CountCorrected, RegatherCountDiff{
				Key: key, Source: old.Source, Default: old.Tag.Default, OldCount: old.Count, NewCount: 0,
			})
		}
	}

	if len(models) > 0 {
		if _, err := SourceTagsDB.BulkWrite(ctx, models); err != nil {
			return nil, fmt.Errorf("writing regathered source tags: %w", err)
		}
	}
	return summary, nil
}

// countSources sums len(Sources) across all of userID's sets.
func countSources(ctx context.Context, userID bson.ObjectID) (int, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"kscope_userid": userID.Hex()}}},
		{{Key: "$project", Value: bson.M{"n": bson.M{"$size": bson.M{"$ifNull": bson.A{"$sources", bson.A{}}}}}}},
		{{Key: "$group", Value: bson.M{"_id": nil, "total": bson.M{"$sum": "$n"}}}},
	}
	cursor, err := imageset.Collection.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	var result struct {
		Total int `bson:"total"`
	}
	if !cursor.Next(ctx) {
		return 0, nil
	}
	if err := cursor.Decode(&result); err != nil {
		return 0, err
	}
	return result.Total, nil
}

// aggregateSourceTags counts each (source, normalized tag) pair as one
// increment per source-instance, not deduplicated by set (matches
// recordSourceTagUsage's semantics), taking Default/EN from whichever
// instance is freshest and preferring a non-empty EN. Returned keyed by
// sourceTagKey.
func aggregateSourceTags(ctx context.Context, userID bson.ObjectID) (map[string]regatheredTag, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"kscope_userid": userID.Hex()}}},
		{{Key: "$unwind", Value: "$sources"}},
		{{Key: "$unwind", Value: "$sources.tags"}},
		{{Key: "$addFields", Value: bson.M{
			"normalizedTag": bson.M{"$trim": bson.M{"input": bson.M{"$toLower": "$sources.tags.default"}}},
			"freshness":     bson.M{"$ifNull": bson.A{"$sources.last_checked", "$date_added"}},
			"hasEN":         bson.M{"$cond": bson.A{bson.M{"$ne": bson.A{"$sources.tags.en", ""}}, 1, 0}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "hasEN", Value: -1}, {Key: "freshness", Value: -1}}}},
		{{Key: "$group", Value: bson.M{
			"_id": bson.M{"source": "$sources.name", "tag": "$normalizedTag"},
			"count": bson.M{"$sum": 1},
			"default": bson.M{"$first": "$sources.tags.default"},
			"en":      bson.M{"$first": "$sources.tags.en"},
		}}},
	}
	cursor, err := imageset.Collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var rows []regatheredTag
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}

	result := make(map[string]regatheredTag, len(rows))
	for _, r := range rows {
		result[sourceTagKey(userID, r.ID.Source, r.Default)] = r
	}
	return result, nil
}

func allSourceTagsForUser(userID bson.ObjectID) (map[string]SourceTagDoc, error) {
	cursor, err := SourceTagsDB.Find(context.Background(), bson.M{"userId": userID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())

	var docs []SourceTagDoc
	if err := cursor.All(context.Background(), &docs); err != nil {
		return nil, err
	}
	result := make(map[string]SourceTagDoc, len(docs))
	for _, d := range docs {
		result[d.Key] = d
	}
	return result, nil
}
