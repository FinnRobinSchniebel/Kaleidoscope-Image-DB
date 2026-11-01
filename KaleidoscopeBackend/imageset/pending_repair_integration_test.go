//go:build integration

package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestPublishMissingPendingUpdates(t *testing.T) {

	useTestDatabase(t, fakeTagger{})
	ctx := context.Background()
	a, b, c, d, e := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	source := func(name, id string, state ImageChangeState) SourceInfo {
		return SourceInfo{Name: name, SourceID: id, ImageChangeState: state}
	}
	insertSets(t,
		ImageSetMongo{ID: a, KscopeUserId: "user1", Sources: []SourceInfo{source("pixiv", "1", ImageChangePending), source("zipsrc", "9", ImageChangePending)}},
		ImageSetMongo{ID: b, KscopeUserId: "user1", Sources: []SourceInfo{source("pixiv", "2", ImageChangePending), source("zipsrc", "8", ImageChangeDeclined)}},
		ImageSetMongo{ID: c, KscopeUserId: "user1", Sources: []SourceInfo{source("pixiv", "3", ImageChangeDeclined)}},
		ImageSetMongo{ID: d, KscopeUserId: "user1", Sources: []SourceInfo{source("pixiv", "4", ImageChangeNone)}},
		ImageSetMongo{ID: e, KscopeUserId: "user2", Sources: []SourceInfo{source("pixiv", "1", ImageChangePending)}},
	)

	//A's pixiv copy already has one; B's pixiv copy only has one that turned invalid.
	//The declined copies get none: C's isn't even found, B's is found beside a pending one.
	for _, p := range []notification.PendingImageUpdate{{SetID: a, Service: "pixiv", SourceID: "1"}, {SetID: b, Service: "pixiv", SourceID: "2"}} {
		if err := notification.Publish("user1", p); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := notification.Collection.UpdateOne(ctx, bson.M{"payload.set_id": b}, bson.M{"$set": bson.M{"status": notification.StatusInvalid}}); err != nil {
		t.Fatal(err)
	}

	if err := PublishMissingPendingUpdates(ctx); err != nil {
		t.Fatal(err)
	}

	got := loadPendingUpdates(t)
	if len(got) != 5 {
		t.Fatalf("%+v, want the two existing notifications and three new ones", got)
	}

	pending := func(user string, setID bson.ObjectID, service, sourceID string) storedPendingUpdate {
		return storedPendingUpdate{UserID: user, Status: notification.StatusPending, Payload: notification.PendingImageUpdate{SetID: setID, Service: service, SourceID: sourceID}}
	}
	want := map[storedPendingUpdate]bool{
		pending("user1", a, "zipsrc", "9"): true,
		pending("user1", b, "pixiv", "2"):  true,
		pending("user2", e, "pixiv", "1"):  true,
	}

	for _, published := range got[2:] {

		if !want[published] {
			t.Errorf("published %+v, which wasn't missing", published)
		}
		delete(want, published)
	}

	for missed := range want {
		t.Errorf("never published %+v", missed)
	}

	if err := PublishMissingPendingUpdates(ctx); err != nil {
		t.Fatal(err)
	}

	if again := loadPendingUpdates(t); len(again) != 5 {
		t.Errorf("second run: %d notifications, want still 5", len(again))
	}
}
