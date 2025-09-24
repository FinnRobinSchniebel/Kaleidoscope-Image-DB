package main

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/authUtil"
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
	ImageLinks       []string      `json:"images,omitempty" bson:"images,omitempty" form:"images"`
	LowImageLinks    []string      `json:"low_images,omitempty" bson:"low_images,omitempty" form:"low_images"`
	ImageHash        []string      `json:"hash" bson:"hash,omitempty" form:"hash"`
	AutoTags         []string      `json:"autotags" bson:"autotags,omitempty" form:"autotags"`
	TagRuleOverrides []string      `json:"tag_rule_overrides" bson:"tag_rule_overrides,omitempty" form:"tag_rule_overrides"`
	Itype            string        `json:"type" bson:"type,omitempty" form:"type"`
	Description      string        `json:"description" bson:"description,omitempty" form:"description"`
	Other            string        `json:"other" bson:"other,omitempty" form:"other"`
	// API will send file as well but it will not be placed in the struct: `json: media`
}

var BackendVolumeLocation string

var client *mongo.Client
var db *mongo.Database
var collection *mongo.Collection
var userCollection *mongo.Collection
var SessionDb *mongo.Collection

const minSecretKeySize = 32
const ImageDbName = "ImageSets"
const UserDbName = "Users"
const SessionDbName = "Sessions"

func main() {
	BackendVolumeLocation = os.Getenv("BACKEND_VOLUME_LOCATION")
	SecretKey := os.Getenv("JWT_SECRET")

	if minSecretKeySize > len(SecretKey) {
		log.Fatalf("Secret Key Must be at least %d character is length", minSecretKeySize)
	}

	authUtil.JWTSecret = []byte(SecretKey)

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
	userCollection = db.Collection(UserDbName)
	SessionDb = db.Collection((SessionDbName))

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

	//authentication

	//imageSet retrievel
	app.Get("/api/ImageSets", GetImageSetById)

	app.Post("/api/ImageSets", PostImageSet)

	app.Delete("/api/ImageSets", DeleteImageSets)

	app.Post("/api/register", RegisterUser)
	app.Post("/api/login", LoginUser)

	//set to listen on port 3000
	app.Listen(":" + serverPort)
}

// This api Call is to get info about the Image.
// It does not provide the image itself.
func GetImageSetById(c *fiber.Ctx) error {

	err := authUtil.BasicAuthorize(c, userCollection)
	if err != nil {
		return err
	}

	paramIdRaw := c.Context().QueryArgs().PeekMulti("ids")

	var paramid []string
	for _, groupedIds := range paramIdRaw {
		paramid = append(paramid, strings.Split(string(groupedIds), ",")...)
	}
	if paramid == nil {
		return c.Status(400).SendString("Requires an 'ids' param to be sent with the request (eg: ?ids=12345,49325,...)")
	}
	iSets, err := GetFromID(paramid...)
	iSets = CleanImagSetForFrontEnd(iSets...)
	if err != nil {
		log.Println("Could Not fetch Items from DB")
		return err
	}

	return c.Status(200).JSON(iSets)

}

func PostImageSet(c *fiber.Ctx) error {

	err := authUtil.BasicAuthorize(c, userCollection)
	if err != nil {
		return err
	}

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

	err := authUtil.BasicAuthorize(c, userCollection)
	if err != nil {
		return err
	}

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

func RegisterUser(c *fiber.Ctx) error {

	username := c.FormValue("username")
	password := c.FormValue("password")
	if len(username) < 3 {
		return errors.New("user Name Not long enough")
	}
	if len(password) < 6 {
		return errors.New("password is not long enough")
	}
	var single authUtil.User

	err := userCollection.FindOne(context.Background(), bson.M{"username": username}).Decode(single)
	if err != mongo.ErrNoDocuments {
		return c.Status(400).SendString("username already in use")
	}

	hashedPassword, err := authUtil.HashPassword(password)
	if err != nil {
		return c.Status(400).SendString("bad password")
	}

	newuser := authUtil.User{
		Username:       username,
		HashedPassword: hashedPassword,
	}

	log.Println("User: " + newuser.Username + " pass: " + password)

	_, err = userCollection.InsertOne(context.Background(), newuser)

	if err != nil {
		return err
	}

	return c.SendStatus(200)
}

func LoginUser(c *fiber.Ctx) error {

	username := c.FormValue("username")
	password := c.FormValue("password")
	StayLoggedIn := c.QueryBool("stay_logged_in")

	/**** Check valid login ****/
	var userInfo authUtil.User
	err := userCollection.FindOne(context.Background(), bson.M{"username": username}).Decode(&userInfo)
	if err != nil {
		//return err
		return c.Status(400).SendString("user does not exist")
	}

	if !authUtil.ComparePassword(password, userInfo.HashedPassword) {
		return c.Status(400).SendString("username and password did not match")
	}

	/**** create tokens ****/
	sessionToken, _, err := authUtil.GenerateToken(userInfo)
	if err != nil {
		//return err
		return err
	}

	RefreshToken, token, err := authUtil.GenerateToken(userInfo)
	if err != nil {
		//return err
		return err
	}

	/**** store refresh token ****/
	err = StoreRefresh(token, StayLoggedIn)
	if err != nil {
		return err
	}

	/**** send tokens ****/
	c.Cookie(&fiber.Cookie{
		Name:    "token",
		Value:   RefreshToken,
		Expires: time.Now().Add(time.Minute * 25),
		Path:    "/session",
		//Secure:   true,
		HTTPOnly: true,
	})

	res := fiber.Map{
		"session_token": sessionToken,
	}

	return c.Status(200).JSON(res)

	// sessionToken := generateToken(32)
	// csrfToken := generateToken(32)

	// c.Cookie(&fiber.Cookie{
	// 	Name:     "session_token",
	// 	Value:    sessionToken,
	// 	Expires:  time.Now().Add(48 * time.Hour),
	// 	HTTPOnly: true,
	// })
	// c.Cookie(&fiber.Cookie{
	// 	Name:     "csrf_token",
	// 	Value:    csrfToken,
	// 	Expires:  time.Now().Add(48 * time.Hour),
	// 	HTTPOnly: false,
	// })
	// userInfo.CsrfToken = csrfToken
	// userInfo.SessionCookie = sessionToken

	// _, err = userCollection.UpdateOne(context.Background(), bson.M{"_id": userInfo.Id}, bson.M{"$set": userInfo})

	// if err != nil {
	// 	return err
	// }

	return c.SendStatus(200)
}

func NewRefreshToken(c *fiber.Ctx, userInfo authUtil.User) error {

	//refTok, _, err := authUtil.GenerateToken(userInfo)

	// if err != nil {
	// 	return err
	// }

	return nil
}
