package main

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/authutil"
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/tags"
	zipupload "Kaleidoscopedb/Backend/KaleidoscopeBackend/zip_upload"
	"context"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/png"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var client *mongo.Client
var db *mongo.Database

const minSecretKeySize = 32
const ImageDbName = "ImageSets"
const UserDbName = "Users"
const SessionDbName = "Sessions"
const tagDbName = "Tags"
const LowResPathAppend = "low/"
const MaxFileSize = 5 * 1024 * 1024 * 1024

func main() {
	imageset.BackendVolumeLocation = os.Getenv("BACKEND_VOLUME_LOCATION")
	SecretKey := os.Getenv("JWT_SECRET")

	if minSecretKeySize > len(SecretKey) {
		log.Fatalf("Secret Key Must be at least %d character is length", minSecretKeySize)
	}

	authutil.JWTSecret = []byte(SecretKey)

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
	imageset.Collection = db.Collection(ImageDbName)
	authutil.UserCollection = db.Collection(UserDbName)
	authutil.SessionDb = db.Collection((SessionDbName))
	tags.TagsDB = db.Collection(tagDbName)
	imageset.LowResPathAppend = LowResPathAppend

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
	app := fiber.New(fiber.Config{BodyLimit: MaxFileSize})

	app.Use(cors.New(cors.Config{
		AllowOrigins:     "http://localhost:3000",
		AllowHeaders:     "Origin, Content-Type, Accept, session_token",
		AllowCredentials: true,
	}))

	//authentication

	//imageSet upload/retrieval
	app.Get("/api/imagesets", authutil.AuthSessionToken, GetImageSetById)
	app.Post("/api/imagesets", authutil.AuthSessionToken, PostImageSet)
	app.Delete("/api/imagesets", authutil.AuthSessionToken, DeleteImageSets)
	//TODO: Edit imageset api
	//TODO: MarkForDepetion api

	//zip upload
	app.Post("/api/uploadZip", authutil.AuthSessionToken, UploadZip)

	//authentication
	app.Post("/api/session/register", authutil.RegisterUser)
	app.Post("/api/session/login", authutil.LoginUser)
	app.Post("/api/session/logout", authutil.AuthSessionToken, authutil.LogoutUser)
	//TODO: User Delete API

	//jwt
	app.Get("/api/session", authutil.NewSessionToken)
	app.Delete("/api/session", authutil.AuthSessionToken, authutil.InvalidateRefreshToken)

	//ImageRetrieve
	app.Get("/api/image", authutil.AuthSessionToken, GetImageFromID)
	app.Post("/api/search", authutil.AuthSessionToken, FilterForImages)
	app.Get("/api/getimagedata", authutil.AuthSessionToken, ImageInfo)

	app.Get("/api/thumbnail", authutil.AuthSessionToken, imageset.GetThumbnail)

	//tags
	app.Get("/api/getAllTags", authutil.AuthSessionToken, TagRetrieve)
	app.Get("/api/testAutoTag", Testautotag)
	app.Post("/api/addtag", authutil.AuthSessionToken, AddTag)

	//get all author names

	//set to listen on port 3000
	err := app.Listen(":" + serverPort)
	if err != nil {
		log.Fatal(err.Error())
	}
}

// This api Call is to get info about the Image.
// It does not provide the image itself.
func GetImageSetById(c *fiber.Ctx) error {
	//get the ids from the api
	paramIdRaw := c.Context().QueryArgs().PeekMulti("ids")

	var paramid []string
	for _, groupedIds := range paramIdRaw {
		paramid = append(paramid, strings.Split(string(groupedIds), ",")...)
	}
	if paramid == nil {
		return c.Status(400).SendString("Requires an 'ids' param to be sent with the request (eg: ?ids=12345,49325,...)")
	}

	userID := c.Locals("UserID").(string)
	if userID == "" {
		return c.Status(500).SendString("No user ID provided")
	}

	//check if user can access the images and remove any images that would not be valid
	iSets, err := imageset.GetFromID(userID, paramid...)
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString("could not get imageset from the request: " + err.Error())
	}

	//clean response to avoid backend info reaching the front end and create api Json response
	iSets = imageset.CleanImagSetForFrontEnd(iSets...)

	res := fiber.Map{
		"image_sets": iSets,
	}

	if err != nil {
		log.Println("Could Not fetch Items from DB")
		return err
	}

	return c.Status(200).JSON(res)

}

