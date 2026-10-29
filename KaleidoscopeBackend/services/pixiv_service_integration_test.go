//go:build integration

package services

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/testdb"
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	pixivmodel "github.com/ryohidaka/go-pixiv/models/appmodel"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// fakeTagger changes no tags; ProcessSourceTags fails for failSetID's set.
type fakeTagger struct {
	failSetID bson.ObjectID
}

func (f fakeTagger) ProcessSourceTags(_ string, set *imageset.ImageSetMongo, _ int, _ []imageset.SourceTag) (imageset.TagCountChange, error) {
	if !f.failSetID.IsZero() && set.ID == f.failSetID {
		return imageset.TagCountChange{}, errors.New("tags failed")
	}
	return imageset.TagCountChange{}, nil
}
func (fakeTagger) RecordDeletion(string, []imageset.SourceInfo, []string) error { return nil }
func (fakeTagger) RecomputeSystemTags(string, *imageset.ImageSetMongo) (imageset.TagCountChange, error) {
	return imageset.TagCountChange{}, nil
}
func (fakeTagger) UndoTagCounts(string, imageset.TagCountChange) error { return nil }
func (fakeTagger) CombineTagOverrides(string, *imageset.ImageSetMongo, []string) (imageset.TagCountChange, error) {
	return imageset.TagCountChange{}, nil
}
func (fakeTagger) RefreshUntaggedCount(string) error { return nil }
func (fakeTagger) ResolveTagTerm(string, string) ([]string, bool, error) {
	return nil, false, nil
}

func TestProcessBookmarkItemsAcrossSets(t *testing.T) {

	db := testdb.CreateDatabase(t)
	oldCollection, oldTagger := imageset.Collection, imageset.Tagger
	imageset.Collection = db.Collection("imagesets")
	t.Cleanup(func() { imageset.Collection, imageset.Tagger = oldCollection, oldTagger })

	const user = "pixiv-page-user"
	created := time.Date(2026, 8, 27, 9, 12, 12, 0, time.UTC)
	lastChecked := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	withSource := func(setID bson.ObjectID, illustID uint64, date time.Time) imageset.ImageSetMongo {
		src := imageset.SourceInfo{Name: pixivServiceName, SourceID: strconv.FormatUint(illustID, 10), Title: "Art", Date: date, LastChecked: lastChecked}
		return imageset.ImageSetMongo{ID: setID, KscopeUserId: user, Title: "Art", Sources: []imageset.SourceInfo{src}}
	}

	aCurrent, aStale, b, c, d := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	for _, set := range []imageset.ImageSetMongo{
		withSource(aCurrent, 1, created),
		withSource(aStale, 1, created.Add(-time.Hour)),
		withSource(b, 2, created),
		withSource(c, 3, created),
		withSource(d, 4, created.Add(-time.Hour)),
	} {
		if _, err := imageset.Collection.InsertOne(context.Background(), set); err != nil {
			t.Fatal(err)
		}
	}

	storedLastChecked := func(setID bson.ObjectID) time.Time {
		var set imageset.ImageSetMongo
		if err := imageset.Collection.FindOne(context.Background(), bson.M{"_id": setID}).Decode(&set); err != nil {
			t.Fatal(err)
		}
		return set.Sources[0].LastChecked
	}

	type item struct {
		kind  notification.ItemKind
		ref   string
		setID bson.ObjectID
	}
	expectItems := func(pass string, got []notification.ItemResult, want []item) {

		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s: %+v, want %d items", pass, got, len(want))
		}

		for i, w := range want {
			if got[i].Kind != w.kind || got[i].Ref != w.ref || got[i].SetID != w.setID {
				t.Errorf("%s: item %d is %+v, want %s %s in set %s", pass, i, got[i], w.kind, w.ref, w.setID.Hex())
			}
		}
	}

	//newest bookmark first, as Pixiv lists them: D, C, B, A
	illust := func(id uint64) pixivmodel.Illust {
		return pixivmodel.Illust{ID: id, Visible: true, Title: "Art", CreateDate: created}
	}
	page := []pixivmodel.Illust{illust(4), illust(3), illust(2), illust(1)}

	imageset.Tagger = fakeTagger{failSetID: d}
	before := time.Now().Truncate(time.Millisecond)
	items := processBookmarkItems(context.Background(), user, page, false)
	after := time.Now()

	expectItems("first pass", items, []item{
		{notification.ItemUpdated, "1", aStale},
		{notification.ItemUnchanged, "2", bson.NilObjectID},
		{notification.ItemUnchanged, "3", bson.NilObjectID},
		{notification.ItemFailed, "4", d},
	})

	for _, setID := range []bson.ObjectID{b, c} {
		if checked := storedLastChecked(setID); checked.Before(before) || checked.After(after) {
			t.Errorf("set %s: LastChecked %v, want the page's time", setID.Hex(), checked)
		}
	}

	if checked := storedLastChecked(d); !checked.Equal(lastChecked) {
		t.Errorf("failed set: LastChecked %v, want it left at %v", checked, lastChecked)
	}

	//the next sync, once D's failure is gone
	imageset.Tagger = fakeTagger{}
	items = processBookmarkItems(context.Background(), user, page, false)
	expectItems("second pass", items, []item{
		{notification.ItemUnchanged, "1", bson.NilObjectID},
		{notification.ItemUnchanged, "2", bson.NilObjectID},
		{notification.ItemUnchanged, "3", bson.NilObjectID},
		{notification.ItemUpdated, "4", d},
	})
}
