//go:build integration

package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestSuggestMergeReportsFailedPublish(t *testing.T) {

	useTestDatabase(t, fakeTagger{})
	ctx := context.Background()
	setID := bson.NewObjectID()
	pair := notification.ImagePair{ImageID: bson.NewObjectID(), CandidateImageID: bson.NewObjectID(), Match: notification.MatchExact}
	candidates := []mergeCandidate{{set: &ImageSetMongo{ID: bson.NewObjectID()}, pairs: []notification.ImagePair{pair}}}

	db := Collection.Database()
	validator := options.CreateCollection().SetValidator(bson.M{"never": bson.M{"$exists": true}})
	if err := db.CreateCollection(ctx, "rejecting", validator); err != nil {
		t.Fatal(err)
	}

	notifications := notification.Collection
	notification.Collection = db.Collection("rejecting")
	items := suggestMerge("user1", "art/g", setID, candidates, nil)
	notification.Collection = notifications

	if len(items) != 1 || items[0].Kind != notification.ItemFailed || items[0].Ref != "art/g" || items[0].SetID != setID || items[0].Reason != notification.ErrSuggestionSaveFailed.Error() {
		t.Errorf("failed publish: %+v, want one failed item for the set with the suggestion reason", items)
	}

	items = suggestMerge("user1", "art/g", setID, candidates, nil)
	if len(items) != 1 || items[0].Kind != notification.ItemPending || items[0].SetID != setID || items[0].Reason != "suggested a merge with 1 set" {
		t.Errorf("publish: %+v, want one pending item for the set", items)
	}

	stored, err := notification.Collection.CountDocuments(ctx, bson.M{"type": notification.TypeSuggestedMerge, "payload.set_id": setID})
	if err != nil || stored != 1 {
		t.Errorf("stored suggestions: %d (%v), want 1", stored, err)
	}
}

// writeBlockPNGs writes count distinct pictures into dir as PNGs, each a 256x256
// grid of 8x8 random blocks seeded by its index.
func writeBlockPNGs(t *testing.T, dir string, count int) []MediaSource {

	t.Helper()
	files := make([]MediaSource, count)
	for i := range count {

		rng := rand.New(rand.NewSource(int64(i + 1)))
		img := image.NewNRGBA(image.Rect(0, 0, 256, 256))
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				block := color.NRGBA{R: uint8(rng.Intn(256)), G: uint8(rng.Intn(256)), B: uint8(rng.Intn(256)), A: 255}
				draw.Draw(img, image.Rect(x*32, y*32, x*32+32, y*32+32), &image.Uniform{C: block}, image.Point{}, draw.Src)
			}
		}

		var encoded bytes.Buffer
		if err := png.Encode(&encoded, img); err != nil {
			t.Fatal(err)
		}

		path := filepath.Join(dir, fmt.Sprintf("page%d.png", i))
		if err := os.WriteFile(path, encoded.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		files[i] = DiskSource{Path: path}
	}
	return files
}

