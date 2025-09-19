package main

import (
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"time"

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
	Title            string        `json:"title" bson:"title,omitempty" form:"title"`
	Tags             []string      `json:"tags" bson:"tags,omitempty" form:"tags"`
	Sources          []SourceInfo  `json:"sources" bson:"sources,omitempty" form:"sources"`
	Authors          []string      `json:"authors" bson:"authors,omitempty" form:"authors"`
	ImageLinks       []string      `json:"images" bson:"images,omitempty" form:"images"`
	LowImageLinks    []string      `json:"low_images" bson:"low_images,omitempty" form:"low_images"`
	ImageHash        []string      `json:"hash" bson:"hash,omitempty" form:"hash"`
	AutoTags         []string      `json:"autotags" bson:"autotags,omitempty" form:"autotags"`
	TagRuleOverrides []string      `json:"tag_rule_overrides" bson:"tag_rule_overrides,omitempty" form:"tag_rule_overrides"`
	Itype            string        `json:"type" bson:"type,omitempty" form:"type"`
	Description      string        `json:"description" bson:"description,omitempty" form:"description"`
	other            string        `json:"other" bson:"other,omitempty" form:"other"`
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

	//Todo: get certificate and enable https

	log.Print("Starting API")
	app := fiber.New()

	app.Get("/api/ImageSets", GetAllImages)

	app.Post("/api/ImageSets", PostImageSet)

	app.Delete("/api/ImageSets", DeleteImageSets)

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

	//A id was sent which is invalid
	if imageSet.ID != bson.NilObjectID {
		//TODO : item sent to wrong api
		return c.Status(400).SendString("Called API to add while trying to update.")
	}

	// parse images from api request
	form, err := c.MultipartForm()
	if err != nil {
		return err
	}

	media := form.File["media"]

	response, hashHits := AddImageSet(imageSet, media)

	return c.Status(response.errorCode).JSON(fiber.Map{"error": response.errorString, "hash_hits": hashHits})
}

func DeleteImageSets(c *fiber.Ctx) error {

	//get all params of type 'ids' and split the param by delimiter "," to get a list of all ids to be deleted
	paramIdRaw := c.Context().QueryArgs().PeekMulti("ids")

	var paramid []string
	for _, groupedIds := range paramIdRaw {
		paramid = append(paramid, strings.Split(string(groupedIds), ",")...)
	}

	log.Println("List of Items to delete:\n" + strings.Join(paramid, ", "))

	if paramid == nil {
		return c.Status(400).SendString("Requires an 'ids' param to be sent with the request (eg: ?ids=12345,49325,...)")
	}

	var DeletedList []string

	var errList error
	for _, id := range paramid {

		ObjId, err := bson.ObjectIDFromHex(id)
		if err != nil {
			errList = errors.Join(errList, err)

			continue
		}

		err = DeleteImageSetInDB(ObjId)
		if err != nil {
			errList = errors.Join(errList, err)
			continue
		}
		DeletedList = append(DeletedList, id)
	}

	if errList != nil {
		return c.JSON(fiber.Map{"deleted": DeletedList, "errors": errList.Error()})
	}

	if DeletedList == nil {
		return c.Status(404).SendString("Invalid IDs: " + strings.Join(paramid, ", "))
	}

	return c.Status(200).JSON(fiber.Map{"deleted": DeletedList})
}
