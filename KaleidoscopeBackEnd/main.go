package main

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/authutil"
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/services"
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/tagging"
	zipupload "Kaleidoscopedb/Backend/KaleidoscopeBackend/zip_upload"

	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var client *mongo.Client
var db *mongo.Database

const minSecretKeySize = 32
const ImageDbName = "ImageSets"
const UserDbName = "Users"
const SessionDbName = "Sessions"
const sourceTagsDbName = "SourceTags"
const autoTagsDbName = "AutoTags"
const servicesDbName = "services"

// const notificationDbName = "notifications"
const LowResPathAppend = "low/"
const MaxFileSize = 5 * 1024 * 1024 * 1024

// readSecret reads a Docker/Compose file secret mounted at /run/secrets/<name>,
// trimming surrounding whitespace so a trailing newline from the secret file
// doesn't silently become part of the value.
func readSecret(name string) (string, error) {
	data, err := os.ReadFile("/run/secrets/" + name)
	if err != nil {
		return "", fmt.Errorf("reading secret %q: %w", name, err)
	}
	return strings.TrimSpace(string(data)), nil
}

func main() {
	imageset.BackendVolumeLocation = os.Getenv("BACKEND_VOLUME_LOCATION")

	SecretKey, err := readSecret("jwt_secret")
	if err != nil {
		log.Fatal(err)
	}
	if minSecretKeySize > len(SecretKey) {
		log.Fatalf("Secret Key Must be at least %d character is length", minSecretKeySize)
	}

	authutil.JWTSecret = []byte(SecretKey)

	PasswordPepper, err := readSecret("password_pepper")
	if err != nil {
		log.Fatal(err)
	}
	if minSecretKeySize > len(PasswordPepper) {
		log.Fatalf("Password Pepper Must be at least %d character is length", minSecretKeySize)
	}

	authutil.PasswordPepper = []byte(PasswordPepper)

	if err := authutil.SetCookieSecurityMode(os.Getenv("COOKIE_SECURITY_MODE")); err != nil {
		log.Fatal(err)
	}
	log.Printf("Cookie security mode: Secure=%t SameSite=%s (set COOKIE_SECURITY_MODE=%s if a TLS-terminating proxy sits in front of this app, or =%s if the frontend is on a different domain)",
		authutil.CookieSecure, authutil.CookieSameSite, authutil.CookieModeSecure, authutil.CookieModeCrossSite)

	ConnectDB()
	defer client.Disconnect(context.Background())
	StartServices()
	StartAPI()
}

func ConnectDB() {
	//set up a basic connection timout
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	//build the connection URI from the mongo user/password secret files
	mongoUser, err := readSecret("mongo_user")
	if err != nil {
		log.Fatal(err)
	}
	mongoPassword, err := readSecret("mongo_password")
	if err != nil {
		log.Fatal(err)
	}

	mongoURI := (&url.URL{
		Scheme: "mongodb",
		User:   url.UserPassword(mongoUser, mongoPassword),
		Host:   "db:27017",
	}).String()

	//Connect to the mongoDB and catch errors
	client, err = mongo.Connect(options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatal(err)
	}

	db = client.Database("KaleidoScopedb")

	//points to the collection and creates it if none exists
	imageset.Collection = db.Collection(ImageDbName)
	authutil.UserCollection = db.Collection(UserDbName)
	authutil.SessionDb = db.Collection((SessionDbName))
	tagging.SourceTagsDB = db.Collection(sourceTagsDbName)
	tagging.AutoTagsDB = db.Collection(autoTagsDbName)
	services.ServicesDb = db.Collection(servicesDbName)
	imageset.LowResPathAppend = LowResPathAppend
	imageset.Tagger = tagging.AutoTagFunc{}

	if err := imageset.EnsureIndexes(context.Background()); err != nil {
		log.Fatal(err)
	}
	if err := tagging.EnsureIndexes(context.Background()); err != nil {
		log.Fatal(err)
	}

	log.Print("Connected, no issues ---------------------")

}

