package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"context"
	"fmt"
	"log"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// PublishMissingPendingUpdates publishes a pending-update notification for every
// pending source that has none still pending, e.g. after the pending writer's
// publish failed. It takes no lock, so it must run at startup before anything
// else can write sets or notifications. A failed publish is logged and left for
// the next startup.
func PublishMissingPendingUpdates(ctx context.Context) error {

	filter := bson.M{"sources.image_change_state": ImageChangePending}
	projection := bson.M{"kscope_userid": 1, "sources.name": 1, "sources.source_id": 1, "sources.image_change_state": 1}
	cursor, err := Collection.Find(ctx, filter, options.Find().SetProjection(projection))
	if err != nil {
		return fmt.Errorf("finding sets with a pending source: %w", err)
	}

	var sets []ImageSetMongo
	if err := cursor.All(ctx, &sets); err != nil {
		return fmt.Errorf("reading sets with a pending source: %w", err)
	}

	setIDs := make([]bson.ObjectID, len(sets))
	for i, set := range sets {
		setIDs[i] = set.ID
	}

	published, err := notification.FindPendingImageUpdates(ctx, setIDs)
	if err != nil {
		return err
	}

	notified := make(map[notification.PendingImageUpdate]bool, len(published))
	for _, update := range published {
		notified[update] = true
	}

	count := 0
	for _, set := range sets {
		for _, src := range set.Sources {

			update := notification.PendingImageUpdate{SetID: set.ID, Service: src.Name, SourceID: src.SourceID}
			if src.ImageChangeState != ImageChangePending || notified[update] {
				continue
			}

			if err := notification.Publish(set.KscopeUserId, update); err != nil {
				log.Printf("------ Warning: publishing the missing pending update for %s/%s in set %s: %s ------", src.Name, src.SourceID, set.ID.Hex(), err)
				continue
			}
			count++
		}
	}

	log.Printf("pending updates: published %d missing notifications", count)
	return nil
}
