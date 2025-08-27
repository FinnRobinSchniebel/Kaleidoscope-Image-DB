package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Todo struct {
	ID        int    `json:"id"`
	Completed bool   `json:"completed"`
	Body      string `json:"body"`
}

var collection *mongo.Collection

func main() {

	ConnectDB()
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

	collection = client.Database("golang_db").Collection("todos")

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

	todos := []Todo{}

	//test get request
	app.Get("/", func(c *fiber.Ctx) error {
		return c.Status(200).JSON(fiber.Map{"msg": "hello world"})
	})

	//test post request
	app.Post("/api/todos", func(c *fiber.Ctx) error {
		todo := &Todo{}
		if err := c.BodyParser(todo); err != nil {
			return err
		}

		if todo.Body == "" {
			return c.Status(400).JSON(fiber.Map{"error": "Todo body is required"})
		}
		todo.ID = len(todos) + 1
		todos = append(todos, *todo)

		return c.Status(201).JSON(todo)
	})

	//set to listen on port 3000
	app.Listen(":" + serverPort)
}
