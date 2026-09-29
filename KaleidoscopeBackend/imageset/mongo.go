package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/authutil"
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var Collection *mongo.Collection

// EnsureIndexes creates the indexes used to find image sets by owner plus
// source tag or AutoTag (tagging.applyAutoTagToSets' candidate query), and
// the indexes search's own pipeline relies on: the default newest-first
// sort (with an _id tie-breaker for determinism), and tags/authors
// membership lookups. Idempotent, safe to call on every startup.
func EnsureIndexes(ctx context.Context) error {
	_, err := Collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "kscope_userid", Value: 1}, {Key: "sources.tags.default", Value: 1}}},
		{Keys: bson.D{{Key: "kscope_userid", Value: 1}, {Key: "autotags", Value: 1}}},
		{Keys: bson.D{{Key: "kscope_userid", Value: 1}, {Key: "date_added", Value: -1}, {Key: "_id", Value: -1}}},
		{Keys: bson.D{{Key: "kscope_userid", Value: 1}, {Key: "tags", Value: 1}}},
		{Keys: bson.D{{Key: "kscope_userid", Value: 1}, {Key: "authors", Value: 1}}},
	})
	return err
}

// updateSourceFields only writes if Sources[index] still holds the same
// source (by name and source_id), so a shifted array can't redirect the write.
// Keys in fields are full paths, e.g. "sources.2.last_checked". A write that
// includes "tags" refreshes the owner's Untagged count afterwards.
func updateSourceFields(set *ImageSetMongo, index int, fields bson.M) error {
	src := set.Sources[index]
	prefix := fmt.Sprintf("sources.%d.", index)
	filter := bson.M{
		"_id":                set.ID,
		prefix + "name":      src.Name,
		prefix + "source_id": src.SourceID,
	}
	result, err := Collection.UpdateOne(context.Background(), filter, bson.M{"$set": fields})
	if err != nil {
		return fmt.Errorf("updating image set source: %w", err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("update matched no image set with source %s/%s at index %d", src.Name, src.SourceID, index)
	}
	if _, tagsChanged := fields["tags"]; tagsChanged {
		refreshUntagged(set.KscopeUserId)
	}
	return nil
}

// refreshUntagged recounts userID's Untagged tag after a write that changed
// tags. The write has already landed, so a failure is only logged.
func refreshUntagged(userID string) {
	if err := Tagger.RefreshUntaggedCount(userID); err != nil {
		log.Printf("------ Warning: refreshing untagged count for user [%s]: %s ------", userID, err)
	}
}

// tagFields never writes autotags as null: $addToSet/$pull elsewhere reject it.
func tagFields(set *ImageSetMongo) bson.M {
	autoTags := set.AutoTags
	if autoTags == nil {
		autoTags = []bson.ObjectID{}
	}
	return bson.M{"autotags": autoTags, "tags": set.Tags}
}

// UpdateTagTranslations applies EN to every image set (userID's own) with a
// source named sourceName carrying a tag matching each entry's Default
// (case/whitespace-insensitive, same identity NormalizeTagText defines),
// wherever the stored EN currently differs. One BulkWrite for the batch.
func UpdateTagTranslations(userID, sourceName string, tags []SourceTag) error {
	if len(tags) == 0 {
		return nil
	}
	models := make([]mongo.WriteModel, 0, len(tags))
	for _, t := range tags {
		pattern := "^" + regexp.QuoteMeta(strings.TrimSpace(t.Default)) + "$"
		matchesTag := bson.M{"default": bson.M{"$regex": pattern, "$options": "i"}, "en": bson.M{"$ne": t.EN}}
		filter := bson.M{
			"kscope_userid": userID,
			"sources":       bson.M{"$elemMatch": bson.M{"name": sourceName, "tags": bson.M{"$elemMatch": matchesTag}}},
		}
		update := bson.M{"$set": bson.M{"sources.$[src].tags.$[tag].en": t.EN}}
		arrayFilters := []any{
			bson.M{"src.name": sourceName},
			bson.M{"tag.default": bson.M{"$regex": pattern, "$options": "i"}, "tag.en": bson.M{"$ne": t.EN}},
		}
		models = append(models, mongo.NewUpdateManyModel().SetFilter(filter).SetUpdate(update).SetArrayFilters(arrayFilters))
	}
	_, err := Collection.BulkWrite(context.Background(), models)
	return err
}

// ErrAccessDenied is returned by GetFromID when a non-admin user requests an
// image set they do not own. Callers map it to HTTP 403. Invalid-id and
// not-found failures are reported with bson.ErrInvalidHex and
// mongo.ErrNoDocuments; test for any of these with errors.Is.
var ErrAccessDenied = errors.New("access denied")

type SearchParams struct {
	PageCount  int    `json:"page_count"`  //number of images to return
	SkipCount  int    `json:"skip_count"`  //What page to return
	RandomSeed string `json:"random_seed"` //still unused - no random-order sort exists yet

	Words   []string `json:"words"`   //bare terms with no recognized prefix
	Tags    []string `json:"tags"`    //from tag: prefix - names/partial names, resolved to AutoTag ids per term
	Titles  []string `json:"titles"`  //from title: prefix
	Authors []string `json:"authors"` //from author: prefix
	Sources []string `json:"sources"` //from source: prefix

	SearchTags    bool `json:"searchTags"` //gates bare-word-vs-tag matching only, never explicit tag: terms
	SearchTitles  bool `json:"searchTitles"`
	SearchAuthors bool `json:"searchAuthors"`
	SearchSources bool `json:"searchSources"`

	FromDate string `json:"fromDate"`
	ToDate   string `json:"toDate"`

	FromDateParsed *time.Time `json:"-"` //set by FilterForImageSets after validating FromDate
	ToDateParsed   *time.Time `json:"-"`

	User string `json:"-"`

	//TODO: type, image count,
}

type CollisionResponsePair struct {
	IdOfHashCollision bson.ObjectID
	ImageNumber       int
}

type CollisionMap map[int][]CollisionResponsePair

func findOverlappingHashes(hash string, userID string) ([]CollisionResponsePair, error) {

	filter := bson.D{
		{"kscope_userid", userID},
		{"images.hash", hash},
	}
	cursor, err := Collection.Find(context.Background(), filter)
	if err != nil {
		return nil, err
	}

	defer cursor.Close(context.Background())

	var itemList []ImageSetMongo

	cursor.All(context.Background(), &itemList)
	if len(itemList) == 0 {
		return nil, nil
	}

	var idList []CollisionResponsePair
	for _, item := range itemList {
		for index, _ := range item.Image {
			if item.Image[index].ImageHash == hash {
				idList = append(idList, CollisionResponsePair{item.ID, index})
			}
		}

		//var iSet ImageSetMongo
		//bson.Unmarshal([]byte(item.String()), &iSet)
		//item["_id"].(bson.ObjectID)

		//itemList = append(itemList)

		//idList = append(idList, CollisionResponsePair{item.ID, })
	}

	return idList, nil
}

// Validates user credentials
func GetFromID(usr string, id ...string) ([]ImageSetMongo, error) {

	var IdBson []bson.ObjectID

	for _, item := range id {
		ObjId, err := bson.ObjectIDFromHex(item)
		if err != nil {
			// ObjectIDFromHex returns bson.ErrInvalidHex or a raw hex error
			// depending on the input; normalize onto ErrInvalidHex so callers
			// can classify any bad id with errors.Is.
			return nil, fmt.Errorf("%w: %q", bson.ErrInvalidHex, item)
		}
		IdBson = append(IdBson, ObjId)
	}

	var iSets []ImageSetMongo

	for _, ObjId := range IdBson {
		// declared per pass: Decode reuses a struct's slice memory, which would be shared with earlier results
		var entry ImageSetMongo
		err := Collection.FindOne(context.Background(), bson.D{{"_id", ObjId}}).Decode(&entry)
		if err != nil {
			// mongo.ErrNoDocuments stays matchable through the wrap so callers
			// can tell a missing set (404) from a real DB error (500).
			log.Printf("failed to query image set %s: %v", ObjId.Hex(), err)
			return nil, fmt.Errorf("querying image set %s: %w", ObjId.Hex(), err)
		}
		//check access
		if entry.KscopeUserId != usr && entry.KscopeUserId != "" {
			//if denied  and not admin, error
			if !authutil.IsAdmin(usr) {
				log.Printf("User access denied: user %s, accessing %s", usr, ObjId.Hex())
				return nil, fmt.Errorf("%w: image set %s", ErrAccessDenied, ObjId.Hex())
			}
		}

		iSets = append(iSets, entry)
	}
	if len(iSets) == 0 {
		return nil, mongo.ErrNoDocuments
	}

	return iSets, nil
}

// ownedFilter matches the image sets in ids, restricted to userID's own sets
// unless admin is set.
func ownedFilter(ids []bson.ObjectID, userID string, admin bool) bson.M {
	filter := bson.M{"_id": bson.M{"$in": ids}}
	if !admin {
		filter["kscope_userid"] = userID
	}
	return filter
}

// DeleteImageSetInDB deletes the set, its image files and its recorded tag
// counts. Non-admins can only delete their own sets; a set that is missing or
// not theirs returns mongo.ErrNoDocuments either way. Files go first, so a
// failed file removal leaves the document for a retry to finish.
func DeleteImageSetInDB(id bson.ObjectID, userID string, admin bool) error {
	filter := ownedFilter([]bson.ObjectID{id}, userID, admin)

	var entryToDelete ImageSetMongo
	if err := Collection.FindOne(context.Background(), filter).Decode(&entryToDelete); err != nil {
		return err
	}
	var imageNames []string
	for i := range entryToDelete.Image {
		imageNames = append(imageNames, entryToDelete.Image[i].Name)
	}
	log.Println("Image links to delete:" + strings.Join(imageNames, ", "))

	if err := DeleteFilesFromInfoList(entryToDelete.Path, entryToDelete.Image); err != nil {
		return fmt.Errorf("deleting image files: %w", err)
	}
	if err := DeleteThumbnailFile(entryToDelete.Path, entryToDelete.ThumbNail); err != nil {
		return fmt.Errorf("deleting thumbnail file: %w", err)
	}

	result, err := Collection.DeleteOne(context.Background(), filter)
	if err != nil {
		return fmt.Errorf("deleting image set: %w", err)
	}
	if result.DeletedCount == 0 {
		// a concurrent delete removed it first and records the counts itself
		return mongo.ErrNoDocuments
	}

	refreshUntagged(entryToDelete.KscopeUserId)
	if err := Tagger.RecordDeletion(entryToDelete.KscopeUserId, entryToDelete.Sources, entryToDelete.Tags); err != nil {
		return fmt.Errorf("undoing tag counts: %w", err)
	}

	log.Print("---delete complete--- ")
	return nil
}

// ErrSetsNotFound is returned by TagStatesForSets when some ids are missing
// or not the caller's. Callers map it to HTTP 404.
var ErrSetsNotFound = errors.New("image sets not found")

// SetTagState is the tag-related slice of an image set that tag override
// updates need to read.
type SetTagState struct {
	ID           bson.ObjectID   `bson:"_id"`
	KscopeUserId string          `bson:"kscope_userid"`
	Tags         []string        `bson:"tags"`
	AutoTags     []bson.ObjectID `bson:"autotags"`
}

// TagStatesForSets returns the tag state of every set in ids, restricted to
// userID's own sets unless admin is set. Returns ErrSetsNotFound naming any
// id that didn't match.
func TagStatesForSets(ids []bson.ObjectID, userID string, admin bool) ([]SetTagState, error) {
	opts := options.Find().SetProjection(bson.M{"kscope_userid": 1, "tags": 1, "autotags": 1})
	cursor, err := Collection.Find(context.Background(), ownedFilter(ids, userID, admin), opts)
	if err != nil {
		return nil, fmt.Errorf("finding image sets: %w", err)
	}
	defer cursor.Close(context.Background())

	var states []SetTagState
	if err := cursor.All(context.Background(), &states); err != nil {
		return nil, fmt.Errorf("finding image sets: %w", err)
	}

	found := make(map[bson.ObjectID]bool, len(states))
	for _, s := range states {
		found[s.ID] = true
	}
	var missing []string
	for _, id := range ids {
		if !found[id] {
			missing = append(missing, id.Hex())
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrSetsNotFound, strings.Join(missing, ", "))
	}
	return states, nil
}

// SaveTagOverrides sets every set in tagsByID to overrides and its own tags,
// in one bulk write. The ids must already be ownership-checked (see
// TagStatesForSets). Nil overrides are stored as an empty list.
func SaveTagOverrides(overrides []string, tagsByID map[bson.ObjectID][]string) error {
	if len(tagsByID) == 0 {
		return nil
	}
	if overrides == nil {
		overrides = []string{}
	}
	models := make([]mongo.WriteModel, 0, len(tagsByID))
	for id, tags := range tagsByID {
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": id}).
			SetUpdate(bson.M{"$set": bson.M{"tag_rule_overrides": overrides, "tags": tags}}))
	}
	if _, err := Collection.BulkWrite(context.Background(), models); err != nil {
		return fmt.Errorf("saving tag overrides: %w", err)
	}
	return nil
}

