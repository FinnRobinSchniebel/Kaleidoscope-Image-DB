//go:build integration

package services

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/testdb"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync"
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
	//dated like the listing, so a stale work stays a metadata update
	withSource := func(setID bson.ObjectID, illustID uint64, title string) imageset.ImageSetMongo {
		src := imageset.SourceInfo{Name: pixivServiceName, SourceID: strconv.FormatUint(illustID, 10), Title: title, Date: created, LastChecked: lastChecked}
		return imageset.ImageSetMongo{ID: setID, KscopeUserId: user, Title: title, Sources: []imageset.SourceInfo{src}}
	}

	aCurrent, aStale, b, c, d := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	for _, set := range []imageset.ImageSetMongo{
		withSource(aCurrent, 1, "Art"),
		withSource(aStale, 1, "Old"),
		withSource(b, 2, "Art"),
		withSource(c, 3, "Art"),
		withSource(d, 4, "Old"),
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

	//newest bookmark first, as Pixiv lists them: D, C, B, A; a tag the copies lack makes works 1 and 4 stale
	illust := func(id uint64, tags ...pixivmodel.Tag) pixivmodel.Illust {
		return pixivmodel.Illust{ID: id, Visible: true, Title: "Art", CreateDate: created, Tags: tags}
	}
	newTag := pixivmodel.Tag{Name: "new"}
	page := []pixivmodel.Illust{illust(4, newTag), illust(3), illust(2), illust(1, newTag)}

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
		notStale(withCopy(free, "1", imageset.ImageChangeNone, lastChecked)),
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
	//work 2, hidden by the account's filter; work 1, stale by a new tag and held in one of its sets
	page := []pixivmodel.Illust{
		{ID: 4, ImageURLs: &pixivmodel.Images{SquareMedium: "https://s.pximg.net/common/images/limit_unknown_360.png"}},
		{ID: 3, Visible: true, Title: "Art", CreateDate: created},
		{ID: 2},
		{ID: 1, Visible: true, Title: "New", CreateDate: created, Tags: []pixivmodel.Tag{{Name: "new"}}},
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

func TestProcessBookmarkItemsQueuesImageChecks(t *testing.T) {

	db := testdb.CreateDatabase(t)
	oldCollection, oldTagger, oldEnqueue := imageset.Collection, imageset.Tagger, enqueuePixivWorkTask
	imageset.Collection, imageset.Tagger = db.Collection("imagesets"), fakeTagger{}
	t.Cleanup(func() {
		imageset.Collection, imageset.Tagger, enqueuePixivWorkTask = oldCollection, oldTagger, oldEnqueue
	})

	var queued []Task
	enqueuePixivWorkTask = func(_ string, task Task) error {
		queued = append(queued, task)
		return nil
	}

	const user = "pixiv-route-user"
	created := time.Date(2026, 8, 27, 9, 12, 12, 0, time.UTC)
	behind := created.Add(-time.Hour)
	lastChecked := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	withCopy := func(illustID, title string, date time.Time, state imageset.ImageChangeState) imageset.ImageSetMongo {
		src := imageset.SourceInfo{Name: pixivServiceName, SourceID: illustID, Title: title, Date: date, LastChecked: lastChecked, ImageChangeState: state}
		return imageset.ImageSetMongo{ID: bson.NewObjectID(), KscopeUserId: user, Title: title, Sources: []imageset.SourceInfo{src}}
	}

	moved := withCopy("1", "Art", behind, imageset.ImageChangeNone)
	heldBehind := withCopy("2", "Art", behind, imageset.ImageChangePending)
	stale := withCopy("4", "Old", created, imageset.ImageChangeNone)
	deleted := withCopy("6", "Art", behind, imageset.ImageChangeNone)
	for _, set := range []imageset.ImageSetMongo{
		moved,
		heldBehind,
		withCopy("2", "Art", created, imageset.ImageChangeNone),
		withCopy("3", "Art", behind, imageset.ImageChangePending),
		withCopy("3", "Art", behind, imageset.ImageChangeNone),
		stale,
		deleted,
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

	//PageCount 1 and no URLs, so a queued task fails before any download or database call
	listed := func(id uint64, tags ...pixivmodel.Tag) pixivmodel.Illust {
		return pixivmodel.Illust{ID: id, Visible: true, Title: "Art", CreateDate: created, PageCount: 1, Tags: tags}
	}

	//newest bookmark first: works 1 to 6; work 4 is stale by a tag its copy lacks, work 5 is new, work 6 deleted at Pixiv
	page := []pixivmodel.Illust{
		listed(1), listed(2), listed(3), listed(4, pixivmodel.Tag{Name: "new"}), listed(5),
		{ID: 6, CreateDate: created, ImageURLs: &pixivmodel.Images{SquareMedium: "https://s.pximg.net/common/images/limit_unknown_360.png"}},
	}
	items := processBookmarkItems(context.Background(), user, page, false)

	want := []notification.ItemResult{
		{Kind: notification.ItemMissing, Ref: "6", SetID: deleted.ID},
		{Kind: notification.ItemUpdated, Ref: "4", SetID: stale.ID},
		{Kind: notification.ItemSkipped, Ref: "2", SetID: heldBehind.ID, Reason: "an image update is waiting for review"},
	}
	if !slices.Equal(items, want) {
		t.Errorf("items %+v, want %+v", items, want)
	}

	//each queued task names its work in its one Failed item
	stopped, cancel := context.WithCancel(context.Background())
	cancel()
	var refs []string
	for _, task := range queued {

		results, err := task(stopped)
		if err != nil || len(results) != 1 || results[0].Kind != notification.ItemFailed {
			t.Fatalf("queued task: %+v, %v; want one failed item", results, err)
		}
		refs = append(refs, results[0].Ref)
	}

	if !slices.Equal(refs, []string{"5", "3", "1"}) {
		t.Errorf("queued tasks for works %v, want 5, 3 and 1, oldest bookmark first", refs)
	}

	if got := storedLastChecked(moved.ID); !got.Equal(lastChecked) {
		t.Errorf("work 1's copy: LastChecked %v, want it left for the check to stamp", got)
	}

	enqueuePixivWorkTask = func(string, Task) error { return errors.New("not in the rotation") }
	items = processBookmarkItems(context.Background(), user, []pixivmodel.Illust{listed(1)}, false)
	if len(items) != 1 || items[0].Kind != notification.ItemFailed || items[0].Ref != "1" || items[0].Reason != notification.ErrSyncStopped.Error() {
		t.Errorf("failed queue: %+v, want one failed item for work 1 with the stopped reason", items)
	}

	if got := storedLastChecked(moved.ID); !got.Equal(lastChecked) {
		t.Errorf("work 1's copy after the failed queue: LastChecked %v, want it unstamped", got)
	}
}

// blockImage is a 256x256 picture of an 8x8 grid of random blocks, the same for
// the same seed.
func blockImage(seed int64) image.Image {

	rng := rand.New(rand.NewSource(seed))
	img := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			block := color.NRGBA{R: uint8(rng.Intn(256)), G: uint8(rng.Intn(256)), B: uint8(rng.Intn(256)), A: 255}
			draw.Draw(img, image.Rect(x*32, y*32, x*32+32, y*32+32), &image.Uniform{C: block}, image.Point{}, draw.Src)
		}
	}
	return img
}

// pngClaiming returns a tiny PNG whose header claims width x height.
func pngClaiming(t *testing.T, width, height uint32) []byte {

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}

	//IHDR: width at 16, height at 20, CRC of type and data at 29
	claimed := encoded.Bytes()
	binary.BigEndian.PutUint32(claimed[16:], width)
	binary.BigEndian.PutUint32(claimed[20:], height)
	binary.BigEndian.PutUint32(claimed[29:], crc32.ChecksumIEEE(claimed[12:29]))
	return claimed
}