func PostImageSet(c *fiber.Ctx) error {

	var imageSet *imageset.ImageSetMongo = new(imageset.ImageSetMongo)

	userID := c.Locals("UserID").(string)
	if userID == "" {
		return c.Status(500).SendString("No user ID provided")
	}

	if err := c.BodyParser(imageSet); err != nil {
		return err
	}

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

	MedSour := make([]imageset.MediaSource, len(media))
	for i, m := range media {
		MedSour[i] = imageset.MultipartSource{m}
	}

	hashHits, _, response := imageset.AddImageSet(imageSet, MedSour, userID)

	return c.Status(response.ErrorCode).JSON(fiber.Map{"error": response.ErrorString, "hash_hits": hashHits})
}

// takes in one or multiple "ids" in a coma separated list (no spaces)
// returns a list of Ids that were deleted.
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

	userID := c.Locals("UserID").(string)
	if userID == "" {
		return c.Status(500).SendString("No user ID provided")
	}

	var UnauthorizedImageIDs []bson.ObjectID

	//If user is not admin check for authority to do deletions to avoid users trying to delete other peoples images
	if !authutil.IsAdmin(userID) {
		//check if user can access the images and remove any images that would not be valid
		iSets, err := imageset.GetFromID(userID, paramid...)
		if err != nil {
			return c.Status(http.StatusInternalServerError).SendString("Could not get ID from the Request")
		}
		if len(iSets) != len(paramid) {
			return c.Status(500).SendString("something has gone wrong with getting image sets from the IDs")
		}

		for index := range iSets {
			if iSets[index].KscopeUserId != userID {
				UnauthorizedImageIDs = append(UnauthorizedImageIDs, iSets[index].ID)
				//Must remove unauthorized items to avoid deletion during next step
				paramid = append(paramid[:index], paramid[(index+1):]...)
			}
		}
	}

	var DeletedList []string

	var errList error
	for _, id := range paramid {

		ObjId, err := bson.ObjectIDFromHex(id)
		if err != nil {
			errList = errors.Join(errList, err)

			continue
		}

		err = imageset.DeleteImageSetInDB(ObjId)
		if err != nil {
			errList = errors.Join(errList, err)
			continue
		}
		DeletedList = append(DeletedList, id)
	}

	var errorText string
	if errList != nil {
		errorText = errList.Error()
	}

	res := fiber.Map{
		"deleted":      DeletedList,
		"unauthorized": UnauthorizedImageIDs,
		"errors":       errorText,
	}

	if DeletedList != nil && (errList != nil || UnauthorizedImageIDs != nil) {
		return c.Status(http.StatusPartialContent).JSON(res)
	}

	if DeletedList == nil {
		return c.Status(404).JSON(res)
	}

	return c.Status(200).JSON(res)
}

