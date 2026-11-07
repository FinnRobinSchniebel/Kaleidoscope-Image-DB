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
	items := suggestMerge("user1", "art/g", setID, candidates, nil, nil)
	notification.Collection = notifications

	if len(items) != 1 || items[0].Kind != notification.ItemFailed || items[0].Ref != "art/g" || items[0].SetID != setID || items[0].Reason != notification.ErrSuggestionSaveFailed.Error() {
		t.Errorf("failed publish: %+v, want one failed item for the set with the suggestion reason", items)
	}

	items = suggestMerge("user1", "art/g", setID, candidates, nil, nil)
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
	newSet := func(name string, pages ...PreparedImage) *ImageSetMongo {

		folder := filepath.Join(BackendVolumeLocation, name) + "/"
		if err := os.MkdirAll(folder, 0700); err != nil {
			t.Fatal(err)
		}

		set := &ImageSetMongo{ID: bson.NewObjectID(), KscopeUserId: user, Title: name, Authors: []string{"author"}, Path: folder, DateAdded: checked, AutoTags: []bson.ObjectID{}}
		set.Image = storeImages(t, set, pages...)
		return set
	}

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
	s := newSet("s", pages[0], pages[1])
	s.Sources = []SourceInfo{
		{Name: "zipsrc", SourceID: "1", AttributedTo: []bson.ObjectID{s.Image[0].ID}, LastChecked: checked, Date: heldDate, ImageChangeState: ImageChangePending},
		{Name: "fanbox", SourceID: "2", AttributedTo: []bson.ObjectID{s.Image[1].ID}, LastChecked: checked, ImageChangeState: ImageChangeDeclined},
	}
	insertSets(t, *s)

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

	//part 2: U keeps its held copies through the import's less and through V combining into it as a less
	u := newSet("u", pages[3], pages[4], pages[5])
	u.Sources = []SourceInfo{
		{Name: "pixiv", SourceID: "7", AttributedTo: []bson.ObjectID{u.Image[0].ID, u.Image[1].ID, u.Image[2].ID}, LastChecked: checked, Date: heldDate, ImageChangeState: ImageChangePending},
		{Name: "fanbox", SourceID: "8", AttributedTo: []bson.ObjectID{u.Image[0].ID}, LastChecked: checked, ImageChangeState: ImageChangeDeclined},
	}

	//V is a subset of U that was never merged, which only old data has
	v := newSet("v", pages[3])
	v.Sources = []SourceInfo{{Name: "pixiv", SourceID: "7", AttributedTo: []bson.ObjectID{v.Image[0].ID}, Date: zipDate}}
	insertSets(t, *u, *v)

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
}
