//go:build integration

package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/testdb"
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// useTestDatabase points Collection at a throwaway database and Tagger at
// tagger until the test ends.
func useTestDatabase(t *testing.T, tagger AutoTagger) {

	t.Helper()
	db := testdb.CreateDatabase(t)
	oldCollection, oldTagger := Collection, Tagger
	Collection, Tagger = db.Collection("imagesets"), tagger
	t.Cleanup(func() { Collection, Tagger = oldCollection, oldTagger })
}

func insertSets(t *testing.T, sets ...ImageSetMongo) {

	t.Helper()
	for _, set := range sets {
		if _, err := Collection.InsertOne(context.Background(), set); err != nil {
			t.Fatalf("inserting set %s: %v", set.ID.Hex(), err)
		}
	}
}

func loadSet(t *testing.T, id bson.ObjectID) ImageSetMongo {

	t.Helper()
	var set ImageSetMongo
	if err := Collection.FindOne(context.Background(), bson.M{"_id": id}).Decode(&set); err != nil {
		t.Fatalf("loading set %s: %v", id.Hex(), err)
	}
	return set
}

func setIDs(sets []*ImageSetMongo) []bson.ObjectID {
	ids := make([]bson.ObjectID, len(sets))
	for i, set := range sets {
		ids[i] = set.ID
	}
	return ids
}

func TestGetImageSetsBySourceIDsListsEverySet(t *testing.T) {

	useTestDatabase(t, fakeTagger{})
	older, newer := bson.NewObjectID(), bson.NewObjectID()

	//the newer set goes in first, so only the sort puts the older one first
	insertSets(t,
		ImageSetMongo{ID: newer, KscopeUserId: "user1", Sources: []SourceInfo{{Name: "pixiv", SourceID: "1"}}},
		ImageSetMongo{ID: older, KscopeUserId: "user1", Sources: []SourceInfo{{Name: "pixiv", SourceID: "2"}, {Name: "pixiv", SourceID: "1"}}},
		ImageSetMongo{ID: bson.NewObjectID(), KscopeUserId: "user2", Sources: []SourceInfo{{Name: "pixiv", SourceID: "1"}}},
	)

	found, err := GetImageSetsBySourceIDs("user1", "pixiv", []string{"1", "2"})
	if err != nil {
		t.Fatal(err)
	}

	if got := setIDs(found["1"]); !slices.Equal(got, []bson.ObjectID{older, newer}) {
		t.Errorf("under 1: %v, want the user's two sets, oldest first", got)
	}

	if got := setIDs(found["2"]); !slices.Equal(got, []bson.ObjectID{older}) {
		t.Errorf("under 2: %v, want the older set", got)
	}
}

func TestSourceWritersUpdateEverySet(t *testing.T) {

	useTestDatabase(t, fakeTagger{})
	first, second := bson.NewObjectID(), bson.NewObjectID()
	insertSets(t,
		ImageSetMongo{ID: first, KscopeUserId: "user1", Title: "Old", Sources: []SourceInfo{{Name: "pixiv", SourceID: "1", Title: "Old"}}},
		ImageSetMongo{ID: second, KscopeUserId: "user1", Title: "Old", Sources: []SourceInfo{{Name: "zipsrc", SourceID: "9"}, {Name: "pixiv", SourceID: "1", Title: "Old"}}},
	)
	ctx, now := context.Background(), time.Now()

	expectChanged := func(write string, results []SourceWriteResult, err error, want bool) {

		t.Helper()
		if err != nil || len(results) != 2 {
			t.Fatalf("%s: %+v, %v; want a result for each set", write, results, err)
		}

		for _, result := range results {
			if result.Err != nil || result.Changed != want {
				t.Errorf("%s: set %s: %+v, want Changed %t", write, result.SetID.Hex(), result, want)
			}
		}
	}

	results, err := ApplySourceMetadataUpdate(ctx, "user1", SourceInfo{Name: "pixiv", SourceID: "1", Title: "New"}, now)
	expectChanged("metadata update", results, err, true)
	if a, b := loadSet(t, first), loadSet(t, second); a.Sources[0].Title != "New" || b.Sources[1].Title != "New" {
		t.Errorf("stored titles %q and %q, want New in both sets", a.Sources[0].Title, b.Sources[1].Title)
	}

	results, err = MarkSourceMissing(ctx, "user1", "pixiv", "1", now)
	expectChanged("missing mark", results, err, true)
	results, err = MarkSourceMissing(ctx, "user1", "pixiv", "1", now)
	expectChanged("repeated missing mark", results, err, false)

	results, err = MarkSourceRecovered(ctx, "user1", "pixiv", "1", now)
	expectChanged("recovered mark", results, err, true)

	results, err = MarkSourcePendingImageChange(ctx, "user1", "pixiv", "1", now, now)
	expectChanged("pending mark", results, err, true)
	results, err = MarkSourcePendingImageChange(ctx, "user1", "pixiv", "1", now, now)
	expectChanged("repeated pending mark", results, err, false)
}

