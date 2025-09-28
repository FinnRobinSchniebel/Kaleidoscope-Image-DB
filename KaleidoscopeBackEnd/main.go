package main

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/authutil"
	"context"
	"errors"
	"log"
	"net/http"
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
	KscopeUserId     string        `json:"kscope_userid" bson:"kscope_userid" form:"kscope_userid"`
	// API will send file as well but it will not be placed in the struct: `json: media`
}

var BackendVolumeLocation string

var client *mongo.Client
var db *mongo.Database
var collection *mongo.Collection

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
	collection = db.Collection(ImageDbName)
	authutil.UserCollection = db.Collection(UserDbName)
	authutil.SessionDb = db.Collection((SessionDbName))

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
	app.Get("/api/ImageSets", AuthSessionToken, GetImageSetById)
	app.Post("/api/ImageSets", AuthSessionToken, PostImageSet)
	app.Delete("/api/ImageSets", AuthSessionToken, DeleteImageSets)
	//TODO: Edit imageset api
	//TODO: MarkForDepetion api

	//authentication
	app.Post("/api/session/register", RegisterUser)
	app.Post("/api/session/login", LoginUser)
	app.Post("/api/session/logout", LogoutUser)
	//TODO: User Delete API

	//jwt
	app.Get("/api/session", AuthSessionToken, NewSessionToken)
	app.Delete("/api/session", AuthSessionToken, InvalidateRefreshToken)

	//ImageRetrieve

	//set to listen on port 3000
	app.Listen(":" + serverPort)
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
	//get token for validation
	sessionToken, _ := authutil.GetSessionTokenFromApiHelper(c)
	claims, _ := authutil.VerifyToken(sessionToken)

	//check if user can access the images and remove any images that would not be valid
	iSets, err := GetFromID(paramid...)
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString("could nod get imageset Id from the request")
	}

	var UnauthorizedImageIDs []bson.ObjectID
	for index := range iSets {
		if iSets[index].KscopeUserId != claims.UserID && iSets[index].KscopeUserId != "" {
			UnauthorizedImageIDs = append(UnauthorizedImageIDs, iSets[index].ID)
			iSets[index] = ImageSetMongo{}
		}
	}

	//clean response to avoid backend info reaching the front end and create api Json response
	iSets = CleanImagSetForFrontEnd(iSets...)

	res := fiber.Map{
		"image_sets":       iSets,
		"unauthorized_ids": UnauthorizedImageIDs,
	}

	if err != nil {
		log.Println("Could Not fetch Items from DB")
		return err
	}

	return c.Status(200).JSON(res)

}

