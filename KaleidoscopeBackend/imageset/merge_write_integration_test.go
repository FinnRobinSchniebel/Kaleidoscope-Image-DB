//go:build integration

package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestSuggestMergeReportsFailedPublish(t *testing.T) {

	useTestDatabase(t, fakeTagger{})
	ctx := context.Background()
	setID := bson.NewObjectID()
	pair := notification.ImagePair{ImageID: bson.NewObjectID(), CandidateImageID: bson.NewObjectID(), Match: notification.MatchExact}
	candidates := []mergeCandidate{{set: &ImageSetMongo{ID: bson.NewObjectID()}, pairs: []notification.ImagePair{pair}}}

	db := Collection.Database()
	validator := options.CreateCollection().SetValidator(bson.M{"never": bson.M{"$exists": true}})
	if err := db.CreateCollection(ctx, "rejecting", validator); err != nil {
		t.Fatal(err)
	}

	notifications := notification.Collection
	notification.Collection = db.Collection("rejecting")
	items := suggestMerge("user1", "art/g", setID, candidates, nil, nil)
	notification.Collection = notifications

	if len(items) != 1 || items[0].Kind != notification.ItemFailed || items[0].Ref != "art/g" || items[0].SetID != setID || items[0].Reason != notification.ErrSuggestionSaveFailed.Error() {
		t.Errorf("failed publish: %+v, want one failed item for the set with the suggestion reason", items)
	}

	items = suggestMerge("user1", "art/g", setID, candidates, nil, nil)
	if len(items) != 1 || items[0].Kind != notification.ItemPending || items[0].SetID != setID || items[0].Reason != "suggested a merge with 1 set" {
		t.Errorf("publish: %+v, want one pending item for the set", items)
	}

	stored, err := notification.Collection.CountDocuments(ctx, bson.M{"type": notification.TypeSuggestedMerge, "payload.set_id": setID})
	if err != nil || stored != 1 {
		t.Errorf("stored suggestions: %d (%v), want 1", stored, err)
	}
}
