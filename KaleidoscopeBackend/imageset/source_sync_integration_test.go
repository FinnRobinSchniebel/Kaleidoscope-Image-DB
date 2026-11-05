//go:build integration

package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/testdb"
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// useTestDatabase points Collection and notification.Collection at a throwaway
// database and Tagger at tagger until the test ends.
func useTestDatabase(t *testing.T, tagger AutoTagger) {

	t.Helper()
	db := testdb.CreateDatabase(t)
	oldCollection, oldNotifications, oldTagger := Collection, notification.Collection, Tagger
	Collection, notification.Collection, Tagger = db.Collection("imagesets"), db.Collection("notifications"), tagger
	t.Cleanup(func() { Collection, notification.Collection, Tagger = oldCollection, oldNotifications, oldTagger })
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

// storedPendingUpdate is a stored pending-update notification with its payload
// decoded by type.
type storedPendingUpdate struct {
	UserID  string                          `bson:"user_id"`
	Status  notification.Status             `bson:"status"`
	Payload notification.PendingImageUpdate `bson:"payload"`
}

// loadPendingUpdates returns every stored pending-update notification, oldest
// first.
func loadPendingUpdates(t *testing.T) []storedPendingUpdate {

	t.Helper()
	filter := bson.M{"type": notification.TypePendingImageUpdate}
	cursor, err := notification.Collection.Find(context.Background(), filter, options.Find().SetSort(bson.M{"_id": 1}))
	if err != nil {
		t.Fatalf("finding pending update notifications: %v", err)
	}

	var updates []storedPendingUpdate
	if err := cursor.All(context.Background(), &updates); err != nil {
		t.Fatalf("reading pending update notifications: %v", err)
	}
	return updates
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
	image := ImageInfo{ID: bson.NewObjectID(), Name: "page.png", ImageHashes: ImageHashes{Pixel: pagePixels("p1"), PDQ: pagePDQ("p1")}, Width: 100, Height: 200}

	//the newer set goes in first, so only the sort puts the older one first
	insertSets(t,
		ImageSetMongo{ID: newer, KscopeUserId: "user1", Sources: []SourceInfo{{Name: "pixiv", SourceID: "1"}}},
		ImageSetMongo{ID: older, KscopeUserId: "user1", Title: "Art", Path: "user1/art", Image: []ImageInfo{image}, Sources: []SourceInfo{{Name: "pixiv", SourceID: "2"}, {Name: "pixiv", SourceID: "1"}}},
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

	//projected: sources and each image's ID, hashes and size only
	read := found["2"][0]
	if read.Title != "" || read.Path != "" || len(read.Sources) != 2 || len(read.Image) != 1 {
		t.Fatalf("older set read as %+v, want only its sources and its one image", read)
	}

	got := read.Image[0]
	if got.ID != image.ID || got.Name != "" || got.Width != 100 || got.Height != 200 || !slices.Equal(got.PDQ, image.PDQ) || !slices.Equal(got.Pixel, image.Pixel) {
		t.Errorf("older set's image read as %+v, want its ID, hashes and size without its file name", got)
	}
}

func TestSourceWritersUpdateEverySet(t *testing.T) {

	recomputed := 0
	tagger := fakeTagger{}
	tagger.onRecompute = func() { recomputed++ }
	useTestDatabase(t, tagger)
	first, second := bson.NewObjectID(), bson.NewObjectID()
	sourceDate := time.Date(2026, 8, 27, 9, 12, 12, 0, time.UTC)

	//dated like the update, so it stays a metadata update
	insertSets(t,
		ImageSetMongo{ID: first, KscopeUserId: "user1", Title: "Old", Sources: []SourceInfo{{Name: "pixiv", SourceID: "1", Title: "Old", Date: sourceDate}}},
		ImageSetMongo{ID: second, KscopeUserId: "user1", Title: "Old", Sources: []SourceInfo{{Name: "zipsrc", SourceID: "9"}, {Name: "pixiv", SourceID: "1", Title: "Old", Date: sourceDate}}},
	)
	ctx, now := context.Background(), time.Now()

	expectChanged := func(write string, results []SourceWriteResult, err error, want bool, kind SourceWriteKind) {

		t.Helper()
		if err != nil || len(results) != 2 {
			t.Fatalf("%s: %+v, %v; want a result for each set", write, results, err)
		}

		for _, result := range results {
			if result.Err != nil || result.Changed != want || result.Kind != kind {
				t.Errorf("%s: set %s: %+v, want Changed %t and Kind %q", write, result.SetID.Hex(), result, want, kind)
			}
		}
	}

	markPending := func() ([]SourceWriteResult, error) {
		return writeEachSetHoldingSource(ctx, "user1", "pixiv", "1", func(set *ImageSetMongo, index int) SourceWriteResult {
			return markSourcePendingImageChangeInSet(set, index, now)
		})
	}

	//each write recomputes the system tags only when it flips SourceMissing
	expectRecomputed := func(write string, want int) {

		t.Helper()
		if recomputed != want {
			t.Errorf("%s: system tags recomputed %d times, want %d", write, recomputed, want)
		}
		recomputed = 0
	}

	results, err := ApplySourceUpdate(ctx, "user1", SourceInfo{Name: "pixiv", SourceID: "1", Title: "New", Date: sourceDate}, nil, now)
	expectChanged("metadata update", results, err, true, SourceMetadataUpdate)
	expectRecomputed("metadata update", 0)
	if a, b := loadSet(t, first), loadSet(t, second); a.Sources[0].Title != "New" || b.Sources[1].Title != "New" {
		t.Errorf("stored titles %q and %q, want New in both sets", a.Sources[0].Title, b.Sources[1].Title)
	}

	results, err = MarkSourceMissing(ctx, "user1", "pixiv", "1", now)
	expectChanged("missing mark", results, err, true, SourceMissingMark)
	expectRecomputed("missing mark", 2)
	results, err = MarkSourceMissing(ctx, "user1", "pixiv", "1", now)
	expectChanged("repeated missing mark", results, err, false, SourceMissingMark)
	expectRecomputed("repeated missing mark", 0)

	expectNotified := func(when string) {

		t.Helper()
		got := loadPendingUpdates(t)
		if len(got) != 2 {
			t.Fatalf("%s: %+v, want one pending update notification per set", when, got)
		}

		for i, setID := range []bson.ObjectID{first, second} {

			key := notification.PendingUpdateKey{SetID: setID, Service: "pixiv", SourceID: "1"}
			want := storedPendingUpdate{UserID: "user1", Status: notification.StatusPending, Payload: notification.PendingImageUpdate{PendingUpdateKey: key}}
			if got[i] != want {
				t.Errorf("%s: notification %d is %+v, want %+v", when, i, got[i], want)
			}
		}
	}

	//the missing mark stored the state as "", which the pending mark's guard must still match
	results, err = markPending()
	expectChanged("pending mark", results, err, true, SourcePendingFlip)
	expectRecomputed("pending mark on missing copies", 2)
	expectNotified("pending mark")
	if a, b := loadSet(t, first), loadSet(t, second); !a.Sources[0].Date.Equal(sourceDate) || !b.Sources[1].Date.Equal(sourceDate) {
		t.Errorf("Dates %v and %v after the pending mark, want both left at %v", a.Sources[0].Date, b.Sources[1].Date, sourceDate)
	}

	results, err = markPending()
	expectChanged("repeated pending mark", results, err, false, SourceHeldStamp)
	expectRecomputed("repeated pending mark", 0)
	expectNotified("repeated pending mark")
	for _, result := range results {
		if result.Held != ImageChangePending {
			t.Errorf("repeated pending mark: set %s: Held %q, want pending", result.SetID.Hex(), result.Held)
		}
	}

	//a missing mark still applies to a held copy, through the guard on the state it read
	results, err = MarkSourceMissing(ctx, "user1", "pixiv", "1", now)
	expectChanged("missing mark on held copies", results, err, true, SourceMissingMark)
	expectRecomputed("missing mark on held copies", 2)
	expectNotified("missing mark on held copies")
	if a, b := loadSet(t, first), loadSet(t, second); a.Sources[0].ImageChangeState != ImageChangeNone || b.Sources[1].ImageChangeState != ImageChangeNone {
		t.Errorf("states %q and %q after the missing mark, want none in both", a.Sources[0].ImageChangeState, b.Sources[1].ImageChangeState)
	}
}

func TestSourceWriterFailureIsRetried(t *testing.T) {

	first, second := bson.NewObjectID(), bson.NewObjectID()
	useTestDatabase(t, fakeTagger{processErr: errors.New("tags failed"), failSetID: first})
	insertSets(t,
		ImageSetMongo{ID: first, KscopeUserId: "user1", Sources: []SourceInfo{{Name: "pixiv", SourceID: "1", Title: "Old"}}},
		ImageSetMongo{ID: second, KscopeUserId: "user1", Sources: []SourceInfo{{Name: "pixiv", SourceID: "1", Title: "Old"}}},
	)
	update := SourceInfo{Name: "pixiv", SourceID: "1", Title: "New"}

	results, err := ApplySourceUpdate(context.Background(), "user1", update, nil, time.Now())
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
	results, err = ApplySourceUpdate(context.Background(), "user1", update, nil, time.Now())
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

	results, err := ApplySourceUpdate(context.Background(), "user1", SourceInfo{Name: "pixiv", SourceID: "1", Title: "New"}, nil, time.Now())
	if err != nil || len(results) != 2 {
		t.Fatalf("%+v, %v; want a result for each set", results, err)
	}

	if results[0].Err != nil || results[1].SetID != second || results[1].Err == nil || results[1].Kind != SourceMetadataUpdate {
		t.Errorf("%+v, want the first set written and the second's metadata update failed", results)
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

func TestMetadataUpdateHoldsHeldCopies(t *testing.T) {

	calls := 0
	tagger := fakeTagger{}
	tagger.onProcess = func() { calls++ }
	useTestDatabase(t, tagger)

	free, pending, declined := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	old := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	oldDate := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	withCopy := func(id bson.ObjectID, state ImageChangeState, checked time.Time) ImageSetMongo {
		src := SourceInfo{Name: "pixiv", SourceID: "1", Title: "Old", Date: oldDate, Tags: []SourceTag{{Default: "cat"}}, LastChecked: checked, ImageChangeState: state}
		return ImageSetMongo{ID: id, KscopeUserId: "user1", Title: "Old", Sources: []SourceInfo{src}}
	}

	//the free copy is dated like the update, so it stays a metadata update; the held ones are behind
	update := SourceInfo{Name: "pixiv", SourceID: "1", Title: "New", Date: oldDate.Add(time.Hour), Tags: []SourceTag{{Default: "cat"}, {Default: "dog"}}}
	freeSet := withCopy(free, ImageChangeNone, old)
	freeSet.Sources[0].Date = update.Date
	insertSets(t, freeSet, withCopy(pending, ImageChangePending, old), withCopy(declined, ImageChangeDeclined, time.Time{}))

	checkedAt := time.Now().UTC().Truncate(time.Millisecond)
	results, err := ApplySourceUpdate(context.Background(), "user1", update, nil, checkedAt)
	if err != nil || len(results) != 3 {
		t.Fatalf("%+v, %v; want a result for each set", results, err)
	}

	if r := results[0]; r.SetID != free || r.Err != nil || !r.Changed || r.Held != ImageChangeNone || r.Kind != SourceMetadataUpdate {
		t.Errorf("free set: %+v, want a metadata update that changed it, not held", r)
	}

	if src := loadSet(t, free).Sources[0]; src.Title != "New" {
		t.Errorf("free set's title %q, want New", src.Title)
	}

	expectHeld := func(i int, setID bson.ObjectID, state ImageChangeState, lastChecked time.Time) {

		t.Helper()
		if r := results[i]; r.SetID != setID || r.Err != nil || r.Changed || r.Held != state || r.Kind != SourceHeldStamp {
			t.Errorf("%s set: %+v, want a held stamp by %s that changed nothing", state, r, state)
		}

		src := loadSet(t, setID).Sources[0]
		if src.Title != "Old" || !src.Date.Equal(oldDate) || len(src.Tags) != 1 || src.ImageChangeState != state || !src.LastChecked.Equal(lastChecked) {
			t.Errorf("%s copy %+v: want title, Date and tags as they were, the state kept and LastChecked %v", state, src, lastChecked)
		}
	}
	expectHeld(1, pending, ImageChangePending, checkedAt)
	expectHeld(2, declined, ImageChangeDeclined, time.Time{})

	if calls != 1 {
		t.Errorf("ProcessSourceTags ran %d times, want once (the free set only)", calls)
	}
}

func TestMetadataUpdateReportsReturnedCopies(t *testing.T) {

	useTestDatabase(t, fakeTagger{})
	returned, present := bson.NewObjectID(), bson.NewObjectID()
	applied := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	withCopy := func(id bson.ObjectID, missing bool) ImageSetMongo {
		src := SourceInfo{Name: "pixiv", SourceID: "1", Title: "Same", LastAppliedUpdate: applied, SourceMissing: missing}
		return ImageSetMongo{ID: id, KscopeUserId: "user1", Title: "Same", Sources: []SourceInfo{src}}
	}
	insertSets(t, withCopy(returned, true), withCopy(present, false))

	results, err := ApplySourceUpdate(context.Background(), "user1", SourceInfo{Name: "pixiv", SourceID: "1", Title: "Same"}, nil, time.Now())
	if err != nil || len(results) != 2 {
		t.Fatalf("%+v, %v; want a result for each set", results, err)
	}

	if r := results[0]; r.SetID != returned || r.Err != nil || !r.Changed {
		t.Errorf("returned copy: %+v, want changed", r)
	}

	if r := results[1]; r.SetID != present || r.Err != nil || r.Changed {
		t.Errorf("present copy: %+v, want unchanged", r)
	}

	if src := loadSet(t, returned).Sources[0]; src.SourceMissing || !src.LastAppliedUpdate.Equal(applied) {
		t.Errorf("returned copy %+v: want SourceMissing cleared and LastAppliedUpdate kept", src)
	}
}

func TestMarkSourcePendingImageChange(t *testing.T) {

	ctx := context.Background()
	flips, declined, failing, unpublished := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	var undone []TagCountChange
	var moveSourceOf bson.ObjectID
	tagger := fakeTagger{undone: &undone}
	tagger.onRecompute = func() {

		if moveSourceOf.IsZero() {
			return
		}

		//the source moves between the re-read and the write, so the guarded write matches nothing
		if _, err := Collection.UpdateOne(ctx, bson.M{"_id": moveSourceOf}, bson.M{"$set": bson.M{"sources.0.source_id": "moved"}}); err != nil {
			t.Errorf("moving the source: %v", err)
		}
	}
	useTestDatabase(t, tagger)

	sourceDate := time.Date(2026, 8, 27, 9, 12, 12, 0, time.UTC)
	withCopy := func(id bson.ObjectID, sourceID string, src SourceInfo) ImageSetMongo {
		src.Name, src.SourceID, src.Date = "pixiv", sourceID, sourceDate
		return ImageSetMongo{ID: id, KscopeUserId: "user1", Sources: []SourceInfo{src}}
	}
	insertSets(t,
		withCopy(flips, "1", SourceInfo{SourceMissing: true}),
		withCopy(declined, "2", SourceInfo{ImageChangeState: ImageChangeDeclined, LastChecked: sourceDate}),
		withCopy(failing, "3", SourceInfo{SourceMissing: true}),
		withCopy(unpublished, "4", SourceInfo{}),
	)
	checkedAt := time.Now().UTC().Truncate(time.Millisecond)

	markOne := func(sourceID string) SourceWriteResult {

		t.Helper()
		results, err := writeEachSetHoldingSource(ctx, "user1", "pixiv", sourceID, func(set *ImageSetMongo, index int) SourceWriteResult {
			return markSourcePendingImageChangeInSet(set, index, checkedAt)
		})
		if err != nil || len(results) != 1 {
			t.Fatalf("pixiv/%s: %+v, %v; want one result", sourceID, results, err)
		}
		return results[0]
	}

	if r := markOne("1"); r.Err != nil || !r.Changed {
		t.Errorf("missing copy: %+v, want it flipped", r)
	}

	if src := loadSet(t, flips).Sources[0]; src.ImageChangeState != ImageChangePending || src.SourceMissing || !src.LastChecked.Equal(checkedAt) || !src.Date.Equal(sourceDate) {
		t.Errorf("flipped copy %+v: want pending, not missing, LastChecked set over the zero and Date untouched", src)
	}

	if got := loadPendingUpdates(t); len(got) != 1 || got[0].Payload.SetID != flips {
		t.Errorf("after the flip: %+v, want one notification for the flipped set", got)
	}

	if r := markOne("2"); r.Err != nil || r.Changed || r.Held != ImageChangeDeclined {
		t.Errorf("declined copy: %+v, want held by declined", r)
	}

	if src := loadSet(t, declined).Sources[0]; src.ImageChangeState != ImageChangeDeclined {
		t.Errorf("declined copy's state %q, want it still declined", src.ImageChangeState)
	}

	moveSourceOf = failing
	if r := markOne("3"); r.SetID != failing || r.Err == nil || r.Changed {
		t.Errorf("failed write: %+v, want an error for its set", r)
	}
	moveSourceOf = bson.NilObjectID

	if got := loadPendingUpdates(t); len(got) != 1 {
		t.Errorf("after the failed write: %+v, want nothing more published", got)
	}

	if len(undone) != 1 || undone[0].SourceName != failing.Hex() {
		t.Errorf("undone %+v, want exactly the failed set's counts", undone)
	}

	if src := loadSet(t, failing).Sources[0]; !src.SourceMissing || src.ImageChangeState != ImageChangeNone {
		t.Errorf("failed set's copy %+v, want it still missing with state none", src)
	}

	db := Collection.Database()
	validator := options.CreateCollection().SetValidator(bson.M{"never": bson.M{"$exists": true}})
	if err := db.CreateCollection(ctx, "rejecting", validator); err != nil {
		t.Fatal(err)
	}

	notifications := notification.Collection
	notification.Collection = db.Collection("rejecting")
	r := markOne("4")
	notification.Collection = notifications
	if r.Err != nil || !r.Changed {
		t.Errorf("failed publish: %+v, want the flip written anyway", r)
	}

	if src := loadSet(t, unpublished).Sources[0]; src.ImageChangeState != ImageChangePending {
		t.Errorf("unpublished copy's state %q, want pending", src.ImageChangeState)
	}

	if err := PublishMissingPendingUpdates(ctx); err != nil {
		t.Fatal(err)
	}

	if got := loadPendingUpdates(t); len(got) != 2 || got[1].Payload.SetID != unpublished {
		t.Errorf("after the startup repair: %+v, want the unpublished set's notification added", got)
	}
}

func TestSourceWriteGuardedOnState(t *testing.T) {

	ctx := context.Background()
	guarded, blank := bson.NewObjectID(), bson.NewObjectID()
	var undone []TagCountChange
	tagger := fakeTagger{undone: &undone}
	tagger.onProcess = func() {

		//a decision lands between the re-read and the write; a no-op once it's pending
		if _, err := Collection.UpdateOne(ctx, bson.M{"_id": guarded}, bson.M{"$set": bson.M{"sources.1.image_change_state": ImageChangePending}}); err != nil {
			t.Errorf("pending the guarded copy: %v", err)
		}
	}
	useTestDatabase(t, tagger)

	//at index 1, where a null-matching guard would match any document
	withCopyAtOne := func(id bson.ObjectID, sourceID string) ImageSetMongo {
		return ImageSetMongo{ID: id, KscopeUserId: "user1", Sources: []SourceInfo{{Name: "zipsrc", SourceID: "9"}, {Name: "pixiv", SourceID: sourceID, Title: "Old"}}}
	}
	insertSets(t, withCopyAtOne(guarded, "1"), withCopyAtOne(blank, "2"))

	//none stored as "", which a struct insert leaves out
	if _, err := Collection.UpdateOne(ctx, bson.M{"_id": blank}, bson.M{"$set": bson.M{"sources.1.image_change_state": ""}}); err != nil {
		t.Fatal(err)
	}

	update := func(sourceID string) SourceWriteResult {

		t.Helper()
		results, err := ApplySourceUpdate(ctx, "user1", SourceInfo{Name: "pixiv", SourceID: sourceID, Title: "New"}, nil, time.Now())
		if err != nil || len(results) != 1 {
			t.Fatalf("pixiv/%s: %+v, %v; want one result", sourceID, results, err)
		}
		return results[0]
	}

	if r := update("1"); r.Err == nil {
		t.Errorf("copy pended mid-write: %+v, want the guarded write to fail", r)
	}

	if len(undone) != 1 || undone[0].SourceName != guarded.Hex() {
		t.Errorf("undone %+v, want exactly the guarded set's counts", undone)
	}

	if src := loadSet(t, guarded).Sources[1]; src.Title != "Old" {
		t.Errorf("guarded copy's title %q, want Old", src.Title)
	}

	if r := update("2"); r.Err != nil || !r.Changed {
		t.Errorf("copy with \"\" stored: %+v, want it updated", r)
	}

	if src := loadSet(t, blank).Sources[1]; src.Title != "New" {
		t.Errorf("copy with \"\" stored: title %q, want New", src.Title)
	}
}

func TestApplySourceUpdateDecidesEachSet(t *testing.T) {

	useTestDatabase(t, fakeTagger{})
	ctx := context.Background()

	reordered, reorderedIDs := newCheckSet(checkDate, []string{"p2", "p1", "p3"}, nil)
	reordered.Sources[0].SourceMissing = true
	removed, removedIDs := newCheckSet(checkDate, []string{"p1", "q", "p2", "p3"}, nil)
	attributed, attributedIDs := newCheckSet(checkDate, []string{"p1", "p3"}, []string{"p2"})
	pended, _ := newCheckSet(checkDate, []string{"p1", "p2", "p3"}, nil)
	held, _ := newCheckSet(checkDate, []string{"p1", "p2", "p3"}, nil)
	held.Sources[0].ImageChangeState = ImageChangePending
	current, _ := newCheckSet(serviceDate, []string{"p1", "p2", "p3"}, nil)
	unfetched, unfetchedIDs := newCheckSet(checkDate, []string{"p1", "p2"}, []string{"x3"})
	findImage(unfetched, unfetchedIDs["x3"]).PDQ = flipBits(pagePDQ("p3"), 0, 1, 2, 3, 4, 5, 6, 7, 8, 9)
	zipped, _ := newCheckSet(zipDate, []string{"p1", "p2", "p3"}, nil)

	sets := []*ImageSetMongo{reordered, removed, attributed, pended, held, current, unfetched, zipped}
	for _, set := range sets {
		insertSets(t, *set)
	}

	type outcome struct {
		kind    SourceWriteKind
		changed bool
		held    ImageChangeState
		err     error
	}
	expectResults := func(call string, results []SourceWriteResult, err error, want []outcome) {

		t.Helper()
		if err != nil || len(results) != len(want) {
			t.Fatalf("%s: %+v, %v; want a result for each set", call, results, err)
		}

		for i, w := range want {
			if r := results[i]; r.SetID != sets[i].ID || r.Kind != w.kind || r.Changed != w.changed || r.Held != w.held || !errors.Is(r.Err, w.err) {
				t.Errorf("%s: set %d is %+v, want %+v", call, i, r, w)
			}
		}
	}

	expectCopy := func(when string, set *ImageSetMongo, attribution []bson.ObjectID, date time.Time, title string, state ImageChangeState, lastChecked time.Time) {

		t.Helper()
		src := loadSet(t, set.ID).Sources[0]
		if !slices.Equal(src.AttributedTo, attribution) || !src.Date.Equal(date) || src.Title != title || src.ImageChangeState != state || !src.LastChecked.Equal(lastChecked) || src.SourceMissing {
			t.Errorf("%s: set %s's copy %+v, want attribution %v, Date %v, title %s, state %q, LastChecked %v, not missing", when, set.ID.Hex(), src, attribution, date, title, state, lastChecked)
		}
	}

	pages := &FetchedPages{Previews: previewsOf("p1", "p2", "p3"), Originals: map[int]PreparedImage{1: original("p2")}, FirstPageWidth: 100, FirstPageHeight: 200}
	update := SourceInfo{Name: "pixiv", SourceID: "1", Title: "New", Date: serviceDate}
	checkedAt := time.Now().UTC().Truncate(time.Millisecond)
	results, err := ApplySourceUpdate(ctx, "user1", update, pages, checkedAt)
	expectResults("with pages", results, err, []outcome{
		{kind: SourceReorder, changed: true},
		{kind: SourceRemoval, changed: true},
		{kind: SourceAttribution, changed: true},
		{kind: SourcePendingFlip, changed: true},
		{kind: SourceHeldStamp, held: ImageChangePending},
		{kind: SourceMetadataUpdate, changed: true},
		{kind: SourceImageCheck, err: ErrSetChangedDuringSync},
		{kind: SourceMetadataUpdate, changed: true},
	})

	expectCopy("reorder", reordered, idsOf(reorderedIDs, []string{"p1", "p2", "p3"}), serviceDate, "New", ImageChangeNone, checkedAt)
	expectCopy("removal", removed, idsOf(removedIDs, []string{"p1", "p2", "p3", "q"}), serviceDate, "New", ImageChangeNone, checkedAt)
	expectCopy("attribution", attributed, idsOf(attributedIDs, []string{"p1", "p2", "p3"}), serviceDate, "New", ImageChangeNone, checkedAt)
	expectCopy("pending flip", pended, pended.Sources[0].AttributedTo, checkDate, "Old", ImageChangePending, checkedAt)
	expectCopy("held", held, held.Sources[0].AttributedTo, checkDate, "Old", ImageChangePending, checkedAt)
	expectCopy("current", current, current.Sources[0].AttributedTo, serviceDate, "New", ImageChangeNone, checkedAt)
	expectCopy("unfetched original", unfetched, unfetched.Sources[0].AttributedTo, checkDate, "Old", ImageChangeNone, unfetched.Sources[0].LastChecked)
	expectCopy("zip-dated baseline", zipped, zipped.Sources[0].AttributedTo, serviceDate, "New", ImageChangeNone, checkedAt)

	for _, set := range []*ImageSetMongo{reordered, removed, attributed, current, zipped} {
		if src := loadSet(t, set.ID).Sources[0]; !src.LastAppliedUpdate.Equal(checkedAt) {
			t.Errorf("set %s: LastAppliedUpdate %v, want %v", set.ID.Hex(), src.LastAppliedUpdate, checkedAt)
		}
	}

	if got := loadPendingUpdates(t); len(got) != 1 || got[0].Payload.SetID != pended.ID {
		t.Errorf("notifications %+v, want one for the pending set", got)
	}

	//no pages: only the copy whose Date still moved needs them
	results, err = ApplySourceUpdate(ctx, "user1", update, nil, time.Now())
	expectResults("without pages", results, err, []outcome{
		{kind: SourceMetadataUpdate},
		{kind: SourceMetadataUpdate},
		{kind: SourceMetadataUpdate},
		{kind: SourceHeldStamp, held: ImageChangePending},
		{kind: SourceHeldStamp, held: ImageChangePending},
		{kind: SourceMetadataUpdate},
		{kind: SourceImageCheck, err: ErrSetChangedDuringSync},
		{kind: SourceMetadataUpdate},
	})
	expectCopy("unfetched original, no pages", unfetched, unfetched.Sources[0].AttributedTo, checkDate, "Old", ImageChangeNone, unfetched.Sources[0].LastChecked)
}

func TestApplySourceUpdateFailuresLeaveSetsUntouched(t *testing.T) {

	ctx := context.Background()
	processed := 0
	var undone []TagCountChange
	var moving *ImageSetMongo
	tagger := fakeTagger{undone: &undone}
	tagger.onProcess = func() {

		processed++
		//the source moves between the re-read and the write, so the guarded write matches nothing
		if _, err := Collection.UpdateOne(ctx, bson.M{"_id": moving.ID}, bson.M{"$set": bson.M{"sources.0.source_id": "moved"}}); err != nil {
			t.Errorf("moving the source: %v", err)
		}
	}
	useTestDatabase(t, tagger)

	//a stored attribution naming an image the set doesn't have
	dangling, _ := newCheckSet(checkDate, []string{"p2", "p1", "p3"}, nil)
	dangling.Sources[0].AttributedTo = append(dangling.Sources[0].AttributedTo, bson.NewObjectID())
	moving, _ = newCheckSet(checkDate, []string{"p2", "p1", "p3"}, nil)
	insertSets(t, *dangling, *moving)

	pages := &FetchedPages{Previews: previewsOf("p1", "p2", "p3"), FirstPageWidth: 100, FirstPageHeight: 200}
	update := SourceInfo{Name: "pixiv", SourceID: "1", Title: "New", Date: serviceDate}
	results, err := ApplySourceUpdate(ctx, "user1", update, pages, time.Now())
	if err != nil || len(results) != 2 {
		t.Fatalf("%+v, %v; want a result for each set", results, err)
	}

	if r := results[0]; r.SetID != dangling.ID || r.Kind != SourceReorder || !errors.Is(r.Err, errBrokenAttribution) || r.Changed {
		t.Errorf("set with a dangling attribution: %+v, want its reorder refused by the attribution check", r)
	}

	if r := results[1]; r.SetID != moving.ID || r.Kind != SourceReorder || r.Err == nil || r.Changed {
		t.Errorf("set whose source moved: %+v, want its reorder failed", r)
	}

	if processed != 1 || len(undone) != 1 || undone[0].SourceName != moving.ID.Hex() {
		t.Errorf("tags processed %d times, undone %+v; want only the moved set's counted and undone", processed, undone)
	}

	for _, set := range []*ImageSetMongo{dangling, moving} {
		if src := loadSet(t, set.ID).Sources[0]; !slices.Equal(src.AttributedTo, set.Sources[0].AttributedTo) || src.Title != "Old" || !src.Date.Equal(checkDate) {
			t.Errorf("set %s's copy %+v, want it as stored", set.ID.Hex(), src)
		}
	}
}

func TestWriteEachSetHoldingSourceRereads(t *testing.T) {

	useTestDatabase(t, fakeTagger{})
	ctx := context.Background()
	first, second, third := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	holding := func(id bson.ObjectID, sourceID string) ImageSetMongo {
		return ImageSetMongo{ID: id, KscopeUserId: "user1", Sources: []SourceInfo{{Name: "pixiv", SourceID: sourceID}}}
	}

	//newest first, so only the sort puts them in _id order
	insertSets(t, holding(third, "1"), holding(second, "1"), holding(first, "1"))

	var called []bson.ObjectID
	record := func(set *ImageSetMongo, _ int) SourceWriteResult {
		called = append(called, set.ID)
		return SourceWriteResult{SetID: set.ID}
	}

	expectCalled := func(when string, results []SourceWriteResult, err error, want []bson.ObjectID) {

		t.Helper()
		if err != nil || !slices.Equal(called, want) || len(results) != len(want) {
			t.Errorf("%s: called for %v with results %+v, %v; want %v", when, called, results, err, want)
		}
		called = nil
	}

	results, err := writeEachSetHoldingSource(ctx, "user1", "pixiv", "1", record)
	expectCalled("every set", results, err, []bson.ObjectID{first, second, third})

	//handling the first set deletes the third and takes the source off the second
	results, err = writeEachSetHoldingSource(ctx, "user1", "pixiv", "1", func(set *ImageSetMongo, index int) SourceWriteResult {

		if set.ID == first {
			if _, err := Collection.DeleteOne(ctx, bson.M{"_id": third}); err != nil {
				t.Errorf("deleting the third set: %v", err)
			}

			if _, err := Collection.UpdateOne(ctx, bson.M{"_id": second}, bson.M{"$set": bson.M{"sources": bson.A{bson.M{"name": "zipsrc", "source_id": "9"}}}}); err != nil {
				t.Errorf("taking the source off the second set: %v", err)
			}
		}
		return record(set, index)
	})
	expectCalled("sets changed before their turn", results, err, []bson.ObjectID{first})

	//a set whose images can't be decoded fails its re-read, and the loop carries on
	broken, readable := bson.NewObjectID(), bson.NewObjectID()
	if _, err := Collection.InsertOne(ctx, bson.M{"_id": broken, "kscope_userid": "user1", "sources": bson.A{bson.M{"name": "pixiv", "source_id": "2"}}, "images": "broken"}); err != nil {
		t.Fatal(err)
	}
	insertSets(t, holding(readable, "2"))

	results, err = writeEachSetHoldingSource(ctx, "user1", "pixiv", "2", record)
	if err != nil || len(results) != 2 || !slices.Equal(called, []bson.ObjectID{readable}) {
		t.Fatalf("unreadable set: called for %v with results %+v, %v; want the readable set called and a result each", called, results, err)
	}

	if r := results[0]; r.SetID != broken || r.Kind != SourceReread || r.Err == nil {
		t.Errorf("unreadable set: %+v, want a failed re-read", r)
	}

	if _, err := writeEachSetHoldingSource(ctx, "user1", "pixiv", "3", record); !errors.Is(err, ErrSourceNotStored) {
		t.Errorf("a source no set holds: %v, want ErrSourceNotStored", err)
	}
}