func TestSourceWriterFailureIsRetried(t *testing.T) {

	first, second := bson.NewObjectID(), bson.NewObjectID()
	useTestDatabase(t, fakeTagger{processErr: errors.New("tags failed"), failSetID: first})
	insertSets(t,
		ImageSetMongo{ID: first, KscopeUserId: "user1", Sources: []SourceInfo{{Name: "pixiv", SourceID: "1", Title: "Old"}}},
		ImageSetMongo{ID: second, KscopeUserId: "user1", Sources: []SourceInfo{{Name: "pixiv", SourceID: "1", Title: "Old"}}},
	)
	update := SourceInfo{Name: "pixiv", SourceID: "1", Title: "New"}

	results, err := ApplySourceMetadataUpdate(context.Background(), "user1", update, time.Now())
	if err != nil || len(results) != 2 {
		t.Fatalf("first call: %+v, %v; want a result for each set", results, err)
	}

	if results[0].SetID != first || results[0].Err == nil || results[1].Err != nil || !results[1].Changed {
		t.Errorf("first call: %+v, want the first set failed and the second changed", results)
	}

	if a, b := loadSet(t, first), loadSet(t, second); a.Sources[0].Title != "Old" || b.Sources[0].Title != "New" {
		t.Errorf("after the first call: titles %q and %q, want Old (failed) and New", a.Sources[0].Title, b.Sources[0].Title)
	}

	//the next sync, once the failure is gone
	Tagger = fakeTagger{}
	results, err = ApplySourceMetadataUpdate(context.Background(), "user1", update, time.Now())
	if err != nil || len(results) != 2 {
		t.Fatalf("retry: %+v, %v; want a result for each set", results, err)
	}

	if results[0].Err != nil || !results[0].Changed || results[1].Err != nil || results[1].Changed {
		t.Errorf("retry: %+v, want the first set changed and the second unchanged", results)
	}

	if a, b := loadSet(t, first), loadSet(t, second); a.Sources[0].Title != "New" || b.Sources[0].Title != "New" {
		t.Errorf("after the retry: titles %q and %q, want New in both", a.Sources[0].Title, b.Sources[0].Title)
	}
}