func UploadZip(c *fiber.Ctx) error {

	//Get the zip
	fileHeader, err := c.FormFile("zipFile")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("No File Sent")
	}

	//create form for array grouping
	form, err := c.MultipartForm()
	if err != nil {
		return fiber.ErrBadRequest
	}
	defer form.RemoveAll()

	//Combine all rules for files and zips for easier use
	var ruleLayers []string

	if v := form.Value["structureZip"]; len(v) > 0 {
		ruleLayers = append(ruleLayers, v...)
	}

	if v := form.Value["folders"]; len(v) > 0 {
		ruleLayers = append(ruleLayers, v...)
	}

	for i := range ruleLayers {
		if ruleLayers[i] == "NAN" {
			ruleLayers[i] = ""
		}
	}

	//keep file rules separate and give a default if no instructions are given.
	fileLayer := "[order]"
	if v := form.Value["files"]; len(v) > 0 && v[0] != "" {
		fileLayer = v[0]
	}

	//grouping index
	GroupingIndex, err := strconv.Atoi(c.FormValue("GroupingLevel", "0"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid grouping index")
	}
	userID := c.Locals("UserID").(string)
	if userID == "" {
		return c.Status(500).SendString("No user ID provided")
	}

	code, hashHits, skip, errors, err := zipupload.ProcessZip(fileHeader, ruleLayers, fileLayer, GroupingIndex, userID)

	if err != nil {
		return c.Status(code).SendString(err.Error())
	}

	return c.Status(code).JSON(
		fiber.Map{
			"hash_hits": hashHits,
			"skipped":   skip,
			"errors":    errors,
		},
	)
}

/*
Will take in ONE imagset ID ('image_set_id') and one Index (index) of the image to provide.

	WARNING: this code assumes that the token has already been validated before running the function
	Returns an array of images in the 'images' field
*/
func GetImageFromID(c *fiber.Ctx) error {

	userID := c.Locals("UserID").(string)
	if userID == "" {
		return c.Status(500).SendString("No user ID provided")
	}

	var requestParams struct {
		ImageSetId string `json:"image_set_id" form:"image_set_id" query:"image_set_id"`
		IndexList  int    `json:"index" form:"index" query:"index"`
		LowRes     bool   `json:"lowres" form:"lowres" query:"lowres"`
	}

	err := c.QueryParser(&requestParams)

	log.Println(requestParams)

	if err != nil {
		return c.Status(http.StatusBadRequest).SendString("could not parse request " + err.Error())
	}
	if requestParams.ImageSetId == "" {
		return c.Status(http.StatusBadRequest).SendString("no image set ID provided")
	}

	//user is validated in request
	iset, err := imageset.GetFromID(userID, requestParams.ImageSetId)
	if err != nil || len(iset) == 0 {
		return c.Status(http.StatusNotFound).SendString("imageSet could not be found" + err.Error())
	}

	if requestParams.IndexList >= len(iset[0].Image) || requestParams.IndexList < 0 {
		if len(iset[0].Image) == 0 {
			return c.Status(fiber.StatusNotFound).SendString("The imageSet does not contain images. If this was recently uploaded wait for it to be processed")
		}
		return c.Status(http.StatusBadRequest).SendString("Index out of bounds")
	}

	var imageLink string

	var retImage image.Image
	var retGif *gif.GIF

	if requestParams.LowRes {

		imageLink = iset[0].Image[requestParams.IndexList].LowResName
		log.Println("res link: " + imageLink)
		if imageLink == "" || imageLink == " " {
			retImage, _, _, err = imageset.GenerateLowResFromHigh(iset[0].Path, iset[0].Image[requestParams.IndexList].Name, 720, 0)

			if err != nil {
				return c.Status(500).SendString("failed to create low res image: " + err.Error())
			}
			//todo save image
			go imageset.AddLowresToSetAndStorage(iset[0].Path, iset[0].Title+"_low", retImage, iset[0], requestParams.IndexList)

		} else {
			retImage, retGif, err = imageset.RetrieveLocalImage(iset[0].Path, imageLink, true)
			if err != nil {
				return fmt.Errorf("could not retrieve low res: %s", err)
			}
		}

	} else {
		retImage, retGif, err = imageset.RetrieveLocalImage(iset[0].Path, iset[0].Image[requestParams.IndexList].Name, false)
		if err != nil {
			return fmt.Errorf("could not retrieve image: %s", err)
		}
	}

	if retImage != nil {
		c.Type("png")
		return png.Encode(c.Response().BodyWriter(), retImage)
	} else if retGif != nil {
		c.Type("gif")
		return gif.EncodeAll(c.Response().BodyWriter(), retGif)
	}

	return nil
}