// OverrideMerge is one MergeTagOverrides change: entries are dropped from
// and added to tag_rule_overrides, and ids added to and dropped from tags.
type OverrideMerge struct {
	Drop     []string
	Add      []string
	TagsAdd  []string
	TagsDrop []string
}

// MergeTagOverrides applies m to every set in ids (userID's own unless
// admin) in one update, atomic per set: tag_rule_overrides becomes its
// entries minus m.Drop, plus m.Add without duplicates, and tags becomes tags
// plus m.TagsAdd minus m.TagsDrop.
func MergeTagOverrides(ids []bson.ObjectID, userID string, admin bool, m OverrideMerge) error {
	arrayOrEmpty := func(field string) bson.D {
		return bson.D{{Key: "$ifNull", Value: bson.A{field, bson.A{}}}}
	}
	literal := func(values []string) bson.D {
		if values == nil {
			values = []string{}
		}
		return bson.D{{Key: "$literal", Value: values}}
	}

	keptOverrides := bson.D{{Key: "$filter", Value: bson.D{
		{Key: "input", Value: arrayOrEmpty("$tag_rule_overrides")},
		{Key: "cond", Value: bson.D{{Key: "$not", Value: bson.A{
			bson.D{{Key: "$in", Value: bson.A{"$$this", literal(m.Drop)}}},
		}}}},
	}}}
	update := mongo.Pipeline{{{Key: "$set", Value: bson.D{
		{Key: "tag_rule_overrides", Value: bson.D{{Key: "$setUnion", Value: bson.A{keptOverrides, literal(m.Add)}}}},
		{Key: "tags", Value: bson.D{{Key: "$setDifference", Value: bson.A{
			bson.D{{Key: "$setUnion", Value: bson.A{arrayOrEmpty("$tags"), literal(m.TagsAdd)}}},
			literal(m.TagsDrop),
		}}}},
	}}}}

	if _, err := Collection.UpdateMany(context.Background(), ownedFilter(ids, userID, admin), update); err != nil {
		return fmt.Errorf("merging tag overrides: %w", err)
	}
	return nil
}