// storeImages copies pages into set's folder under fresh IDs, as a stored set
// holds them, and returns their entries.
func storeImages(t *testing.T, set *ImageSetMongo, pages ...PreparedImage) []ImageInfo {

	t.Helper()
	images := make([]ImageInfo, len(pages))
	for i, page := range pages {

		file, err := page.Media.Open()
		if err != nil {
			t.Fatal(err)
		}

		content, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}

		images[i] = page.Info
		images[i].ID, images[i].IsImageActive = bson.NewObjectID(), true
		images[i].Name = ImageFileName(set.Title, images[i].ID, page.Format)
		if err := os.WriteFile(filepath.Join(set.Path, images[i].Name), content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return images
}

// storeTestSet builds user's set named name with pages as its images, their
// files in its own folder in the volume. It has no sources and isn't inserted.
func storeTestSet(t *testing.T, user, name string, dateAdded time.Time, pages ...PreparedImage) *ImageSetMongo {

	t.Helper()
	folder := filepath.Join(BackendVolumeLocation, name) + "/"
	if err := os.MkdirAll(folder, 0700); err != nil {
		t.Fatal(err)
	}

	set := &ImageSetMongo{ID: bson.NewObjectID(), KscopeUserId: user, Title: name, Authors: []string{"author"}, Path: folder, DateAdded: dateAdded, AutoTags: []bson.ObjectID{}}
	set.Image = storeImages(t, set, pages...)
	return set
}

func TestImportResetsAndKeepsHeldCopies(t *testing.T) {

	useTestDatabase(t, fakeTagger{})
	volume := BackendVolumeLocation
	BackendVolumeLocation = t.TempDir()
	t.Cleanup(func() { BackendVolumeLocation = volume })
	if err := os.MkdirAll(tempDir(), 0700); err != nil {
		t.Fatal(err)
	}

	files := writeBlockPNGs(t, t.TempDir(), 6)
	pages, err := PrepareImages(files)
	if err != nil {
		t.Fatal(err)
	}

	//the pages must be far apart and not plain, or the imports wouldn't relate to the sets as intended
	for i := range pages {

		if pages[i].Plain {
			t.Fatalf("page %d is plain", i)
		}

		for j := i + 1; j < len(pages); j++ {
			if distance := PDQDistance(pages[i].Info.PDQ, pages[j].Info.PDQ); distance <= SimilarDistance {
				t.Fatalf("pages %d and %d are only %d apart", i, j, distance)
			}
		}
	}

	const user = "held-copies-user"
	ctx := context.Background()
	checked := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	heldDate, zipDate := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC), time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

	//the items as "kind set reason", sorted: the hash lookup's read is unordered
	summarize := func(items []notification.ItemResult) []string {

		summaries := make([]string, len(items))
		for i, item := range items {
			summaries[i] = fmt.Sprintf("%s %s %s", item.Kind, item.SetID.Hex(), item.Reason)
		}
		slices.Sort(summaries)
		return summaries
	}

	//part 1: an import that adds a page to S resets S's held copies, its own source's included
	s := storeTestSet(t, user, "s", checked, pages[0], pages[1])
	s.Sources = []SourceInfo{
		{Name: "zipsrc", SourceID: "1", AttributedTo: []bson.ObjectID{s.Image[0].ID}, LastChecked: checked, Date: heldDate, ImageChangeState: ImageChangePending},
		{Name: "fanbox", SourceID: "2", AttributedTo: []bson.ObjectID{s.Image[1].ID}, LastChecked: checked, ImageChangeState: ImageChangeDeclined},
	}
	insertSets(t, *s)
	sNotes := []bson.ObjectID{
		publishTestPendingUpdate(t, user, notification.PendingUpdateKey{SetID: s.ID, Service: "zipsrc", SourceID: "1"}, notification.StatusPending),
		publishTestPendingUpdate(t, user, notification.PendingUpdateKey{SetID: s.ID, Service: "fanbox", SourceID: "2"}, notification.StatusDeclined),
	}

	incoming := &ImageSetMongo{Title: "import", Sources: []SourceInfo{{Name: "zipsrc", SourceID: "1", Date: zipDate}}}
	items := ProcessImageSet(ctx, incoming, files[0:3], user, "part1")
	if got, want := summarize(items), []string{"updated " + s.ID.Hex() + " added 1 new image to an existing set"}; !slices.Equal(got, want) {
		t.Errorf("part 1 items %q, want %q", got, want)
	}

	stored := loadSet(t, s.ID)
	if len(stored.Image) != 3 || stored.Sources[0].ImageChangeState != ImageChangeNone || stored.Sources[1].ImageChangeState != ImageChangeNone {
		t.Errorf("S: %d images, states %q/%q; want 3 and both reset", len(stored.Image), stored.Sources[0].ImageChangeState, stored.Sources[1].ImageChangeState)
	}

	if !stored.Sources[0].Date.Equal(heldDate) {
		t.Errorf("S's reset copy has Date %v, want its held %v kept", stored.Sources[0].Date, heldDate)
	}

	for _, id := range sNotes {
		if got := loadPendingUpdate(t, id); got.Status != notification.StatusInvalid {
			t.Errorf("S's notification %+v, want it invalid with its copy reset", got)
		}
	}

	//part 2: U keeps its held copies through the import's less and through V combining into it as a less
	u := storeTestSet(t, user, "u", checked, pages[3], pages[4], pages[5])
	u.Sources = []SourceInfo{
		{Name: "pixiv", SourceID: "7", AttributedTo: []bson.ObjectID{u.Image[0].ID, u.Image[1].ID, u.Image[2].ID}, LastChecked: checked, Date: heldDate, ImageChangeState: ImageChangePending},
		{Name: "fanbox", SourceID: "8", AttributedTo: []bson.ObjectID{u.Image[0].ID}, LastChecked: checked, ImageChangeState: ImageChangeDeclined},
	}

	//V is a subset of U that was never merged, which only old data has
	v := storeTestSet(t, user, "v", checked, pages[3])
	v.Sources = []SourceInfo{{Name: "pixiv", SourceID: "7", AttributedTo: []bson.ObjectID{v.Image[0].ID}, Date: zipDate}}
	insertSets(t, *u, *v)
	uPixiv := publishTestPendingUpdate(t, user, notification.PendingUpdateKey{SetID: u.ID, Service: "pixiv", SourceID: "7"}, notification.StatusPending)
	uFanbox := publishTestPendingUpdate(t, user, notification.PendingUpdateKey{SetID: u.ID, Service: "fanbox", SourceID: "8"}, notification.StatusDeclined)

	incoming = &ImageSetMongo{Title: "import", Sources: []SourceInfo{{Name: "zipsrc", SourceID: "3", Date: zipDate}}}
	items = ProcessImageSet(ctx, incoming, files[3:5], user, "part2")
	want := []string{
		"updated " + u.ID.Hex() + " combined with set " + v.ID.Hex(),
		"updated " + u.ID.Hex() + " matched an existing set",
		"updated " + v.ID.Hex() + " added 1 new image to an existing set",
	}
	slices.Sort(want)
	if got := summarize(items); !slices.Equal(got, want) {
		t.Errorf("part 2 items %q, want %q", got, want)
	}

	if left, err := Collection.CountDocuments(ctx, bson.M{"_id": v.ID}); err != nil || left != 0 {
		t.Errorf("V: %d left (%v), want it combined away", left, err)
	}

	stored = loadSet(t, u.ID)
	if got := imageIDs(stored.Image); !slices.Equal(got, imageIDs(u.Image)) {
		t.Errorf("U's images %v, want its own %v", got, imageIDs(u.Image))
	}

	if stored.Sources[0].ImageChangeState != ImageChangePending || stored.Sources[1].ImageChangeState != ImageChangeDeclined {
		t.Errorf("U's states %q/%q; want pending and declined kept", stored.Sources[0].ImageChangeState, stored.Sources[1].ImageChangeState)
	}

	if got := loadPendingUpdate(t, uPixiv); got.Status != notification.StatusPending || got.Payload.SetID != u.ID {
		t.Errorf("U's pixiv notification %+v, want it still pending on U", got)
	}

	if got := loadPendingUpdate(t, uFanbox); got.Status != notification.StatusDeclined || got.Payload.SetID != u.ID {
		t.Errorf("U's fanbox notification %+v, want it still declined on U", got)
	}
}