func TestSourceWriterUndoesCountsWhenItsWriteFails(t *testing.T) {

	first, second := bson.NewObjectID(), bson.NewObjectID()
	var undone []TagCountChange
	calls := 0
	tagger := fakeTagger{undone: &undone}
	tagger.onProcess = func() {

		calls++
		if calls != 2 {
			return
		}

		//the second set's source moves, so its guarded write matches nothing
		if _, err := Collection.UpdateOne(context.Background(), bson.M{"_id": second}, bson.M{"$set": bson.M{"sources.0.source_id": "moved"}}); err != nil {
			t.Errorf("moving the second set's source: %v", err)
		}
	}
	useTestDatabase(t, tagger)
	insertSets(t,
		ImageSetMongo{ID: first, KscopeUserId: "user1", Sources: []SourceInfo{{Name: "pixiv", SourceID: "1", Title: "Old"}}},
		ImageSetMongo{ID: second, KscopeUserId: "user1", Sources: []SourceInfo{{Name: "pixiv", SourceID: "1", Title: "Old"}}},
	)

	results, err := ApplySourceMetadataUpdate(context.Background(), "user1", SourceInfo{Name: "pixiv", SourceID: "1", Title: "New"}, time.Now())
	if err != nil || len(results) != 2 {
		t.Fatalf("%+v, %v; want a result for each set", results, err)
	}

	if results[0].Err != nil || results[1].SetID != second || results[1].Err == nil {
		t.Errorf("%+v, want the first set written and the second failed", results)
	}

	if len(undone) != 1 || undone[0].SourceName != second.Hex() {
		t.Errorf("undone %+v, want exactly the second set's counts", undone)
	}

	if a := loadSet(t, first); a.Sources[0].Title != "New" {
		t.Errorf("first set's title %q, want New", a.Sources[0].Title)
	}
}

func TestMarkSourcesChecked(t *testing.T) {

	useTestDatabase(t, fakeTagger{})
	old := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	both, mixed, zero, unlisted, otherUser := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	source := func(name, id string, checked time.Time) SourceInfo {
		return SourceInfo{Name: name, SourceID: id, LastChecked: checked}
	}
	insertSets(t,
		ImageSetMongo{ID: both, KscopeUserId: "user1", Sources: []SourceInfo{source("pixiv", "1", old), source("pixiv", "2", old)}},
		ImageSetMongo{ID: mixed, KscopeUserId: "user1", Sources: []SourceInfo{source("zipsrc", "1", old), source("pixiv", "3", old)}},
		ImageSetMongo{ID: zero, KscopeUserId: "user1", Sources: []SourceInfo{source("pixiv", "4", time.Time{}), source("pixiv", "6", old)}},
		ImageSetMongo{ID: unlisted, KscopeUserId: "user1", Sources: []SourceInfo{source("pixiv", "5", old)}},
		ImageSetMongo{ID: otherUser, KscopeUserId: "user2", Sources: []SourceInfo{source("pixiv", "1", old)}},
	)
	listed := []string{"1", "2", "3", "4", "6"}
	checkedAt := time.Now().UTC().Truncate(time.Millisecond)

	expectLastChecked := func(when string, want map[bson.ObjectID][]time.Time) {

		t.Helper()
		for id, times := range want {

			set := loadSet(t, id)
			for i, src := range set.Sources {
				if !src.LastChecked.Equal(times[i]) {
					t.Errorf("%s: set %s source %s/%s: LastChecked %v, want %v", when, id.Hex(), src.Name, src.SourceID, src.LastChecked, times[i])
				}
			}
		}
	}
	untouched := map[bson.ObjectID][]time.Time{
		both: {old, old}, mixed: {old, old}, zero: {{}, old}, unlisted: {old}, otherUser: {old},
	}

	if err := MarkSourcesChecked(context.Background(), "user1", "pixiv", nil, checkedAt); err != nil {
		t.Fatalf("empty list: %v", err)
	}
	expectLastChecked("empty list", untouched)

	unlock := LockUserSets("user1")
	busy := WithPause(context.Background(), func(context.Context) error { return errors.New("gave up") })
	if err := MarkSourcesChecked(busy, "user1", "pixiv", listed, checkedAt); !errors.Is(err, ErrLibraryBusy) {
		t.Errorf("library busy: %v, want ErrLibraryBusy", err)
	}
	unlock()
	expectLastChecked("library busy", untouched)

	if err := MarkSourcesChecked(context.Background(), "user1", "pixiv", listed, checkedAt); err != nil {
		t.Fatal(err)
	}
	expectLastChecked("bumped", map[bson.ObjectID][]time.Time{
		both:      {checkedAt, checkedAt},
		mixed:     {old, checkedAt},
		zero:      {{}, checkedAt},
		unlisted:  {old},
		otherUser: {old},
	})
}