// ParseSearchDate parses a fromDate/toDate search param: RFC3339 or a plain
// YYYY-MM-DD date (interpreted as UTC midnight).
func ParseSearchDate(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("invalid date %q: expected RFC3339 or YYYY-MM-DD", s)
}

// buildTagCondition resolves term to AutoTag ids via Tagger.ResolveTagTerm
// and returns the condition matching sets whose Tags satisfies it: contains
// any resolved id, or has an empty Tags array if the reserved Untagged tag
// matched by name. A term matching nothing returns a condition that can
// never match, so it correctly excludes every result rather than being
// silently ignored.
func buildTagCondition(userID, term string) (bson.M, error) {
	ids, matchEmpty, err := Tagger.ResolveTagTerm(userID, term)
	if err != nil {
		return nil, fmt.Errorf("resolving tag term %q: %w", term, err)
	}
	var or bson.A
	if len(ids) > 0 {
		or = append(or, bson.M{"tags": bson.M{"$in": ids}})
	}
	if matchEmpty {
		or = append(or, bson.M{"tags": nil}, bson.M{"tags": bson.M{"$size": 0}})
	}
	switch len(or) {
	case 0:
		return bson.M{"tags": bson.M{"$in": bson.A{}}}, nil
	case 1:
		return or[0].(bson.M), nil
	default:
		return bson.M{"$or": or}, nil
	}
}

