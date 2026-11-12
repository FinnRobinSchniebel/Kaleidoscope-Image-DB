package notification

import (
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"slices"
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

// livePendingUpdateStatuses are the statuses a pending-update notification can
// still be settled from.
var livePendingUpdateStatuses = bson.M{"$in": []Status{StatusPending, StatusDeclined}}

// storedPendingImageUpdate is a stored pending-update notification, read back to
// be settled.
type storedPendingImageUpdate struct {
	ID        bson.ObjectID      `bson:"_id"`
	UserID    string             `bson:"user_id"`
	CreatedAt time.Time          `bson:"created_at"`
	ReadAt    time.Time          `bson:"read_at,omitempty"`
	Status    Status             `bson:"status"`
	Payload   PendingImageUpdate `bson:"payload"`
}

// findLivePendingImageUpdates returns userID's pending-update notifications
// still pending or declined under any of keys.
func findLivePendingImageUpdates(ctx context.Context, userID string, keys []PendingUpdateKey) ([]storedPendingImageUpdate, error) {

	if len(keys) == 0 {
		return nil, nil
	}

	matchKeys := make(bson.A, len(keys))
	for i, key := range keys {
		matchKeys[i] = bson.M{"payload.set_id": key.SetID, "payload.service": key.Service, "payload.source_id": key.SourceID}
	}

	filter := bson.M{"user_id": userID, "type": TypePendingImageUpdate, "status": livePendingUpdateStatuses, "$or": matchKeys}
	cursor, err := Collection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("finding live pending image update notifications: %w", err)
	}

	var live []storedPendingImageUpdate
	if err := cursor.All(ctx, &live); err != nil {
		return nil, fmt.Errorf("reading live pending image update notifications: %w", err)
	}
	return live, nil
}

// settleOnePendingImageUpdate makes n follow its copy to sets: with none it's
// invalidated; otherwise it's retargeted to sets[0], unless its own set is among
// them, and every other set gets a copy keeping its status and dates. Writes
// only take n while it's still live, and a retarget that misses copies nothing.
func settleOnePendingImageUpdate(ctx context.Context, n storedPendingImageUpdate, sets []bson.ObjectID, now time.Time) error {

	stillLive := bson.M{"_id": n.ID, "user_id": n.UserID, "status": livePendingUpdateStatuses}
	if len(sets) == 0 {

		if _, err := Collection.UpdateOne(ctx, stillLive, bson.M{"$set": bson.M{"status": StatusInvalid, "updated_at": now}}); err != nil {
			return fmt.Errorf("invalidating notification %s: %w", n.ID.Hex(), err)
		}
		return nil
	}

	copyTo := sets
	if !slices.Contains(sets, n.Payload.SetID) {

		result, err := Collection.UpdateOne(ctx, stillLive, bson.M{"$set": bson.M{"payload.set_id": sets[0], "updated_at": now}})
		if err != nil {
			return fmt.Errorf("retargeting notification %s: %w", n.ID.Hex(), err)
		}

		if result.MatchedCount == 0 {
			return nil //resolved since it was read
		}
		copyTo = sets[1:]
	}

	var errs []error
	for _, setID := range copyTo {

		if setID == n.Payload.SetID {
			continue
		}

		payload := n.Payload
		payload.SetID = setID
		copied := newNotification(n.UserID, payload, now)
		copied.CreatedAt, copied.ReadAt, copied.Status = n.CreatedAt, n.ReadAt, n.Status
		if _, err := Collection.InsertOne(ctx, copied); err != nil {
			errs = append(errs, fmt.Errorf("copying notification %s to set %s: %w", n.ID.Hex(), setID.Hex(), err))
		}
	}
	return errors.Join(errs...)
}

// SettlePendingImageUpdates makes userID's pending-update notifications follow
// their copies after a merge. locations maps each origin, the key a notification
// is stored under, to the sets its copy is held on now; with several, the first
// takes a retarget (settleOnePendingImageUpdate). Every live notification is
// read before any is written, so one moved onto a key another origin leaves is
// never settled twice. A failure doesn't stop the rest.
func SettlePendingImageUpdates(ctx context.Context, userID string, locations map[PendingUpdateKey][]bson.ObjectID) error {

	live, err := findLivePendingImageUpdates(ctx, userID, slices.Collect(maps.Keys(locations)))
	if err != nil {
		return err
	}

	now := time.Now()
	var errs []error
	for _, n := range live {
		if err := settleOnePendingImageUpdate(ctx, n, locations[n.Payload.PendingUpdateKey], now); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
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
