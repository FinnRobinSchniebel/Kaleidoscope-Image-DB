package main

import (
	//"KaleidoscopeBackend/utility"
	"context"
	"fmt"
	"image"
	"log"
	"os"
	"time"

	"github.com/ajdnik/imghash"
	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type SourceInfo struct {
	Name     string   `json:"name" form:"name"`
	ID       string   `json:"id" form:"id"`
	Title    string   `json:"title" form:"title"`
	SourceID string   `json:"sourceid" form:"sourceid"`
	Tags     []string `json:"tags" form:"tags"`
}

type ImageSetMongo struct {
	ID               bson.ObjectID `json:"id,omitempty" bson:"_id,omitempty" form:"id,omitempty"`
	Title            string        `json:"title" form:"title"`
	Tags             []string      `json:"tags" form:"tags"`
	Sources          []SourceInfo  `json:"source" form:"source"`
	Authors          []string      `json:"authors" form:"authors"`
	ImageLinks       []string      `json:"images" form:"images"`
	LowImageLinks    []string      `json:"lowimage" form:"lowimage"`
	ImageHash        []string      `json:"hash" form:"hash"`
	AutoTags         []string      `json:"autotags" form:"autotags"`
	TagRuleOverrides []string      `json:"tagruleoverrides" form:"tagruleoverrides"`
	Itype            string        `json:"type" form:"type"`
	Description      string        `json:"description" form:"description"`
	other            string        `json:"other" form:"other"`
	// API will send file as well but it will not be placed in the struct: `json: media`
}

var ImageDbName string = "ImageSets"
var BackendVolumeLocation string

var client *mongo.Client
var db *mongo.Database
var collection *mongo.Collection

func main() {
	BackendVolumeLocation = os.Getenv("BACKEND_VOLUME_LOCATION")
	ConnectDB()
	defer client.Disconnect(context.Background())
	StartAPI()

}

func ConnectDB() {
	//set up a basic connection timout
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	//get server URL for connection
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		log.Fatal("MONGO_URI not set")
	}

	//Connect to the mongoDB and catch errors
	client, err := mongo.Connect(options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatal(err)
	}

	db = client.Database("KaleidoScopedb")

	//points to the collection and creates it if none exists
	collection = db.Collection(ImageDbName)

	log.Print("Connected, no issues ---------------------")

}

func StartAPI() {
	serverPort := os.Getenv("SERVERPORT")
	if serverPort == "" {
		log.Print("No Port")
		serverPort = "3000"
	}

	log.Print("Starting API")
	app := fiber.New()

	app.Get("/api/ImageSets", GetAllImages)

	app.Post("/api/ImageSets", PostImageSet)

	//set to listen on port 3000
	app.Listen(":" + serverPort)
}

func GetAllImages(c *fiber.Ctx) error {
	var imageSets []ImageSetMongo

	cursor, err := collection.Find(context.Background(), bson.M{})

	if err != nil {
		return err
	}
	defer cursor.Close(context.Background())

	for cursor.Next(context.Background()) {
		var imageSet ImageSetMongo
		if err := cursor.Decode(&imageSet); err != nil {
			return err
		}
		imageSets = append(imageSets, imageSet)
	}

	return c.JSON(imageSets)
}

func PostImageSet(c *fiber.Ctx) error {

	var imageSet *ImageSetMongo = new(ImageSetMongo)

	if err := c.BodyParser(imageSet); err != nil {
		return err
	}
	// imageSet.ID = primitive.NilObjectID

	if imageSet.ID != bson.NilObjectID {
		//TODO : item sent to wrong api
	}

	//add to DB
	insertResult, err := collection.InsertOne(context.Background(), imageSet)

	if err != nil {
		return err
	}

	imageSet.ID = insertResult.InsertedID.(bson.ObjectID)

	// download images to local storage
	form, err := c.MultipartForm()
	if err != nil {
		return err
	}

	media := form.File["media"]

	//determine folder path
	filePath, err := MakeFileDirectory(imageSet.Authors[0])
	if err != nil {
		return err
	}

	if len(media) == 0 {
		//Todo: send proper feedback
		return c.JSON(imageSet)
	}

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
		fullPath := fmt.Sprintf("%s/%s", filePath, fileName)

		log.Print("FilePath: " + fullPath)

		err = c.SaveFile(item, fullPath)
		if err != nil {
			return err
		}
		imageSet.ImageLinks = append(imageSet.ImageLinks, fullPath)

		/** 	get hash 	**/
		file, _ := item.Open()

		img, _, err := image.Decode(file)
		if err != nil {
			os.Remove(BackendVolumeLocation + item.Filename)
			return err
		}
		phash := imghash.NewPHash()
		ihash := phash.Calculate(img)
		fmt.Printf("Hashed to: %v\n", ihash)
		imageSet.ImageHash = append(imageSet.ImageHash, ihash.String())
		file.Close()
	}
	log.Print("Files Uploaded")

	update := bson.M{"$set": imageSet}
	log.Print("Test 1 ++++")
	result, err := collection.UpdateByID(context.Background(), imageSet.ID, update)
	if err != nil {
		fmt.Println("Update Failed")
		return err
	}
	log.Print("Test 2 ++++")
	if result.MatchedCount == 0 {
		log.Print("COULD NOT UPDATE FILE AFTER ADDING INFO")

	}
	log.Print("---Upload complete---")
	return c.JSON(imageSet)
}
