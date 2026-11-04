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

func TestProcessBookmarkItemsHoldsAndStamps(t *testing.T) {

	db := testdb.CreateDatabase(t)
	oldCollection, oldTagger := imageset.Collection, imageset.Tagger
	imageset.Collection, imageset.Tagger = db.Collection("imagesets"), fakeTagger{}
	t.Cleanup(func() { imageset.Collection, imageset.Tagger = oldCollection, oldTagger })

	const user = "pixiv-hold-user"
	created := time.Date(2026, 8, 27, 9, 12, 12, 0, time.UTC)
	lastChecked := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	withCopy := func(setID bson.ObjectID, illustID string, state imageset.ImageChangeState, checked time.Time) imageset.ImageSetMongo {
		src := imageset.SourceInfo{Name: pixivServiceName, SourceID: illustID, Title: "Art", Date: created.Add(-time.Hour), LastChecked: checked, ImageChangeState: state}
		return imageset.ImageSetMongo{ID: setID, KscopeUserId: user, Title: "Art", Sources: []imageset.SourceInfo{src}}
	}

	notStale := func(set imageset.ImageSetMongo) imageset.ImageSetMongo {
		set.Sources[0].Date = created
		return set
	}

	free, held, hiddenOld, hiddenZero := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	heldPending, heldDeclined, removed := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	for _, set := range []imageset.ImageSetMongo{
		withCopy(free, "1", imageset.ImageChangeNone, lastChecked),
		withCopy(held, "1", imageset.ImageChangePending, lastChecked),
		withCopy(hiddenOld, "2", imageset.ImageChangeNone, lastChecked),
		withCopy(hiddenZero, "2", imageset.ImageChangeNone, time.Time{}),
		notStale(withCopy(heldPending, "3", imageset.ImageChangePending, lastChecked)),
		notStale(withCopy(heldDeclined, "3", imageset.ImageChangeDeclined, lastChecked)),
		withCopy(removed, "4", imageset.ImageChangeDeclined, lastChecked),
	} {
		if _, err := imageset.Collection.InsertOne(context.Background(), set); err != nil {
			t.Fatal(err)
		}
	}

	storedCopy := func(setID bson.ObjectID) imageset.SourceInfo {
		var set imageset.ImageSetMongo
		if err := imageset.Collection.FindOne(context.Background(), bson.M{"_id": setID}).Decode(&set); err != nil {
			t.Fatal(err)
		}
		return set.Sources[0]
	}

	//newest bookmark first: work 4, removed at the source; work 3, held in every set and not stale;
	//work 2, hidden by the account's filter; work 1, stale and held in one of its sets
	page := []pixivmodel.Illust{
		{ID: 4, ImageURLs: &pixivmodel.Images{SquareMedium: "https://s.pximg.net/common/images/limit_unknown_360.png"}},
		{ID: 3, Visible: true, Title: "Art", CreateDate: created},
		{ID: 2},
		{ID: 1, Visible: true, Title: "New", CreateDate: created},
	}
	before := time.Now().Truncate(time.Millisecond)
	items := processBookmarkItems(context.Background(), user, page, false)
	after := time.Now()

	want := []notification.ItemResult{
		{Kind: notification.ItemUpdated, Ref: "1", SetID: free},
		{Kind: notification.ItemSkipped, Ref: "1", SetID: held, Reason: "an image update is waiting for review"},
		{Kind: notification.ItemSkipped, Ref: "2", Reason: "hidden by the account's filter"},
		{Kind: notification.ItemSkipped, Ref: "3", SetID: heldPending, Reason: "an image update is waiting for review"},
		{Kind: notification.ItemSkipped, Ref: "3", SetID: heldDeclined, Reason: "image updates were declined"},
		{Kind: notification.ItemMissing, Ref: "4", SetID: removed},
	}
	if len(items) != len(want) {
		t.Fatalf("items %+v, want %+v", items, want)
	}

	for i := range want {
		if items[i] != want[i] {
			t.Errorf("item %d is %+v, want %+v", i, items[i], want[i])
		}
	}

	stamped := func(checked time.Time) bool { return !checked.Before(before) && !checked.After(after) }
	if src := storedCopy(held); src.Title != "Art" || !stamped(src.LastChecked) {
		t.Errorf("held copy %+v: want its title kept and LastChecked stamped", src)
	}

	if src := storedCopy(hiddenOld); !stamped(src.LastChecked) {
		t.Errorf("hidden work's copy: LastChecked %v, want it stamped", src.LastChecked)
	}

	if src := storedCopy(hiddenZero); !src.LastChecked.IsZero() {
		t.Errorf("hidden work's unchecked copy: LastChecked %v, want it left zero", src.LastChecked)
	}

	for _, setID := range []bson.ObjectID{heldPending, heldDeclined} {
		if src := storedCopy(setID); !stamped(src.LastChecked) {
			t.Errorf("work held in every set, set %s: LastChecked %v, want it stamped", setID.Hex(), src.LastChecked)
		}
	}

	if src := storedCopy(removed); !src.SourceMissing || src.ImageChangeState != imageset.ImageChangeNone {
		t.Errorf("removed work's declined copy %+v: want it marked missing and its state reset", src)
	}
}