// StartServices registers all external service integrations with the scheduler
// and starts the background worker. Add a RegisterProvider call here for each new service.
func StartServices() {
	services.DefaultScheduler.RegisterProvider(&services.PixivProvider{})
	services.DefaultScheduler.Start()
	services.DefaultScheduler.RestoreAllSchedules()
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
		AllowHeaders:     "Origin, Content-Type, Accept, Session-Token",
		AllowCredentials: true,
	}))

	//authentication

	//imageSet upload/retrieval
	app.Get("/api/imagesets", authutil.AuthSessionToken, imageset.GetImageSetById)
	app.Post("/api/imagesets", authutil.AuthSessionToken, imageset.PostImageSet)
	app.Delete("/api/imagesets", authutil.AuthSessionToken, imageset.DeleteImageSets)
	//TODO: Edit imageset api
	//TODO: MarkForDepetion api

	//zip upload
	app.Post("/api/uploadZip", authutil.AuthSessionToken, zipupload.UploadZip)

	//authentication
	app.Post("/api/session/register", authutil.RegisterUser)
	app.Post("/api/session/login", authutil.LoginUser)
	app.Post("/api/session/logout", authutil.AuthSessionToken, authutil.LogoutUser)
	//TODO: User Delete API

	//jwt
	app.Get("/api/session", authutil.NewSessionToken)
	app.Delete("/api/session", authutil.AuthSessionToken, authutil.InvalidateRefreshToken)

	//ImageRetrieve
	app.Get("/api/image", authutil.AuthSessionToken, imageset.GetImageFromID)
	app.Post("/api/search", authutil.AuthSessionToken, imageset.FilterForImageSets)
	app.Get("/api/getimagedata", authutil.AuthSessionToken, imageset.GetImageInfo)

	app.Get("/api/thumbnail", authutil.AuthSessionToken, imageset.GetThumbnail)

	//source tags (imported tags, read-only from the frontend)
	app.Get("/api/sourcetags/search", authutil.AuthSessionToken, tagging.SearchSourceTagsHandler)
	app.Get("/api/sourcetags", authutil.AuthSessionToken, tagging.ListSourceTagsHandler)

	//auto tags
	app.Get("/api/autotags", authutil.AuthSessionToken, tagging.ListAutoTagsHandler)
	app.Get("/api/autotags/details", authutil.AuthSessionToken, tagging.ListAutoTagDetailsHandler)
	app.Post("/api/autotags", authutil.AuthSessionToken, tagging.CreateAutoTagHandler)
	app.Patch("/api/autotags/:id", authutil.AuthSessionToken, tagging.UpdateAutoTagHandler)
	app.Delete("/api/autotags/:id", authutil.AuthSessionToken, tagging.DeleteAutoTagHandler)
	//TODO: per-image-set "refresh tags" trigger (re-check one set's tags against current AutoTags)

	//services
	app.Get("/api/service/services", authutil.AuthSessionToken, services.ListServices) //lists all services with if the user has registered with it
	app.Post("/api/service/:name/register", authutil.AuthSessionToken, services.Register)
	app.Get("/api/service/:name/key", authutil.AuthSessionToken, services.GetKeys)
	app.Post("/api/service/:name/sync", authutil.AuthSessionToken, services.SyncService)
	app.Get("/api/service/:name/syncSchedule", authutil.AuthSessionToken, services.GetServiceSyncInfo)
	app.Post("/api/service/:name/syncSchedule", authutil.AuthSessionToken, services.SetServiceSyncSchedule)
	app.Delete("/api/service/:name", authutil.AuthSessionToken, services.RemoveService)
	//special service
	app.Post("/api/service/pixivconnect", authutil.AuthSessionToken, services.PixivConnect)

	//get all author names

	//set to listen on port 3000
	err := app.Listen(":" + serverPort)
	if err != nil {
		log.Fatal(err.Error())
	}
}
