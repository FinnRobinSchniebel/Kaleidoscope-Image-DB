package tagging

import (
	"context"
	"regexp"
	"sort"
	"strings"

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
	return strings.ToLower(strings.TrimSpace(tag))
}

// sourceTagKey must be used everywhere a SourceTag is written or looked up;
// it is the doc's _id, so a mismatched key here silently creates a duplicate
// rather than finding the existing tag. A plain deterministic string rather
// than a bson.ObjectID, deliberately: unlike AutoTagEntry.ID, nothing ever
// needs to look this up by anything other than (userID, source, default).
func sourceTagKey(userID bson.ObjectID, source, tagDefault string) string {
	return userID.Hex() + "::" + source + "::" + normalizeTag(tagDefault)
}

// TODO: reconciliation pass that recomputes every SourceTagDoc's Count from
// scratch by scanning all of a user's image sets, to correct any drift from
// the incremental recordSourceTagUsage/decrementSourceTagUsage bookkeeping.

func EnsureIndexes(ctx context.Context) error {
	_, err := SourceTagsDB.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "userId", Value: 1}, {Key: "tag.default", Value: 1}}},
		{Keys: bson.D{{Key: "userId", Value: 1}, {Key: "tag.en", Value: 1}}},
	})
	return err
}

// recordSourceTagUsage upserts one SourceTagDoc per tag, incrementing Count.
func recordSourceTagUsage(userID bson.ObjectID, source string, sourceTags []imageset.SourceTag) error {
	if len(sourceTags) == 0 {
		return nil
	}
	models := make([]mongo.WriteModel, 0, len(sourceTags))
	for _, t := range sourceTags {
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": sourceTagKey(userID, source, t.Default)}).
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

	cursor, err := SourceTagsDB.Find(context.Background(), filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())

	var results []SourceTagDoc
	if err := cursor.All(context.Background(), &results); err != nil {
		return nil, err
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Count > results[j].Count })
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// sourceTagsByKey looks up SourceTagDocs by _id (their Key), keyed by the
// same Key in the result map.
func sourceTagsByKey(keys []string) (map[string]SourceTagDoc, error) {
	result := make(map[string]SourceTagDoc, len(keys))
	if len(keys) == 0 {
		return result, nil
	}
	cursor, err := SourceTagsDB.Find(context.Background(), bson.M{"_id": bson.M{"$in": keys}})
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

	opts := options.Find().SetSort(bson.D{{Key: "tag.default", Value: 1}}).SetLimit(int64(limit))
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
