package services

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// this file is for interactions with the database. Not all database related access may be here but the ones that are pure database accessing functions should be placed in this file.

var ServicesDb *mongo.Collection

type ExternalApiKeys struct {
	Key1     string `bson:"key1,omitempty"`
	Key2     string `bson:"key2,omitempty"`
	UserName string `bson:"username,omitempty"`
	Password string `bson:"password,omitempty"`
}

// One document per user; each service name is a key inside the Services map.
type UserServices struct {
	UserId   string                     `bson:"user_id"`
	Services map[string]ExternalApiKeys `bson:"services"`
}

// AddServiceCredentials upserts credentials for a single service into the user's services document.
// If the user has no services document yet, one is created.
func AddServiceCredentials(userId string, serviceName string, creds ExternalApiKeys) error {
	filter := bson.M{"user_id": userId}
	update := bson.M{
		"$set": bson.M{
			"services." + serviceName: creds,
		},
	}
	_, err := ServicesDb.UpdateOne(context.Background(), filter, update, options.UpdateOne().SetUpsert(true))
	return err
}

// DeleteServiceInfo removes a service entry from the user's services document.
func DeleteServiceInfo(userId string, serviceName string) error {
	filter := bson.M{"user_id": userId}
	update := bson.M{
		"$unset": bson.M{
			"services." + serviceName: "",
		},
	}
	result, err := ServicesDb.UpdateOne(context.Background(), filter, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("no services document found for user")
	}
	return nil
}
