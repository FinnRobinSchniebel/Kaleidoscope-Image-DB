//go:build integration

// Package testdb gives integration tests a throwaway database on the local
// MongoDB. It only builds with -tags integration.
package testdb

import (
	"context"
	"net/url"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// CreateDatabase connects with KSCOPE_TEST_MONGO_USER, KSCOPE_TEST_MONGO_PASSWORD
// and KSCOPE_TEST_MONGO_HOST (default localhost:27017) and returns a new, empty
// database that is dropped when the test ends. It fails the test when the user
// isn't set.
func CreateDatabase(t testing.TB) *mongo.Database {

	t.Helper()
	user := os.Getenv("KSCOPE_TEST_MONGO_USER")
	if user == "" {
		t.Fatal("KSCOPE_TEST_MONGO_USER is not set; integration tests need the local MongoDB")
	}

	host := os.Getenv("KSCOPE_TEST_MONGO_HOST")
	if host == "" {
		host = "localhost:27017"
	}

	uri := (&url.URL{
		Scheme: "mongodb",
		User:   url.UserPassword(user, os.Getenv("KSCOPE_TEST_MONGO_PASSWORD")),
		Host:   host,
	}).String()
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("connecting to the test MongoDB: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx, nil); err != nil {
		client.Disconnect(context.Background())
		t.Fatalf("reaching the test MongoDB at %s: %v", host, err)
	}

	//an ObjectID, not a timestamp: test binaries of several packages run at once
	db := client.Database("kscope_test_" + bson.NewObjectID().Hex())
	t.Cleanup(func() {

		if err := db.Drop(context.Background()); err != nil {
			t.Errorf("dropping test database %s: %v", db.Name(), err)
		}
		client.Disconnect(context.Background())
	})
	return db
}