func TestCheckPixivSourceImages(t *testing.T) {

	db := testdb.CreateDatabase(t)
	oldCollection, oldNotifications, oldTagger, oldEnqueue := imageset.Collection, notification.Collection, imageset.Tagger, enqueuePixivWorkTask
	imageset.Collection, notification.Collection, imageset.Tagger = db.Collection("imagesets"), db.Collection("notifications"), fakeTagger{}
	t.Cleanup(func() {
		imageset.Collection, notification.Collection, imageset.Tagger, enqueuePixivWorkTask = oldCollection, oldNotifications, oldTagger, oldEnqueue
	})

	var queued Task
	enqueuePixivWorkTask = func(_ string, task Task) error {
		queued = task
		return nil
	}

	//each page is served at a preview path and an original path, with the same PNG; requests are counted per path
	var mu sync.Mutex
	served, requests := make(map[string][]byte), make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		mu.Lock()
		requests[r.URL.Path]++
		body, ok := served[r.URL.Path]
		mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(body)
	}))
	defer server.Close()

	requested := func(path string) int {

		mu.Lock()
		defer mu.Unlock()
		return requests[path]
	}

	//stored hashes come from decoding the served bytes, as the check hashes its downloads
	names := []string{"a1", "a2", "a3", "b1", "b2", "b3", "c1", "d1", "d2", "e1", "e2", "g1", "g2", "g3"}
	pageBytes, hashes := make(map[string][]byte), make(map[string]imageset.ImageHashes)
	for i, name := range names {

		var encoded bytes.Buffer
		if err := png.Encode(&encoded, blockImage(int64(i+1))); err != nil {
			t.Fatal(err)
		}

		decoded, err := png.Decode(bytes.NewReader(encoded.Bytes()))
		if err != nil {
			t.Fatal(err)
		}

		if hashes[name], err = imageset.HashImage(decoded); err != nil {
			t.Fatal(err)
		}
		pageBytes[name] = encoded.Bytes()
	}

	//the pages must be far apart and not plain, or the matching would mean nothing
	white := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	draw.Draw(white, white.Bounds(), image.White, image.Point{}, draw.Src)
	whiteHashes, err := imageset.HashImage(white)
	if err != nil {
		t.Fatal(err)
	}

	for i, name := range names {

		if imageset.PDQDistance(hashes[name].PDQ, whiteHashes.PDQ) <= imageset.PlainDistance || imageset.PDQDistance(hashes[name].PDQ, make([]byte, 32)) <= imageset.PlainDistance {
			t.Fatalf("page %s hashes as plain", name)
		}

		for _, other := range names[i+1:] {
			if distance := imageset.PDQDistance(hashes[name].PDQ, hashes[other].PDQ); distance <= imageset.SimilarDistance {
				t.Fatalf("pages %s and %s are %d bits apart, want more than %d", name, other, distance, imageset.SimilarDistance)
			}
		}
	}

	previewPath := func(work, page string) string { return "/" + work + "/" + page + "_master1200.png" }
	originalPath := func(work, page string) string { return "/" + work + "/" + page + ".png" }
	serve := func(work string, pages ...string) {
		for _, page := range pages {
			served[previewPath(work, page)], served[originalPath(work, page)] = pageBytes[page], pageBytes[page]
		}
	}

	d0 := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	d1 := time.Date(2026, 8, 27, 9, 12, 12, 0, time.UTC)
	checked := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	listing := func(id uint64, work string, pages ...string) *pixivmodel.Illust {

		il := &pixivmodel.Illust{ID: id, Visible: true, Title: "New", CreateDate: d1, PageCount: len(pages), Width: 256, Height: 256}
		if len(pages) == 1 {
			il.ImageURLs = &pixivmodel.Images{Large: server.URL + previewPath(work, pages[0])}
			il.MetaSinglePage = &pixivmodel.MetaSinglePage{OriginalImageURL: server.URL + originalPath(work, pages[0])}
			return il
		}

		for _, page := range pages {
			il.MetaPages = append(il.MetaPages, pixivmodel.MetaPage{Images: pixivmodel.Images{Large: server.URL + previewPath(work, page), Original: server.URL + originalPath(work, page)}})
		}
		return il
	}

	const user = "pixiv-check-user"
	storeSet := func(illustID uint64, date, lastChecked time.Time, own, others []string) (imageset.ImageSetMongo, map[string]bson.ObjectID) {

		ids := make(map[string]bson.ObjectID)
		set := imageset.ImageSetMongo{ID: bson.NewObjectID(), KscopeUserId: user, Title: "Old"}
		attribute := func(pages []string) []bson.ObjectID {

			attributed := make([]bson.ObjectID, len(pages))
			for i, page := range pages {

				stored := imageset.ImageInfo{ID: bson.NewObjectID(), ImageHashes: hashes[page], Width: 256, Height: 256}
				set.Image = append(set.Image, stored)
				ids[page], attributed[i] = stored.ID, stored.ID
			}
			return attributed
		}

		src := imageset.SourceInfo{Name: pixivServiceName, SourceID: strconv.FormatUint(illustID, 10), Title: "Old", Date: date, LastChecked: lastChecked, AttributedTo: attribute(own)}
		set.Sources = []imageset.SourceInfo{src}
		if len(others) > 0 {
			set.Sources = append(set.Sources, imageset.SourceInfo{Name: "zipsrc", SourceID: "9", AttributedTo: attribute(others)})
		}

		if _, err := imageset.Collection.InsertOne(context.Background(), set); err != nil {
			t.Fatal(err)
		}
		return set, ids
	}

	storedCopy := func(setID bson.ObjectID) imageset.SourceInfo {

		var set imageset.ImageSetMongo
		if err := imageset.Collection.FindOne(context.Background(), bson.M{"_id": setID}).Decode(&set); err != nil {
			t.Fatal(err)
		}
		return set.Sources[0]
	}

	//the task is queued with the page lookup's snapshot, then run
	check := func(ctx context.Context, il *pixivmodel.Illust) []notification.ItemResult {

		t.Helper()
		sourceID := strconv.FormatUint(il.ID, 10)
		found, err := imageset.GetImageSetsBySourceIDs(user, pixivServiceName, []string{sourceID})
		if err != nil {
			t.Fatal(err)
		}

		if err := enqueuePixivImageCheck(user, *il, false, found[sourceID]); err != nil {
			t.Fatal(err)
		}

		results, err := queued(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return results
	}

	expectItems := func(work string, got, want []notification.ItemResult) {

		t.Helper()
		if !slices.Equal(got, want) {
			t.Errorf("work %s: items %+v, want %+v", work, got, want)
		}
	}

	failed := func(ref string, reason error) []notification.ItemResult {
		return []notification.ItemResult{{Kind: notification.ItemFailed, Ref: ref, Reason: reason.Error()}}
	}

	ctx := context.Background()
	before := time.Now().Truncate(time.Millisecond)

	//A: reordered at Pixiv
	serve("a", "a1", "a2", "a3")
	setA, idsA := storeSet(11, d0, checked, []string{"a1", "a2", "a3"}, nil)
	listingA := listing(11, "a", "a2", "a1", "a3")
	expectItems("A", check(ctx, listingA), []notification.ItemResult{{Kind: notification.ItemUpdated, Ref: "11", SetID: setA.ID, Reason: "the pages were reordered at the source"}})
	if src := storedCopy(setA.ID); !slices.Equal(src.AttributedTo, []bson.ObjectID{idsA["a2"], idsA["a1"], idsA["a3"]}) || !src.Date.Equal(d1) || src.LastChecked.Before(before) {
		t.Errorf("work A's copy %+v: want the new page order, Date %v and a fresh LastChecked", src, d1)
	}

	for _, page := range []string{"a1", "a2", "a3"} {
		if requested(previewPath("a", page)) != 1 || requested(originalPath("a", page)) != 0 {
			t.Errorf("work A, page %s: %d preview and %d original requests, want 1 and 0", page, requested(previewPath("a", page)), requested(originalPath("a", page)))
		}
	}

	//B: a page another source already brought
	serve("b", "b1", "b2", "b3")
	setB, idsB := storeSet(12, d0, checked, []string{"b1", "b3"}, []string{"b2"})
	expectItems("B", check(ctx, listing(12, "b", "b1", "b2", "b3")), []notification.ItemResult{{Kind: notification.ItemUpdated, Ref: "12", SetID: setB.ID, Reason: "matched pages already in the set"}})
	if src := storedCopy(setB.ID); !slices.Equal(src.AttributedTo, []bson.ObjectID{idsB["b1"], idsB["b2"], idsB["b3"]}) {
		t.Errorf("work B's attribution %v, want b1, b2 and b3", src.AttributedTo)
	}

	if requested(originalPath("b", "b1")) != 0 || requested(originalPath("b", "b2")) != 1 || requested(originalPath("b", "b3")) != 0 {
		t.Errorf("work B: originals requested %d, %d and %d times, want b2's only, once", requested(originalPath("b", "b1")), requested(originalPath("b", "b2")), requested(originalPath("b", "b3")))
	}

	//C: a single-page work whose preview is gone
	setC, _ := storeSet(13, d0, checked, []string{"c1"}, nil)
	expectItems("C", check(ctx, listing(13, "c", "c1")), failed("13", notification.ErrDownloadFailed))
	if src := storedCopy(setC.ID); !src.Date.Equal(d0) || !src.LastChecked.Equal(checked) {
		t.Errorf("work C's copy %+v: want it untouched and unstamped", src)
	}

	//D: a page without any URL
	serve("d", "d1", "d2")
	storeSet(14, d0, checked, []string{"d1", "d2"}, nil)
	listingD := listing(14, "d", "d1", "d2")
	listingD.MetaPages[1] = pixivmodel.MetaPage{}
	expectItems("D", check(ctx, listingD), failed("14", notification.ErrDownloadFailed))
	if requested(previewPath("d", "d1")) != 0 {
		t.Errorf("work D: %d preview requests, want none", requested(previewPath("d", "d1")))
	}

	//E: a zip-dated copy whose pages are in place, sized like the listing's first page
	serve("e", "e1", "e2")
	setE, _ := storeSet(15, imageset.DayOnlyDate(d0), time.Time{}, []string{"e1", "e2"}, nil)
	expectItems("E", check(ctx, listing(15, "e", "e1", "e2")), []notification.ItemResult{{Kind: notification.ItemUpdated, Ref: "15", SetID: setE.ID}})
	if src := storedCopy(setE.ID); !src.Date.Equal(d1) || src.LastChecked.Before(before) {
		t.Errorf("work E's copy %+v: want its baseline Date %v and a fresh LastChecked", src, d1)
	}

	//F and F2: previews that can't be decoded, or claim too many pixels
	served[previewPath("f", "f1")] = []byte("not an image")
	storeSet(16, d0, checked, []string{"c1"}, nil)
	expectItems("F", check(ctx, listing(16, "f", "f1")), failed("16", notification.ErrDownloadFailed))

	served[previewPath("f2", "f1")] = pngClaiming(t, 20000, 10001)
	storeSet(17, d0, checked, []string{"c1"}, nil)
	expectItems("F2", check(ctx, listing(17, "f2", "f1")), failed("17", notification.ErrImageTooLarge))

	//G: like B, but the page it needs has no original URL
	serve("g", "g1", "g2", "g3")
	storeSet(18, d0, checked, []string{"g1", "g3"}, []string{"g2"})
	listingG := listing(18, "g", "g1", "g2", "g3")
	listingG.MetaPages[1].Images.Original = ""
	expectItems("G", check(ctx, listingG), failed("18", notification.ErrDownloadFailed))
	for _, page := range []string{"g1", "g2", "g3"} {
		if requested(originalPath("g", page)) != 0 {
			t.Errorf("work G: page %s's original requested, want none", page)
		}
	}

	stopped, cancel := context.WithCancel(ctx)
	cancel()
	expectItems("A, stopped", check(stopped, listingA), failed("11", notification.ErrSyncStopped))
}