func FilterForImages(c *fiber.Ctx) error {
	var requestParams imageset.SearchParams
	err := c.BodyParser(&requestParams)
	if err != nil {
		return err
	}

	userID := c.Locals("UserID").(string)
	if userID == "" {
		return c.Status(500).SendString("No user ID provided")
	}

	requestParams.User = userID

	// fmt.Printf("tags: %s, authors %s\n", fmt.Sprintf("%s", requestParams.Tags), fmt.Sprintf("%s", requestParams.Author))

	result, err := imageset.SearchDBForImages(requestParams)
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString("an error occurd in the query: " + err.Error())
	}
	res := result

	return c.JSON(res)
}

func ImageInfo(c *fiber.Ctx) error {

	var requestParams struct {
		IDs []string `json:"ids" bson:"ids" form:"ids" query:"ids"`
	}
	err := c.QueryParser(&requestParams)
	fmt.Println(requestParams.IDs)
	if len(requestParams.IDs) == 0 || err != nil {
		return c.Status(http.StatusBadRequest).SendString("no id given")
	}

	var objectIDs []bson.ObjectID
	for _, idStr := range requestParams.IDs {
		oid, err := bson.ObjectIDFromHex(idStr)
		if err != nil {
			return err
		}
		objectIDs = append(objectIDs, oid)
	}

	userID := c.Locals("UserID").(string)
	if userID == "" {
		return c.Status(500).SendString("No user ID provided")
	}

	result, err := imageset.GetImageInfoFromDB(objectIDs, userID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString("an error occurd in the query: " + err.Error())
	}
	res := fiber.Map{
		"imagesets": result,
	}
	return c.JSON(res)
}

// TODO
func TagRetrieve(c *fiber.Ctx) error {

	var requestParams struct {
		IDs []string `json:"ids" bson:"ids" form:"ids" query:"ids"`
	}
	err := c.QueryParser(&requestParams)
	fmt.Println(requestParams.IDs)
	if len(requestParams.IDs) == 0 || err != nil {
		return c.Status(http.StatusBadRequest).SendString("no id given")
	}

	var objectIDs []bson.ObjectID
	for _, idStr := range requestParams.IDs {
		oid, err := bson.ObjectIDFromHex(idStr)
		if err != nil {
			return err
		}
		objectIDs = append(objectIDs, oid)
	}

	// fmt.Printf("tags: %s, authors %s\n", fmt.Sprintf("%s", requestParams.Tags), fmt.Sprintf("%s", requestParams.Author))

	// result, err := imageset.GetImageInfoFromDB(objectIDs)
	// if err != nil {
	// 	return c.Status(http.StatusInternalServerError).SendString("an error occurd in the query: " + err.Error())
	// }
	// res := fiber.Map{
	// 	"imagesets": result,
	// }
	// return c.JSON(res)
	return nil
}

/*
Returns an array of images in the 'images' field
*/
func AddTag(c *fiber.Ctx) error {

	var inputs tags.Tag

	c.BodyParser(&inputs)

	userID := c.Locals("UserID").(string)
	if userID == "" {
		return c.Status(500).SendString("No user ID provided")
	}

	var err error

	inputs.User, err = bson.ObjectIDFromHex(userID)
	if err != nil {
		return err
	}

	err = tags.AddTags(inputs)

	if err != nil {
		return err
	}
	return c.SendStatus(200)
}

func Testautotag(c *fiber.Ctx) error {

	var items struct {
		Tags []string `json:"tags" bson:"tags" form:"tags"`
	}

	err := c.BodyParser(&items)
	if err != nil {
		return err
	}
	if len(items.Tags) == 0 {
		return c.Status(http.StatusBadRequest).SendString("no tags given")
	}

	res, err := tags.FindAutoTag(items.Tags)
	if err != nil {
		return err
	}

	return c.JSON(res)

}
