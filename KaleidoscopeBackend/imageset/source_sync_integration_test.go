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
	sourceDate := time.Date(2026, 8, 27, 9, 12, 12, 0, time.UTC)

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

	results, err := ApplySourceMetadataUpdate(ctx, "user1", SourceInfo{Name: "pixiv", SourceID: "1", Title: "New", Date: sourceDate}, now)
	expectChanged("metadata update", results, err, true)
	if a, b := loadSet(t, first), loadSet(t, second); a.Sources[0].Title != "New" || b.Sources[1].Title != "New" {
		t.Errorf("stored titles %q and %q, want New in both sets", a.Sources[0].Title, b.Sources[1].Title)
	}

	results, err = MarkSourceMissing(ctx, "user1", "pixiv", "1", now)
	expectChanged("missing mark", results, err, true)
	results, err = MarkSourceMissing(ctx, "user1", "pixiv", "1", now)
	expectChanged("repeated missing mark", results, err, false)

	//the missing mark stored the state as "", which the recovered mark's guard must still match
	results, err = MarkSourceRecovered(ctx, "user1", "pixiv", "1", now)
	expectChanged("recovered mark", results, err, true)

	expectNotified := func(when string) {

		t.Helper()
		got := loadPendingUpdates(t)
		if len(got) != 2 {
			t.Fatalf("%s: %+v, want one pending update notification per set", when, got)
		}

		for i, setID := range []bson.ObjectID{first, second} {

			want := storedPendingUpdate{UserID: "user1", Status: notification.StatusPending, Payload: notification.PendingImageUpdate{SetID: setID, Service: "pixiv", SourceID: "1"}}
			if got[i] != want {
				t.Errorf("%s: notification %d is %+v, want %+v", when, i, got[i], want)
			}
		}
	}

	results, err = MarkSourcePendingImageChange(ctx, "user1", "pixiv", "1", now)
	expectChanged("pending mark", results, err, true)
	expectNotified("pending mark")
	if a, b := loadSet(t, first), loadSet(t, second); !a.Sources[0].Date.Equal(sourceDate) || !b.Sources[1].Date.Equal(sourceDate) {
		t.Errorf("Dates %v and %v after the pending mark, want both left at %v", a.Sources[0].Date, b.Sources[1].Date, sourceDate)
	}

	results, err = MarkSourcePendingImageChange(ctx, "user1", "pixiv", "1", now)
	expectChanged("repeated pending mark", results, err, false)
	expectNotified("repeated pending mark")
	for _, result := range results {
		if result.Held != ImageChangePending {
			t.Errorf("repeated pending mark: set %s: Held %q, want pending", result.SetID.Hex(), result.Held)
		}
	}

	//a missing mark still applies to a held copy, through the guard on the state it read
	results, err = MarkSourceMissing(ctx, "user1", "pixiv", "1", now)
	expectChanged("missing mark on held copies", results, err, true)
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
	insertSets(t, withCopy(free, ImageChangeNone, old), withCopy(pending, ImageChangePending, old), withCopy(declined, ImageChangeDeclined, time.Time{}))

	checkedAt := time.Now().UTC().Truncate(time.Millisecond)
	update := SourceInfo{Name: "pixiv", SourceID: "1", Title: "New", Date: oldDate.Add(time.Hour), Tags: []SourceTag{{Default: "cat"}, {Default: "dog"}}}
	results, err := ApplySourceMetadataUpdate(context.Background(), "user1", update, checkedAt)
	if err != nil || len(results) != 3 {
		t.Fatalf("%+v, %v; want a result for each set", results, err)
	}

	if r := results[0]; r.SetID != free || r.Err != nil || !r.Changed || r.Held != ImageChangeNone {
		t.Errorf("free set: %+v, want changed and not held", r)
	}

	if src := loadSet(t, free).Sources[0]; src.Title != "New" {
		t.Errorf("free set's title %q, want New", src.Title)
	}

	expectHeld := func(i int, setID bson.ObjectID, state ImageChangeState, lastChecked time.Time) {

		t.Helper()
		if r := results[i]; r.SetID != setID || r.Err != nil || r.Changed || r.Held != state {
			t.Errorf("%s set: %+v, want held by %s and not changed", state, r, state)
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

	results, err := ApplySourceMetadataUpdate(context.Background(), "user1", SourceInfo{Name: "pixiv", SourceID: "1", Title: "Same"}, time.Now())
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
		results, err := MarkSourcePendingImageChange(ctx, "user1", "pixiv", sourceID, checkedAt)
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
		results, err := ApplySourceMetadataUpdate(ctx, "user1", SourceInfo{Name: "pixiv", SourceID: sourceID, Title: "New"}, time.Now())
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