func TestImportSuggestsNoSetBySourceAlone(t *testing.T) {

	useTestDatabase(t, fakeTagger{})
	useTestVolume(t)

	files := writeBlockPNGs(t, t.TempDir(), 4)
	pages, err := PrepareImages(files)
	if err != nil {
		t.Fatal(err)
	}

	//no two pages may relate by hash, or a suggestion could come from the images
	for i := range pages {

		if pages[i].Plain {
			t.Fatalf("page %d is plain", i)
		}

		for j := i + 1; j < len(pages); j++ {
			if distance := PDQDistance(pages[i].Info.PDQ, pages[j].Info.PDQ); distance <= SimilarDistance {
				t.Fatalf("pages %d and %d are only %d apart", i, j, distance)
			}
		}
	}

	const user = "source-alone-user"
	ctx := context.Background()
	added := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

	w := storeTestSet(t, user, "w", added, pages[0])
	w.Sources = []SourceInfo{{Name: "zipsrc", SourceID: "1", AttributedTo: []bson.ObjectID{w.Image[0].ID}}}
	target := storeTestSet(t, user, "t", added, pages[2])
	target.Sources = []SourceInfo{{Name: "fanbox", SourceID: "2", AttributedTo: []bson.ObjectID{target.Image[0].ID}}}
	u := storeTestSet(t, user, "u", added, pages[3])
	u.Sources = []SourceInfo{{Name: "zipsrc", SourceID: "3", AttributedTo: []bson.ObjectID{u.Image[0].ID}}}
	insertSets(t, *w, *target, *u)

	//case a: an import sharing only W's source becomes a new set, and W isn't suggested
	incoming := &ImageSetMongo{Title: "import", Sources: []SourceInfo{{Name: "zipsrc", SourceID: "1"}}}
	items := ProcessImageSet(ctx, incoming, files[1:2], user, "a")
	if len(items) != 1 || items[0].Kind != notification.ItemAdded || items[0].SetID != incoming.ID {
		t.Errorf("case a items %+v, want only the new set's Added item", items)
	}

	//case b: a duplicate of T brings U's source to T, and U isn't suggested to T
	incoming = &ImageSetMongo{Title: "import", Sources: []SourceInfo{{Name: "zipsrc", SourceID: "3"}}}
	items = ProcessImageSet(ctx, incoming, files[2:3], user, "b")
	matched := notification.ItemResult{Kind: notification.ItemUpdated, Ref: "b", SetID: target.ID, Reason: "matched an existing set"}
	if !slices.Equal(items, []notification.ItemResult{matched}) {
		t.Errorf("case b items %+v, want only T's %+v", items, matched)
	}

	if stored := loadSet(t, target.ID); !slices.ContainsFunc(stored.Sources, func(src SourceInfo) bool { return SameSource(src, u.Sources[0]) }) {
		t.Errorf("T's sources %+v, want zipsrc/3 attached", stored.Sources)
	}

	var suggestions []bson.M
	cursor, err := notification.Collection.Find(ctx, bson.M{"type": notification.TypeSuggestedMerge})
	if err == nil {
		err = cursor.All(ctx, &suggestions)
	}

	if err != nil || len(suggestions) != 0 {
		t.Errorf("stored suggestions %v (%v), want none", suggestions, err)
	}
}

