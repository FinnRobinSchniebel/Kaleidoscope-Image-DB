package imageset

import (
	"slices"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

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
