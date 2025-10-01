package imageset

import (
	"context"
	"errors"
	"fmt"
	"image"
	"log"
	"mime/multipart"
	"os"
	"strings"

	"github.com/ajdnik/imghash"
	"github.com/valyala/fasthttp"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

var Collection *mongo.Collection

type CollisionResponsePair struct {
	IdOfHashCollision bson.ObjectID
	ImageNumber       int
}

func findOverlappingHashes(hash string) ([]CollisionResponsePair, error) {
	cursor, err := Collection.Find(context.Background(), bson.D{{"hash", hash}})
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
		for index, imageH := range item.ImageHash {
			if imageH == hash {
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

func AddImageSet(imageSet *ImageSetMongo, media []*multipart.FileHeader, userId string) (InternalResponse, map[int][]CollisionResponsePair) {

	//clean file paths to avoid unauthorized access
	imageSet.Image = nil
	//imageSet.LowImage = nil
	imageSet.ImageHash = nil
	imageSet.KscopeUserId = ""

	//add userId (done as seperate step to avoid exploits if changes are made)
	imageSet.KscopeUserId = userId

	//add to DB
	insertResult, err := Collection.InsertOne(context.Background(), imageSet)

	if err != nil {
		return InternalResponse{500, err.Error()}, nil
	}

	imageSet.ID = insertResult.InsertedID.(bson.ObjectID)

	//determine folder path
	filePath, err := MakeFileDirectory(imageSet.Authors[0])
	if err != nil {
		return InternalResponse{500, err.Error()}, nil
	}

	if len(media) == 0 {
		return InternalResponse{400, "No Media attached"}, nil
	}

	hashHits := make(map[int][]CollisionResponsePair)

	for index, item := range media {
		fmt.Println(item.Filename, item.Size, item.Header["Content-Type"][0])

		/**		Test FilePath	 **/
		_, err := os.Stat(BackendVolumeLocation)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("File or directory does not exist at: %s\n", BackendVolumeLocation)
			} else {
				fmt.Printf("Error accessing path %s: %v\n", BackendVolumeLocation, err)
			}
		} else {
			fmt.Printf("File or directory exists at: %s\n", BackendVolumeLocation)
		}

		/**		save media		**/

		fileName := ImageFileName(imageSet.Title, imageSet.ID, index, getType(item.Filename))
		fullPath := fmt.Sprintf("%s%s", filePath, fileName)

		log.Print("FilePath: " + fullPath)
		err = fasthttp.SaveMultipartFile(item, fullPath)
		//err = SaveMultipartFile(item, fullPath)
		if err != nil {
			return InternalResponse{500, err.Error()}, nil
		}
		imageSet.Image = append(imageSet.Image, ImageInfo{Name: fileName, IsImageActive: true})
		//imageSet.IsImageActive = append(imageSet.IsImageActive, true)

		/** 	get hash 	**/
		file, err := item.Open()
		if err != nil {
			return InternalResponse{500, err.Error()}, nil
		}

		img, _, err := image.Decode(file)
		if err != nil {
			os.Remove(fullPath)
			return InternalResponse{500, err.Error()}, nil
		}
		phash := imghash.NewPHash()
		ihash := phash.Calculate(img)
		fmt.Printf("Hashed to: %v\n", ihash)
		imageSet.ImageHash = append(imageSet.ImageHash, ihash.String())
		file.Close()

		//compare hash in DB

		HitResults, err := findOverlappingHashes(ihash.String())

		if err != nil {
			return InternalResponse{500, err.Error()}, nil
		}
		if len(HitResults) != 0 {
			fmt.Println("Hash Hit")
			hashHits[index] = HitResults
		}

		//cursor, err := collection.Find(context.Background(), bson.M{"hash": ihash.String()})

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
