package imageset

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func TestMayWriteHaveLanded(t *testing.T) {

	useUnreachableCollection(t)
	_, databaseDown := Collection.InsertOne(context.Background(), bson.M{"page": 1})
	if databaseDown == nil {
		t.Fatal("an insert into an unreachable database succeeded")
	}

	lostReply := mongo.CommandError{Message: "connection closed unexpectedly by the other side", Labels: []string{"NetworkError"}}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"network error", lostReply, true},
		{"network error wrapped by saveMergedSet", fmt.Errorf("saving merged image set: %w", lostReply), true},
		{"write concern error", mongo.WriteException{WriteConcernError: &mongo.WriteConcernError{Name: "WriteConcernTimeout", Code: 64, Message: "waiting for replication timed out"}}, true},
		{"write error only", mongo.WriteException{WriteErrors: mongo.WriteErrors{{Code: 11000, Message: "duplicate key"}}}, false},
		{"client disconnected", mongo.ErrClientDisconnected, false},
		{"guard miss", errors.New("image set changed or was deleted before the merge was saved"), false},
		{"database down", databaseDown, false},
	}

	for _, tc := range tests {
		if got := isWritePossibleFromError(tc.err); got != tc.want {
			t.Errorf("%s (%v): %t, want %t", tc.name, tc.err, got, tc.want)
		}
	}
}

func TestGroupSetsBySourceID(t *testing.T) {

	pixiv := func(id string) SourceInfo { return SourceInfo{Name: "pixiv", SourceID: id} }
	docs := []ImageSetMongo{
		{ID: bson.NewObjectID(), Sources: []SourceInfo{pixiv("1")}},
		{ID: bson.NewObjectID(), Sources: []SourceInfo{{Name: "zipsrc", SourceID: "1"}, pixiv("1"), pixiv("2")}},
		{ID: bson.NewObjectID(), Sources: []SourceInfo{pixiv("1"), pixiv("1")}},
		{ID: bson.NewObjectID(), Sources: []SourceInfo{{Name: "zipsrc", SourceID: "1"}}},
	}

	grouped := groupSetsBySourceID(docs, "pixiv", []string{"1"})

	var listed []bson.ObjectID
	for _, set := range grouped["1"] {
		listed = append(listed, set.ID)
	}
	if want := []bson.ObjectID{docs[0].ID, docs[1].ID, docs[2].ID}; !slices.Equal(listed, want) {
		t.Errorf("under 1: %v, want %v (in order, the set holding it twice once, not the other service's)", listed, want)
	}

	if len(grouped) != 1 {
		t.Errorf("keys: %d, want only the requested ID", len(grouped))
	}
}
