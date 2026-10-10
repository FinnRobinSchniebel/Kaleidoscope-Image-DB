package notification

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

var Collection *mongo.Collection

// EnsureIndexes creates the index a user's notifications are listed by,
// newest first. Idempotent, safe to call on every startup.
func EnsureIndexes(ctx context.Context) error {
	_, err := Collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "created_at", Value: -1}},
	})
	return err
}

// Publish stores p as a new notification for userID.
func Publish(userID string, p Payload) error {
	n := newNotification(userID, p, time.Now())
	if _, err := Collection.InsertOne(context.Background(), n); err != nil {
		return fmt.Errorf("storing %s notification: %w", n.Type, err)
	}
	return nil
}
