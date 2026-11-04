package notification

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var Collection *mongo.Collection

// EnsureIndexes creates the index a user's notifications are listed by,
// newest first, and a partial index of the pending-update notifications still
// pending, by set (findPendingUpdateKeys). Idempotent, safe to call on every
// startup.
func EnsureIndexes(ctx context.Context) error {
	_, err := Collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "created_at", Value: -1}}},
		{
			Keys:    bson.D{{Key: "payload.set_id", Value: 1}},
			Options: options.Index().SetPartialFilterExpression(bson.M{"type": TypePendingImageUpdate, "status": StatusPending}),
		},
	})
	return err
}

// findPendingUpdateKeys returns the keys of the pending-update notifications
// still pending for setIDs.
func findPendingUpdateKeys(ctx context.Context, setIDs []bson.ObjectID) ([]PendingUpdateKey, error) {

	if len(setIDs) == 0 {
		return nil, nil
	}

	filter := bson.M{"type": TypePendingImageUpdate, "status": StatusPending, "payload.set_id": bson.M{"$in": setIDs}}
	projection := bson.M{"payload.set_id": 1, "payload.service": 1, "payload.source_id": 1}
	cursor, err := Collection.Find(ctx, filter, options.Find().SetProjection(projection))
	if err != nil {
		return nil, fmt.Errorf("finding pending image update notifications: %w", err)
	}

	var stored []struct {
		Payload PendingUpdateKey `bson:"payload"`
	}
	if err := cursor.All(ctx, &stored); err != nil {
		return nil, fmt.Errorf("reading pending image update notifications: %w", err)
	}

	keys := make([]PendingUpdateKey, len(stored))
	for i, n := range stored {
		keys[i] = n.Payload
	}
	return keys, nil
}

// UserPendingImageUpdate is a pending-update payload and the user it's
// published for.
type UserPendingImageUpdate struct {
	UserID string
	Update PendingImageUpdate
}

// PublishUnmatchedPendingImageUpdates publishes each of wanted whose key has no
// notification still pending, and returns how many it published. A failed
// publish is logged and left for the next call; a failed lookup publishes
// nothing.
func PublishUnmatchedPendingImageUpdates(ctx context.Context, wanted []UserPendingImageUpdate) (int, error) {

	setIDs := make([]bson.ObjectID, len(wanted))
	for i, w := range wanted {
		setIDs[i] = w.Update.SetID
	}

	keys, err := findPendingUpdateKeys(ctx, setIDs)
	if err != nil {
		return 0, err
	}

	notified := make(map[PendingUpdateKey]bool, len(keys))
	for _, key := range keys {
		notified[key] = true
	}

	published := 0
	for _, w := range wanted {

		key := w.Update.PendingUpdateKey
		if notified[key] {
			continue
		}

		if err := Publish(w.UserID, w.Update); err != nil {
			log.Printf("------ Warning: publishing the missing pending update for %s/%s in set %s: %s ------", key.Service, key.SourceID, key.SetID.Hex(), err)
			continue
		}
		published++
	}
	return published, nil
}

// Publish stores p as a new notification for userID.
func Publish(userID string, p Payload) error {
	n := newNotification(userID, p, time.Now())
	if _, err := Collection.InsertOne(context.Background(), n); err != nil {
		return fmt.Errorf("storing %s notification: %w", n.Type, err)
	}
	return nil
}