func TestMergeFlowMovesHeldCopyNotifications(t *testing.T) {

	useTestDatabase(t, fakeTagger{})
	useTestVolume(t)

	files := writeBlockPNGs(t, t.TempDir(), 8)
	pages, err := PrepareImages(files)
	if err != nil {
		t.Fatal(err)
	}

	//the pictures must be far apart, so sets relate only through the pages they share
	for i := range pages {

		if pages[i].Plain {
			t.Fatalf("page %d is plain", i)
		}

		for j := i + 1; j < len(pages); j++ {
			if distance := PDQDistance(pages[i].Info.PDQ, pages[j].Info.PDQ); distance <= SimilarDistance {
				t.Fatalf("pages %d and %d are only %d apart", i, j, distance)
			}
		}
	}

	const user = "moves-user"
	ctx := context.Background()
	checked := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

	//a stored set whose one source, in state, is attributed to all its pages
	newSet := func(name, service, sourceID string, state ImageChangeState, pages ...PreparedImage) *ImageSetMongo {

		t.Helper()
		set := storeTestSet(t, user, name, checked, pages...)
		set.Sources = []SourceInfo{{Name: service, SourceID: sourceID, AttributedTo: imageIDs(set.Image), LastChecked: checked, ImageChangeState: state}}
		insertSets(t, *set)
		return set
	}

	runFlow := func(entry *ImageSetMongo) {

		t.Helper()
		prepared, err := storedSetAsPreparedImages(entry.Image)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := mergeFlow(user, "ref", entry, prepared, false); err != nil {
			t.Fatalf("flow of %s: %v", entry.Title, err)
		}
	}

	stateOf := func(setID bson.ObjectID, service, sourceID string) ImageChangeState {

		t.Helper()
		stored := loadSet(t, setID)
		index := slices.IndexFunc(stored.Sources, func(src SourceInfo) bool { return src.Name == service && src.SourceID == sourceID })
		if index < 0 {
			t.Fatalf("set %s has no %s/%s", stored.Title, service, sourceID)
		}
		return stored.Sources[index].ImageChangeState
	}

	exists := func(setID bson.ObjectID) bool {

		t.Helper()
		count, err := Collection.CountDocuments(ctx, bson.M{"_id": setID})
		if err != nil {
			t.Fatal(err)
		}
		return count == 1
	}

	//retarget: E extends T, so E's hold moves to T and E is removed
	tA := newSet("t", "zipsrc", "1", ImageChangeNone, pages[0])
	e := newSet("e", "pixiv", "7", ImageChangePending, pages[0], pages[1])
	eNote := publishTestPendingUpdate(t, user, notification.PendingUpdateKey{SetID: e.ID, Service: "pixiv", SourceID: "7"}, notification.StatusPending)
	runFlow(e)

	if exists(e.ID) || stateOf(tA.ID, "pixiv", "7") != ImageChangePending {
		t.Errorf("retarget: E left %t, T's pixiv copy %q; want E gone and its hold on T", exists(e.ID), stateOf(tA.ID, "pixiv", "7"))
	}

	if got := loadPendingUpdate(t, eNote); got.Status != notification.StatusPending || got.Payload.SetID != tA.ID {
		t.Errorf("retarget: E's notification %+v, want it pending on T", got)
	}

	//replaced: E2's pending copy wins over T2's declined one of the same source
	t2 := newSet("t2", "fanbox", "8", ImageChangeDeclined, pages[2])
	e2 := newSet("e2", "fanbox", "8", ImageChangePending, pages[2], pages[3])
	t2Note := publishTestPendingUpdate(t, user, notification.PendingUpdateKey{SetID: t2.ID, Service: "fanbox", SourceID: "8"}, notification.StatusDeclined)
	e2Note := publishTestPendingUpdate(t, user, notification.PendingUpdateKey{SetID: e2.ID, Service: "fanbox", SourceID: "8"}, notification.StatusPending)
	runFlow(e2)

	if got := stateOf(t2.ID, "fanbox", "8"); got != ImageChangePending {
		t.Errorf("replaced: T2's fanbox copy %q, want E2's pending kept", got)
	}

	if got := loadPendingUpdate(t, t2Note); got.Status != notification.StatusInvalid {
		t.Errorf("replaced: T2's own notification %+v, want it invalid", got)
	}

	if got := loadPendingUpdate(t, e2Note); got.Status != notification.StatusPending || got.Payload.SetID != t2.ID {
		t.Errorf("replaced: E2's notification %+v, want it pending on T2", got)
	}

	//two targets collapsing: E3 extends both, which then combine as duplicates
	t3a := newSet("t3a", "zipsrc", "31", ImageChangeNone, pages[4])
	t3b := newSet("t3b", "zipsrc", "32", ImageChangeNone, pages[5])
	e3 := newSet("e3", "pixiv", "9", ImageChangePending, pages[4], pages[5])
	e3Note := publishTestPendingUpdate(t, user, notification.PendingUpdateKey{SetID: e3.ID, Service: "pixiv", SourceID: "9"}, notification.StatusPending)
	runFlow(e3)

	survivor := t3b.ID
	if exists(t3a.ID) {
		survivor = t3a.ID
	}

	if exists(e3.ID) || exists(t3a.ID) == exists(t3b.ID) {
		t.Errorf("two targets: E3 left %t, T3a %t, T3b %t; want only one target left", exists(e3.ID), exists(t3a.ID), exists(t3b.ID))
	}

	if got := loadPendingUpdate(t, e3Note); got.Status != notification.StatusPending || got.Payload.SetID != survivor {
		t.Errorf("two targets: E3's notification %+v, want it pending on the target left", got)
	}

	live := bson.M{"type": notification.TypePendingImageUpdate, "payload.source_id": "9", "status": notification.StatusPending}
	if count, err := notification.Collection.CountDocuments(ctx, live); err != nil || count != 1 {
		t.Errorf("two targets: %d live notifications for pixiv/9 (%v), want 1", count, err)
	}

	//interrupted: E4's removal misses after both writes landed; the targets are made first, so E4 sorts last
	t4a := newSet("t4a", "zipsrc", "41", ImageChangeNone, pages[6])
	t4b := newSet("t4b", "zipsrc", "42", ImageChangeNone, pages[7])
	e4 := newSet("e4", "zipsrc", "4", ImageChangePending, pages[6], pages[7])
	e4Note := publishTestPendingUpdate(t, user, notification.PendingUpdateKey{SetID: e4.ID, Service: "zipsrc", SourceID: "4"}, notification.StatusPending)

	armed := true
	Tagger = fakeTagger{onProcess: func() {

		if !armed {
			return
		}
		armed = false

		result, err := Collection.UpdateOne(ctx, bson.M{"_id": e4.ID}, bson.M{"$push": bson.M{"images": bson.M{"id": bson.NewObjectID()}}})
		if err != nil || result.MatchedCount != 1 {
			t.Errorf("adding an image to E4: matched %v (%v), want 1", result, err)
		}
	}}
	runFlow(e4)

	if !exists(e4.ID) || stateOf(t4a.ID, "zipsrc", "4") != ImageChangePending || stateOf(t4b.ID, "zipsrc", "4") != ImageChangePending {
		t.Errorf("interrupted: E4 left %t; want E4 kept and its hold on T4a and T4b", exists(e4.ID))
	}

	if got := loadPendingUpdate(t, e4Note); got.Status != notification.StatusPending || got.Payload.SetID != e4.ID {
		t.Errorf("interrupted: E4's notification %+v, want it pending on E4", got)
	}

	for name, target := range map[string]bson.ObjectID{"T4a": t4a.ID, "T4b": t4b.ID} {

		copied := bson.M{"type": notification.TypePendingImageUpdate, "payload.set_id": target, "payload.source_id": "4", "status": notification.StatusPending, "created_at": testPublished}
		if count, err := notification.Collection.CountDocuments(ctx, copied); err != nil || count != 1 {
			t.Errorf("interrupted: %d copies on %s with E4's CreatedAt (%v), want 1", count, name, err)
		}
	}
}
