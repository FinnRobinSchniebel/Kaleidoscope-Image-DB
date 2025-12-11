package imageset

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"log"
	"mime/multipart"
	"os"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var Collection *mongo.Collection

type SearchParams struct {
	PageCount  int      `json:"page_count" form:"page_count"`   //number of images to return
	Page       int      `json:"page" form:"page"`               //What page to return
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

func findOverlappingHashes(hash string) ([]CollisionResponsePair, error) {
	cursor, err := Collection.Find(context.Background(), bson.D{{"images.hash", hash}})
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

		itemList = append(itemList)

		//idList = append(idList, CollisionResponsePair{item.ID, })
	}

	return idList, nil
}

func GetFromID(id ...string) ([]ImageSetMongo, error) {

	var IdBson []bson.ObjectID

	for _, item := range id {
		ObjId, err := bson.ObjectIDFromHex(item)
		if err != nil {
			return nil, err
		}
		IdBson = append(IdBson, ObjId)
	}

	var iSets []ImageSetMongo

	var entry ImageSetMongo

	for _, ObjId := range IdBson {
		err := Collection.FindOne(context.Background(), bson.D{{"_id", ObjId}}).Decode(&entry)
		if err != nil {
			log.Println("Failed to find file!")
			return nil, err
		}
		iSets = append(iSets, entry)
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

/*This function adds an image set to the DB and local storage*/
func AddImageSet(imageSet *ImageSetMongo, media []*multipart.FileHeader, userId string) (InternalResponse, map[int][]CollisionResponsePair) {

	//TODO: Test if image size is to large

	//clean file paths to avoid unauthorized access
	imageSet.Image = nil
	//imageSet.LowImage = nil
	imageSet.KscopeUserId = ""

	//set the author in case of none given to avoid issues with file path creation
	if len(imageSet.Authors) == 0 || (imageSet.Authors[0] == "") {
		imageSet.Authors = []string{"unknown"}
	}
	//add userId (done as seperate step to avoid exploits if changes are made)
	imageSet.KscopeUserId = userId

	//check media count first to avoid empty imagsets in db
	if len(media) == 0 {
		return InternalResponse{400, "No Media attached"}, nil
	}

	var err error

	/**		Test FilePath	 **/
	_, err = os.Stat(BackendVolumeLocation)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("File or directory does not exist at: %s\n", BackendVolumeLocation)
		} else {
			fmt.Printf("Error accessing path %s: %v\n", BackendVolumeLocation, err)
		}
	} else {
		fmt.Printf("File or directory exists at: %s\n", BackendVolumeLocation)
	}

	//determine folder path for images and add the path to the imagset before first insert

	imageSet.Path, err = MakeFileDirectoryFromAuthor(imageSet.Authors[0])

	if err != nil {
		return InternalResponse{500, err.Error()}, nil
	}

	imageSet.DateAdded = time.Now()

	//add to DB
	insertResult, err := Collection.InsertOne(context.Background(), imageSet)

	if err != nil {
		return InternalResponse{500, err.Error()}, nil
	}

	imageSet.ID = insertResult.InsertedID.(bson.ObjectID)

	hashHits := make(map[int][]CollisionResponsePair)

	for index := range media {

		fmt.Println(media[index].Filename, media[index].Size, media[index].Header["Content-Type"][0])

		/**		save media		**/
		fileName := media[index].Filename

		//Need to know the file type to save it in the correct format
		itype, err := getFileTypeFromHeader(media[index])
		if err != nil {

			return InternalResponse{500, err.Error()}, nil
		}

		var ihash string

		//gifs must be handled differently
		if itype == "gif" {
			var igif *gif.GIF
			igif, err = FileHeaderToGif(media[index])
			if err != nil {
				return InternalResponse{500, err.Error()}, nil
			}
			fileName, ihash, err = SaveGif(igif, imageSet.Path, fileName, imageSet.ID, index)

		} else {
			var inImage *image.Image
			inImage, _, err = FileHeaderToImage(media[index])
			if err != nil {
				return InternalResponse{500, err.Error()}, nil
			}
			fileName, ihash, err = SaveImage(inImage, imageSet.Path, fileName, imageSet.ID, index, "png")
		}

		if err != nil {
			return InternalResponse{500, err.Error()}, nil
		}

		imageSet.Image = append(imageSet.Image, ImageInfo{Name: fileName, ImageHash: ihash, IsImageActive: true})

		//compare hash in DB

		HitResults, err := findOverlappingHashes(ihash)

		if err != nil {
			return InternalResponse{500, err.Error()}, nil
		}
		if len(HitResults) != 0 {
			fmt.Println("Hash Hit")
			hashHits[index] = HitResults
		}

	}

	log.Print("Files Uploaded")

	update := bson.M{"$set": imageSet}
	result, err := Collection.UpdateByID(context.Background(), imageSet.ID, update)

	if err != nil {
		fmt.Println("Update Failed")
		return InternalResponse{500, err.Error()}, nil
	}

	if result.MatchedCount == 0 {
		log.Print("COULD NOT UPDATE DB FILE AFTER ADDING INFO")
		return InternalResponse{500, "Error while updating db entry after saving files"}, nil
		//return c.Status(500).SendString()
	}
	log.Println("---Upload complete---")
	//hash conflict detected
	if len(hashHits) != 0 {
		//return InternalErrorHandle{202, "Error while updating db entry after saving files"}
		return InternalResponse{202, "Ok, Hash collision detected"}, hashHits
	}

	return InternalResponse{201, "Ok, Added to DB"}, nil
}

/*This function accepts a low res version of an image and stores it to the local storage for future retrieval */
func AddLowresToSetAndStorage(pathWithLowAppend string, name string, img *image.Image, imageset ImageSetMongo, index int) {

	if index < 0 || index > len(imageset.Image) {
		log.Println("Add Lowres: index out of bounds")
		return
	}
	err := os.MkdirAll(pathWithLowAppend, 0700)
	if err != nil {
		log.Println("Add Lowres failed to make dir: " + err.Error())
		return
	}

	filename, _, err := SaveImage(img, pathWithLowAppend, name, imageset.ID, index, "png")
	if err != nil {
		log.Println("Add Lowres: could not save image: " + err.Error())
		return
	}

	filter := bson.M{"_id": imageset.ID}

	update := bson.M{
		"$set": bson.M{
			fmt.Sprintf("images.%d.low_images", index): filename,
		},
	}
	result, err := Collection.UpdateOne(context.Background(), filter, update)
	if err != nil || result.ModifiedCount == 0 {
		err = os.Remove(fmt.Sprintf("%s%s", pathWithLowAppend, name))
		if err != nil {
			log.Println("Add Lowres: could not make changes to db...\n COULD NOT remove image from disk")
		} else {
			log.Println("Add Lowres: could not make changes to db...\n removed image from disk")
		}
		return
	}

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
					{Key: "input", Value: "$images"},
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
				{"$skip": params.Page * params.PageCount},
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
	fmt.Printf("test %d, %d \n", params.Page, params.PageCount)

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

/*Provides display info for the image you associated with the image ID*/
func GetImageInfoFromDB(paramIDs []bson.ObjectID) ([]bson.M, error) {

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

	return results, nil
}
