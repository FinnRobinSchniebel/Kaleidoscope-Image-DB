//go:build integration

package notification

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/testdb"
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestSettlePendingImageUpdates(t *testing.T) {

	db := testdb.CreateDatabase(t)
	old := Collection
	Collection = db.Collection("notifications")
	t.Cleanup(func() { Collection = old })

	ctx := context.Background()
	published, read := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC), time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	key := func(setID bson.ObjectID) PendingUpdateKey {
		return PendingUpdateKey{SetID: setID, Service: "pixiv", SourceID: "7"}
	}

	//a notification as published at a fixed past time, read at readAt unless it's zero
	store := func(user string, setID bson.ObjectID, status Status, readAt time.Time) bson.ObjectID {

		t.Helper()
		n := newNotification(user, PendingImageUpdate{PendingUpdateKey: key(setID)}, published)
		n.Status, n.ReadAt = status, readAt
		result, err := Collection.InsertOne(ctx, n)
		if err != nil {
			t.Fatal(err)
		}
		return result.InsertedID.(bson.ObjectID)
	}

	load := func(id bson.ObjectID) storedPendingImageUpdate {

		t.Helper()
		var n storedPendingImageUpdate
		if err := Collection.FindOne(ctx, bson.M{"_id": id}).Decode(&n); err != nil {
			t.Fatalf("loading notification %s: %v", id.Hex(), err)
		}
		return n
	}

	copiesOn := func(setID bson.ObjectID) []storedPendingImageUpdate {

		t.Helper()
		cursor, err := Collection.Find(ctx, bson.M{"payload.set_id": setID})
		if err != nil {
			t.Fatal(err)
		}

		var found []storedPendingImageUpdate
		if err := cursor.All(ctx, &found); err != nil {
			t.Fatal(err)
		}
		return found
	}

	//each origin's other sets are made before its own, so the sorted first one is never its own
	s2, s3a, s3b, s4b, s5a, s5b, s6 := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	o1, o2, o3, o4, o5, o6, o7 := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()

	n1 := store("user1", o1, StatusPending, time.Time{})
	n1Accepted := store("user1", o1, StatusAccepted, time.Time{})
	n1OtherUser := store("user2", o1, StatusPending, time.Time{})
	n2 := store("user1", o2, StatusPending, read)
	n3 := store("user1", o3, StatusPending, time.Time{})
	n4 := store("user1", o4, StatusPending, time.Time{})
	n4Accepted := store("user1", o4, StatusAccepted, time.Time{})
	n5 := store("user1", o5, StatusDeclined, time.Time{})
	n7 := store("user1", o7, StatusPending, time.Time{})

	locations := map[PendingUpdateKey][]bson.ObjectID{
		key(o1): nil,
		key(o2): {s2},
		key(o3): {s3a, s3b},
		key(o4): {s4b, o4},
		key(o5): {s5a, s5b},
		key(o6): {s6},
	}

	//each pair moves one origin onto the set the other leaves; a settle that doesn't read first fails some
	type pair struct{ moved, left, movedOnto bson.ObjectID }
	pairs := make([]pair, 10)
	for i := range pairs {

		from, onto := bson.NewObjectID(), bson.NewObjectID()
		pairs[i] = pair{moved: store("user1", from, StatusPending, time.Time{}), left: store("user1", onto, StatusPending, time.Time{}), movedOnto: onto}
		locations[key(from)], locations[key(onto)] = []bson.ObjectID{onto}, nil
	}

	if err := SettlePendingImageUpdates(ctx, "user1", locations); err != nil {
		t.Fatal(err)
	}

	if got := load(n1); got.Status != StatusInvalid || got.Payload.SetID != o1 {
		t.Errorf("O1 with no location: %+v, want it invalid", got)
	}

	if got := load(n1Accepted); got.Status != StatusAccepted || got.Payload.SetID != o1 {
		t.Errorf("O1's accepted one: %+v, want it untouched", got)
	}

	if got := load(n1OtherUser); got.Status != StatusPending || got.Payload.SetID != o1 {
		t.Errorf("user2's under O1's key: %+v, want it untouched", got)
	}

	if got := load(n7); got.Status != StatusPending || got.Payload.SetID != o7 {
		t.Errorf("O7, not being settled: %+v, want it untouched", got)
	}

	if got := load(n2); got.Status != StatusPending || got.Payload.SetID != s2 || !got.CreatedAt.Equal(published) || !got.ReadAt.Equal(read) {
		t.Errorf("O2 with one location: %+v, want it pending on S2 with its CreatedAt and ReadAt", got)
	}

	if got := load(n3); got.Status != StatusPending || got.Payload.SetID != s3a {
		t.Errorf("O3: %+v, want it retargeted to S3a", got)
	}

	if got := copiesOn(s3b); len(got) != 1 || got[0].ID == n3 || got[0].Status != StatusPending || !got[0].CreatedAt.Equal(published) || got[0].Payload.SourceID != "7" {
		t.Errorf("O3's copy on S3b: %+v, want one pending copy with O3's CreatedAt", got)
	}

	if got := load(n4); got.Status != StatusPending || got.Payload.SetID != o4 {
		t.Errorf("O4 among its own locations: %+v, want it left in place", got)
	}

	//a resolved one is never copied
	if got := copiesOn(s4b); len(got) != 1 || got[0].Status != StatusPending {
		t.Errorf("O4's copy on S4b: %+v, want one pending copy", got)
	}

	if got := load(n4Accepted); got.Status != StatusAccepted || got.Payload.SetID != o4 {
		t.Errorf("O4's accepted one: %+v, want it untouched", got)
	}

	if got := load(n5); got.Status != StatusDeclined || got.Payload.SetID != s5a {
		t.Errorf("O5: %+v, want it retargeted to S5a, still declined", got)
	}

	if got := copiesOn(s5b); len(got) != 1 || got[0].Status != StatusDeclined {
		t.Errorf("O5's copy on S5b: %+v, want one declined copy", got)
	}

	if got := copiesOn(s6); len(got) != 0 {
		t.Errorf("O6 with no notification: %+v on S6, want nothing written", got)
	}

	for i, p := range pairs {

		if got := load(p.left); got.Status != StatusInvalid || got.Payload.SetID != p.movedOnto {
			t.Errorf("pair %d: the origin left with no location %+v, want it invalid in place", i, got)
		}

		if got := load(p.moved); got.Status != StatusPending || got.Payload.SetID != p.movedOnto {
			t.Errorf("pair %d: the moved origin %+v, want it pending on %s", i, got, p.movedOnto.Hex())
		}
	}
}
