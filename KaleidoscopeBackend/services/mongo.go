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
	Key1              string `json:"key1,omitempty"               bson:"key1,omitempty"`
	Key2              string `json:"key2,omitempty"               bson:"key2,omitempty"`
	UserName          string `json:"username,omitempty"           bson:"username,omitempty"`
	Password          string `json:"password,omitempty"           bson:"password,omitempty"`
	SyncIntervalHours int64  `json:"sync_interval_hours,omitempty" bson:"sync_interval_hours,omitempty"` // 0 = no schedule
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

// GetAllUsersWithService returns the services document for every user that has
// the named service registered. Used at startup to restore periodic schedules.
func GetAllUsersWithService(serviceName string) ([]UserServices, error) {
	filter := bson.M{"services." + serviceName: bson.M{"$exists": true}}
	cursor, err := ServicesDb.Find(context.Background(), filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())
	var docs []UserServices
	if err := cursor.All(context.Background(), &docs); err != nil {
		return nil, err
	}
	return docs, nil
}

// GetServiceCredentials returns the stored credentials for a single service.
func GetServiceCredentials(userId string, serviceName string) (*ExternalApiKeys, error) {
	filter := bson.M{"user_id": userId}
	var doc UserServices
	if err := ServicesDb.FindOne(context.Background(), filter).Decode(&doc); err != nil {
		return nil, err
	}
	creds, ok := doc.Services[serviceName]
	if !ok {
		return nil, fmt.Errorf("service %q not registered for user", serviceName)
	}
	return &creds, nil
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