func PostImageSet(c *fiber.Ctx) error {

	var imageSet *ImageSetMongo = new(ImageSetMongo)

	sessionToken, err := authutil.GetSessionTokenFromApiHelper(c)

	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString("could not find the user id in the sent token")
	}

	//Note: We assume the token was provided a valid user ID and don't check the database.

	claims, err := authutil.VerifyToken(sessionToken)
	//Note: error check could be removed
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString("Failed to find claims in valid Verification token")
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

	response, hashHits := AddImageSet(imageSet, media, claims.UserID)

	return c.Status(response.errorCode).JSON(fiber.Map{"error": response.errorString, "hash_hits": hashHits})
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

	//get token for validation
	sessionToken, _ := authutil.GetSessionTokenFromApiHelper(c)
	claims, _ := authutil.VerifyToken(sessionToken)

	var UnauthorizedImageIDs []bson.ObjectID

	//If user is not admin check for authority to do deletions to avoid users trying to delete other peoples images
	if !authutil.IsAdmin(claims.UserID) {
		//check if user can access the images and remove any images that would not be valid
		iSets, err := GetFromID(paramid...)
		if err != nil {
			return c.Status(http.StatusInternalServerError).SendString("Could not get ID from the Request")
		}
		if len(iSets) != len(paramid) {
			return c.Status(500).SendString("something has gone wrong with getting image sets from the IDs")
		}

		for index := range iSets {
			if iSets[index].KscopeUserId != claims.UserID {
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

		err = DeleteImageSetInDB(ObjId)
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

func RegisterUser(c *fiber.Ctx) error {

	username := c.FormValue("username")
	password := c.FormValue("password")
	if len(username) < 3 {
		return errors.New("user Name Not long enough")
	}
	if len(password) < 6 {
		return errors.New("password is not long enough")
	}
	_, err := authutil.GetUserByName(username)

	//if quary succeeds return an error
	if err == nil {
		return c.Status(400).SendString("username already in use")
	}

	hashedPassword, err := authutil.HashPassword(password)
	if err != nil {
		return c.Status(400).SendString("bad password")
	}

	newuser := authutil.User{
		Username:       username,
		HashedPassword: hashedPassword,
	}

	log.Println("User: " + newuser.Username + " pass: " + password)

	_, err = authutil.AddUser(newuser)

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
	var userInfo authutil.User
	userInfo, err := authutil.GetUserByName(username)
	if err != nil {
		//return err
		return c.Status(400).SendString("user does not exist")
	}

	if !authutil.ComparePassword(password, userInfo.HashedPassword) {
		return c.Status(400).SendString("username and password did not match")
	}

	/**** create tokens ****/
	sessionToken, _, err := authutil.GenerateToken(userInfo, 15*time.Minute)
	if err != nil {
		//return err
		return err
	}

	//refresh token
	RefreshToken, token, err := authutil.GenerateToken(userInfo, 48*time.Hour)
	if err != nil {
		//return err
		return err
	}

	/**** store refresh token ****/
	err = authutil.StoreRefresh(token, StayLoggedIn)
	if err != nil {
		return err
	}

	/**** send tokens ****/
	c.Cookie(&fiber.Cookie{
		Name:    "refresh_token",
		Value:   RefreshToken,
		Expires: time.Now().Add(time.Minute * 25),
		Path:    "/api/session",
		//Secure:   true,
		HTTPOnly: true,
	})

	res := fiber.Map{
		"session_token": sessionToken,
	}

	return c.Status(200).JSON(res)
}

func AuthSessionToken(c *fiber.Ctx) error {

	sessionToken, err := authutil.GetSessionTokenFromApiHelper(c)
	if err != nil {
		return c.Status(http.StatusUnauthorized).SendString(err.Error())
	}

	_, err = authutil.VerifyToken(sessionToken)
	if err != nil {
		return err
	}

	return c.Next()
}

func NewSessionToken(c *fiber.Ctx) error {

	userRefTok := c.Cookies("refresh_token", "")
	if userRefTok == "" {
		return c.Status(http.StatusBadRequest).SendString("no refresh token given")
	}

	userRefClaim, err := authutil.VerifyToken(userRefTok)
	if err != nil {
		return c.Status(http.StatusBadRequest).SendString("Invalid token")
	}

	serverClaim, _, err := authutil.GetRefreshToken(userRefClaim.ID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString("no session on server")
	}

	if serverClaim.Is_revoked {
		return c.Status(http.StatusUnauthorized).SendString("access revoked")
	}

	if serverClaim.UserID != userRefClaim.UserID {
		return c.Status(http.StatusUnauthorized).SendString("invalid session")
	}

	//creating User without checking db to improve speed (if something passes all of the checks we already assume the request is trustworthy but still user our claim to avoid injection)
	bId, err := bson.ObjectIDFromHex(serverClaim.UserID)
	if err != nil {
		return err
	}

	sessionToken, _, err := authutil.GenerateToken(authutil.User{Id: bId, Username: serverClaim.Subject}, 15*time.Minute)
	if err != nil {
		return err
	}

	res := fiber.Map{
		"session_token": sessionToken,
	}

	log.Println("New session token created for user: " + userRefClaim.UserID)

	return c.Status(200).JSON(res)
}

// Accepts a single id of a refresh token (Must be admin to do so).
// If none is given it will try to invalidate the used token
func InvalidateRefreshToken(c *fiber.Ctx) error {

	userRefTok := c.Cookies("refresh_token", "")
	claim, _ := authutil.VerifyToken(userRefTok)
	tokenId := claim.ID

	param := c.Params("id", "")

	if userRefTok == "" {
		return c.Status(http.StatusBadRequest).SendString("no refresh token provided in request")
	}

	if param != "" && tokenId != param {
		bid, err := bson.ObjectIDFromHex(claim.UserID)
		if err != nil {
			return c.Status(http.StatusBadRequest).SendString("Failed to turn user ID into a valid ID")
		}
		user, err := authutil.GetUserById(bid)

		if err != nil {
			return c.Status(http.StatusBadRequest).SendString("Invalid user ID in token")
		}
		if !user.IsAdmin {
			return c.Status(http.StatusUnauthorized).SendString("Must be Admin to Invalidate another users Token")
		}
		tokenId = param
	}

	err := authutil.InvalidateRefreshTokenById(tokenId)
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString("Could not Invalidate token")
	}

	log.Println("Session token invalidated for user: " + claim.UserID)

	return c.Status(200).SendString("session invalidated successfully")
}

// Uses the Refresh_token to invalidate the session and sends a new session token that expires immediately
// Does not send a new refresh token as any check with the old token will fail anyway.
func LogoutUser(c *fiber.Ctx) error {
	userRefTok := c.Cookies("refresh_token", "")
	claim, _ := authutil.VerifyToken(userRefTok)
	tokenId := claim.ID

	err := authutil.InvalidateRefreshTokenById(tokenId)
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString("Could not Invalidate token")
	}

	bId, err := bson.ObjectIDFromHex(claim.UserID)
	if err != nil {
		return err
	}

	sessionToken, _, err := authutil.GenerateToken(authutil.User{Id: bId, Username: claim.Subject}, 0)
	if err != nil {
		//return err
		return err
	}
	res := fiber.Map{
		"session_token": sessionToken,
	}
	//"User Loggedout succesfully"
	log.Printf("User: %s logged out successfully from session %s", claim.UserID, claim.ID)

	return c.Status(200).JSON(res)
}