func titleCondition(term string) bson.M {
	return bson.M{"title": bson.M{"$regex": regexp.QuoteMeta(term), "$options": "i"}}
}

func authorCondition(term string) bson.M {
	return bson.M{"authors": bson.M{"$regex": regexp.QuoteMeta(term), "$options": "i"}}
}

func sourceCondition(term string) bson.M {
	return bson.M{"sources.name": bson.M{"$regex": regexp.QuoteMeta(term), "$options": "i"}}
}

/*This function builds the pipeline use by the SearchDBForImages function to query the DB*/
func FilterSearchPipeline(params SearchParams) (mongo.Pipeline, error) {
	pipeline := mongo.Pipeline{}

	//Make sure the user can only find unowned and their own uploads. This is
	//the leading stage, and the common prefix of every index this pipeline
	//relies on, so every query - filtered or not - gets at least this much
	//index support.
	pipeline = append(pipeline, bson.D{
		{Key: "$match", Value: bson.M{
			"kscope_userid": bson.M{
				"$in": []string{"", params.User},
			},
		}},
	})

	//Each entry below becomes its own AND'd clause - every additional term
	//narrows the result set further. A bare word becomes one AND'd clause
	//that is itself an OR across whichever categories are checkbox-enabled.
	var andClauses bson.A

	for _, term := range params.Tags {
		if term == "" {
			continue
		}
		cond, err := buildTagCondition(params.User, term)
		if err != nil {
			return nil, err
		}
		andClauses = append(andClauses, cond)
	}
	for _, term := range params.Titles {
		if term != "" {
			andClauses = append(andClauses, titleCondition(term))
		}
	}
	for _, term := range params.Authors {
		if term != "" {
			andClauses = append(andClauses, authorCondition(term))
		}
	}
	for _, term := range params.Sources {
		if term != "" {
			andClauses = append(andClauses, sourceCondition(term))
		}
	}
	for _, word := range params.Words {
		if word == "" {
			continue
		}
		var or bson.A
		if params.SearchTags {
			cond, err := buildTagCondition(params.User, word)
			if err != nil {
				return nil, err
			}
			or = append(or, cond)
		}
		if params.SearchTitles {
			or = append(or, titleCondition(word))
		}
		if params.SearchAuthors {
			or = append(or, authorCondition(word))
		}
		if params.SearchSources {
			or = append(or, sourceCondition(word))
		}
		switch len(or) {
		case 0:
			//no enabled category to check this word against - unsatisfiable,
			//not ignored, so the search doesn't silently fall back to unfiltered
			andClauses = append(andClauses, bson.M{"_id": bson.M{"$in": bson.A{}}})
		case 1:
			andClauses = append(andClauses, or[0])
		default:
			andClauses = append(andClauses, bson.M{"$or": or})
		}
	}

	//only appended when there's something to filter on, so a search with no
	//terms at all falls through to the unfiltered, newest-first listing
	if len(andClauses) > 0 {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.M{"$and": andClauses}}})
	}

	dateMatch := bson.D{}
	if params.FromDateParsed != nil {
		dateMatch = append(dateMatch, bson.E{Key: "$gte", Value: *params.FromDateParsed})
	}
	if params.ToDateParsed != nil {
		dateMatch = append(dateMatch, bson.E{Key: "$lte", Value: *params.ToDateParsed})
	}
	if len(dateMatch) > 0 {
		pipeline = append(pipeline, bson.D{
			{Key: "$match", Value: bson.D{
				{Key: "date_added", Value: dateMatch},
			}},
		})
	}

	//Newest first, with an _id tie-breaker for full determinism when
	//multiple sets share a date_added (e.g. a batch zip import) - this is
	//what makes $skip/$limit pagination below consistent page-to-page,
	//since without a stable sort MongoDB doesn't guarantee document order.
	//Sitting directly after the indexed kscope_userid match with nothing
	//blocking index use in between, this can be served off the
	//{kscope_userid, date_added, _id} index without a separate in-memory
	//sort when no other filter clause forces a different index choice.
	pipeline = append(pipeline, bson.D{{Key: "$sort", Value: bson.D{
		{Key: "date_added", Value: -1},
		{Key: "_id", Value: -1},
	}}})

	//This section determines what values get returned from the documents
	project := bson.M{"$project": bson.D{
		{Key: "_id", Value: 1},  // return ID
		{Key: "tags", Value: 1}, // Add tags
		// {Key: "title", Value: 0},              // No title
		// {Key: "authors", Value: 0},            // No authors
		// {Key: "description", Value: 0},        // No description
		// {Key: "date_added", Value: 0},         // No dateAdded
		// {Key: "sources", Value: 0},            // No sources
		// {Key: "tag_rule_overrides", Value: 0}, // No tag_rule_overrides
		// count of images where active = true
		{Key: "activeImageCount", Value: bson.D{
			{Key: "$size", Value: bson.D{
				{Key: "$filter", Value: bson.D{
					{Key: "input", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$images", bson.A{}}}}},
					{Key: "as", Value: "img"},
					{Key: "cond", Value: bson.D{{Key: "$eq", Value: bson.A{"$$img.active", true}}}},
				}},
			}},
		}},
	}}

	// needs a facet to get item count
	pipeline = append(pipeline, bson.D{
		{"$facet", bson.M{
			"totalCount": []bson.M{
				{"$count": "count"},
			},
			"imagesets": []bson.M{
				// Limit the number of returned documents and skip as many pages worth of documents as needed
				{"$skip": params.SkipCount},
				{"$limit": params.PageCount},
				//project to the return results of the itemsset
				project,
			},
		}},
	})

	pipeline = append(pipeline,
		bson.D{{"$project", bson.M{
			"imagesets": 1,
			"totalCount": bson.M{
				"$ifNull": bson.A{
					bson.M{"$arrayElemAt": bson.A{"$totalCount.count", 0}},
					0,
				},
			},
		}}},
	)

	return pipeline, nil
}

