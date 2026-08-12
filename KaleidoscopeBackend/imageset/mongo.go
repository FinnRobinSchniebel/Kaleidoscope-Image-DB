package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/authutil"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var Collection *mongo.Collection

// ErrAccessDenied is returned by GetFromID when a non-admin user requests an
// image set they do not own. Callers map it to HTTP 403. Invalid-id and
// not-found failures are reported with bson.ErrInvalidHex and
// mongo.ErrNoDocuments; test for any of these with errors.Is.
var ErrAccessDenied = errors.New("access denied")

type SearchParams struct {
	PageCount  int      `json:"page_count" form:"page_count"`   //number of images to return
	SkipCount  int      `json:"skip_count" form:"skip_count"`   //What page to return
	RandomSeed string   `json:"random_seed" form:"random_seed"` //if sorting is random, this value is passed for consistent page returns
	Tags       []string `json:"tags" bson:"tags" form:"tags"`
	Author     []string `json:"author"`
	FromDate   string   `json:"fromDate"`
	ToDate     string   `json:"toDate"`
	Title      string   `json:"title"`
	User       string

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

	var entry ImageSetMongo

	for _, ObjId := range IdBson {
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

/*
takes in a imageset ID and deletes the imageset from the mongo db and removes all files from storage
*/
func DeleteImageSetInDB(id bson.ObjectID) error {
	var entryToDelete ImageSetMongo

	//check if entry exists and get it as a struct for processing
	err := Collection.FindOne(context.Background(), bson.D{{"_id", id}}).Decode(&entryToDelete)
	if err != nil {
		log.Println("Failed to find file!")
		return err
	}
	var imageNames []string
	for i := range entryToDelete.Image {
		imageNames = append(imageNames, entryToDelete.Image[i].Name)
	}

	log.Println("Image links to delete:" + strings.Join(imageNames, ", "))

	//delete the entry
	result, err := Collection.DeleteOne(context.Background(), bson.D{{"_id", id}})
	if err != nil || result.DeletedCount == 0 {
		log.Println("Failed to delete file")
		return err
	}

	//delete files
	var errList error

	err = DeleteFilesFromInfoList(entryToDelete.Path, entryToDelete.Image)
	if err != nil {
		errList = errors.Join(errList, err)
	}

	// err = DeleteFileList(entryToDelete.Path, entryToDelete.LowImage)
	// if err != nil {
	// 	errList = errors.Join(errList, err)
	// }

	if errList != nil {
		return errList
	}

	log.Print("---delete complete--- ")

	return nil
}

/*This function builds the pipeline use by the SearchDBForImages function to query the DB*/
func FilterSearchPipeline(params SearchParams) mongo.Pipeline {
	pipeline := mongo.Pipeline{}

	searchTags := bson.D{{Key: "tags", Value: bson.D{{Key: "$all", Value: params.Tags}}}}
	searchTitles := bson.D{{Key: "title", Value: bson.D{{"$regex", params.Title}, {"$options", "i"}}}}
	searchAuthor := bson.D{{"author", bson.D{{"$all", params.Author}}}}
	multiSearchParam := bson.A{}

	//Make sure the user can only find unowned and their own uploads
	FilterUser := bson.D{
		{Key: "$match", Value: bson.M{
			"kscope_userid": bson.M{
				"$in": []string{"", params.User},
			},
		}},
	}

	pipeline = append(pipeline, FilterUser)

	//add tag matches
	if len(params.Tags) > 0 {
		multiSearchParam = append(multiSearchParam, searchTags)
	}
	//add title matches
	if params.Title != "" {
		multiSearchParam = append(multiSearchParam, searchTitles)
	}
	//add tag matches
	if len(params.Author) > 0 {
		multiSearchParam = append(multiSearchParam, searchAuthor)
	}

	if len(params.Tags) > 0 || params.Title != "" || len(params.Author) > 0 {
		pipeline = append(pipeline, bson.D{
			{Key: "$match", Value: bson.D{
				{
					Key: "$or", Value: multiSearchParam,
				},
			}},
		})
	}

	// date will be used later (it will be the date the image was added to db)
	dateMatch := bson.D{}
	if params.FromDate != "" {
		dateMatch = append(dateMatch, bson.E{"$gte", params.FromDate})
	}
	if params.ToDate != "" {
		dateMatch = append(dateMatch, bson.E{"$lte", params.ToDate})
	}
	if len(dateMatch) > 0 {
		pipeline = append(pipeline, bson.D{
			{Key: "$match", Value: bson.D{
				{Key: "date_added", Value: dateMatch},
			}},
		})
	}

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

	return pipeline
}

/*Provides image ID's and tags for images that match the query*/
func SearchDBForImages(params SearchParams) (bson.M, error) {
	fmt.Printf("test %d, %d \n", params.SkipCount, params.PageCount)

	pipeline := FilterSearchPipeline(params)

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
