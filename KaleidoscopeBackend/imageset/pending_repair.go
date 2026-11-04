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
// pending source that has none still pending (matched by
// notification.PublishUnmatchedPendingImageUpdates), e.g. after the pending
// writer's publish failed. It takes no lock, so it must run at startup before
// anything else can write sets or notifications. A failed publish is logged and
// left for the next startup.
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

	var wanted []notification.UserPendingImageUpdate
	for s := range sets {
		for i := range sets[s].Sources {
			if sets[s].Sources[i].ImageChangeState == ImageChangePending {
				wanted = append(wanted, notification.UserPendingImageUpdate{UserID: sets[s].KscopeUserId, Update: newPendingImageUpdate(&sets[s], i)})
			}
		}
	}

	published, err := notification.PublishUnmatchedPendingImageUpdates(ctx, wanted)
	if err != nil {
		return err
	}

	log.Printf("pending updates: published %d missing notifications", published)
	return nil
}