/*Provides image ID's and tags for images that match the query*/
func SearchDBForImages(params SearchParams) (bson.M, error) {
	pipeline, err := FilterSearchPipeline(params)
	if err != nil {
		return nil, err
	}

	cursor, err := Collection.Aggregate(context.Background(), pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())

	var results bson.M
	if !cursor.Next(context.Background()) {
		return nil, fmt.Errorf("Error: Query resulted in a nil return from the pipeline")
	}
	cursor.Decode(&results)

	// if err := cursor.All(context.Background(), &results); err != nil {
	// 	return nil, err
	// }

	return results, nil
}

// GetImageSetsBySourceIDs returns the full imageSet document for every saved set
// that has a source named sourceName whose SourceID is one of sourceIDs, keyed by
// that SourceID. Full documents (not just the matching SourceInfo) are returned
// because callers may need to update the set in place. Only contains items owned
// by the given user.
func GetImageSetsBySourceIDs(userId string, sourceName string, sourceIDs []string) (map[string]*ImageSetMongo, error) {
	ctx := context.Background()

	filter := bson.M{
		"kscope_userid": userId,
		"sources": bson.M{"$elemMatch": bson.M{
			"name":      sourceName,
			"source_id": bson.M{"$in": sourceIDs},
		}},
	}

	cursor, err := Collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var docs []ImageSetMongo
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}

	result := make(map[string]*ImageSetMongo, len(docs))
	for i := range docs {
		for _, src := range docs[i].Sources {
			if src.Name == sourceName {
				result[src.SourceID] = &docs[i]
			}
		}
	}
	return result, nil
}

