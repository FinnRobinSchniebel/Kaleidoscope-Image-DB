package notification

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var Collection *mongo.Collection

// EnsureIndexes creates the index a user's notifications are listed by,
// newest first, and a partial index of the pending-update notifications still
// pending, by set (FindPendingImageUpdates). Idempotent, safe to call on every
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

// FindPendingImageUpdates returns the payloads of the pending-update
// notifications still pending for setIDs.
func FindPendingImageUpdates(ctx context.Context, setIDs []bson.ObjectID) ([]PendingImageUpdate, error) {

	if len(setIDs) == 0 {
		return nil, nil
	}

	filter := bson.M{"type": TypePendingImageUpdate, "status": StatusPending, "payload.set_id": bson.M{"$in": setIDs}}
	cursor, err := Collection.Find(ctx, filter, options.Find().SetProjection(bson.M{"payload": 1}))
	if err != nil {
		return nil, fmt.Errorf("finding pending image update notifications: %w", err)
	}

	var stored []struct {
		Payload PendingImageUpdate `bson:"payload"`
	}
	if err := cursor.All(ctx, &stored); err != nil {
		return nil, fmt.Errorf("reading pending image update notifications: %w", err)
	}

	updates := make([]PendingImageUpdate, len(stored))
	for i, n := range stored {
		updates[i] = n.Payload
	}
	return updates, nil
}

// Publish stores p as a new notification for userID.
func Publish(userID string, p Payload) error {
	n := newNotification(userID, p, time.Now())
	if _, err := Collection.InsertOne(context.Background(), n); err != nil {
		return fmt.Errorf("storing %s notification: %w", n.Type, err)
	}
	return nil
}
