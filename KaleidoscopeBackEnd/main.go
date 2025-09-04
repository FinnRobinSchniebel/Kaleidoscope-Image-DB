package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type ImageSetMongo struct {
	ID               primitive.ObjectID `json:"id" bson:"_id"`
	Title            string             `json:"title"`
	Tags             []string           `json:"tags"`
	Sources          []string           `json:"sources"`
	Author           []string           `json:"author"`
	ImageLinks       []string           `json:"images"`
	LowImageLinks    []string           `json:"lowimage"`
	ImageHash        []string           `json:"Hash"`
	AutoTags         []string           `json:"autotags"`
	TagRuleOverrides []string           `json:"tagruleoverrides"`
	Itype            []string           `json:"type"`
	Description      string             `json:"description"`
	other            string             `json:"other"`
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
	form, err := c.MultipartForm()
	if err != nil {
		return err
	}
	media := form.File["media"]
	for _, item := range media {
		fmt.Println(item.Filename, item.Size, item.Header["content-Type"])

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
		// err := c.SaveFile(item, fmt.Sprintf(BackendVolumeLocation, item.Filename))
		// if err != nil {
		// 	return err
		// }
	}
	return nil
}