// GetImageSetBySourceID is a single-ID convenience wrapper around
// GetImageSetsBySourceIDs. ok is false if no owned set has a matching source.
func GetImageSetBySourceID(userId string, sourceName string, sourceID string) (set *ImageSetMongo, ok bool, err error) {
	sets, err := GetImageSetsBySourceIDs(userId, sourceName, []string{sourceID})
	if err != nil {
		return nil, false, err
	}
	set, ok = sets[sourceID]
	return set, ok, nil
}

/*Provides display info for the image you associated with the image ID*/
func GetImageInfoFromDB(paramIDs []bson.ObjectID, userID string) ([]bson.M, error) {

	cursor, err := Collection.Find(context.Background(),
		bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: paramIDs}}}},
		options.Find().SetProjection(bson.D{
			{Key: "_id", Value: 1},                // return ID
			{Key: "tags", Value: 1},               // keep tags
			{Key: "title", Value: 1},              // keep title
			{Key: "authors", Value: 1},            // keep authors
			{Key: "description", Value: 1},        // keep description
			{Key: "date_added", Value: 1},         // keep dateAdded
			{Key: "sources", Value: 1},            // keep sources
			{Key: "tag_rule_overrides", Value: 1}, // keep tag_rule_overrides
			{Key: "kscope_userid", Value: 1},      //keep user id for validation
			// count of images where active = true
			{Key: "activeImageCount", Value: bson.D{
				{Key: "$size", Value: bson.D{
					{Key: "$filter", Value: bson.D{
						{Key: "input", Value: "$images"},
						{Key: "as", Value: "img"},
						{Key: "cond", Value: bson.D{{Key: "$eq", Value: bson.A{"$$img.active", true}}}},
					}},
				}},
			}},
		}),
	)

	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())

	var results []bson.M

	if err := cursor.All(context.Background(), &results); err != nil {
		return nil, err
	}

	var filtered []bson.M
	hasCheckedAdmin := false
	isAdmin := false

	for _, doc := range results {

		// Safely read the field
		imageUserID, ok := doc["kscope_userid"].(string)

		//skip if access is denied
		if ok && imageUserID != "" && imageUserID != userID {
			//only checks db once and only when needed (lazy)
			if !hasCheckedAdmin {
				hasCheckedAdmin = true
				isAdmin = authutil.IsAdmin(userID)
			}
			if isAdmin {
				filtered = results
				break
			}
			continue
		}

		filtered = append(filtered, doc)
	}

	for i, _ := range results {
		delete(results[i], "kscope_userid")
	}

	return filtered, nil
}
