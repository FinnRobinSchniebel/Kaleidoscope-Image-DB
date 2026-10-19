package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// storedImage returns a stored image with a fresh ID whose pixel hash is pixels
// repeated (none when pixels is 0).
func storedImage(pixels byte, active bool) ImageInfo {

	img := ImageInfo{ID: bson.NewObjectID(), Name: bson.NewObjectID().Hex() + ".png", IsImageActive: active}
	if pixels != 0 {
		img.Pixel = bytes.Repeat([]byte{pixels}, pixelHashBytes)
	}
	return img
}

func imageIDs(images []ImageInfo) []bson.ObjectID {
	ids := make([]bson.ObjectID, len(images))
	for i, img := range images {
		ids[i] = img.ID
	}
	return ids
}

func TestCollapseImages(t *testing.T) {

	imgA, imgB := storedImage(1, false), storedImage(2, true)
	copyOfB, imgC, copyOfA, repeatOfC := storedImage(2, true), storedImage(3, false), storedImage(1, true), storedImage(3, true)
	survivor := &ImageSetMongo{ID: bson.NewObjectID(), Image: []ImageInfo{imgA, imgB}}
	absorbed := &ImageSetMongo{ID: bson.NewObjectID(), Image: []ImageInfo{copyOfB, imgC, copyOfA, repeatOfC}}

	merged, idMap, moved, dropped, err := collapseImages(survivor, absorbed)
	if err != nil {
		t.Fatal(err)
	}

	if want := []bson.ObjectID{imgA.ID, imgB.ID, imgC.ID}; !slices.Equal(imageIDs(merged), want) {
		t.Errorf("merged = %v, want [A B C] with C keeping its ID: %v", imageIDs(merged), want)
	}

	wantMap := map[bson.ObjectID]bson.ObjectID{copyOfB.ID: imgB.ID, imgC.ID: imgC.ID, copyOfA.ID: imgA.ID, repeatOfC.ID: imgC.ID}
	for from, to := range wantMap {
		if idMap[from] != to {
			t.Errorf("idMap[%v] = %v, want %v", from, idMap[from], to)
		}
	}

	if !slices.Equal(imageIDs(moved), []bson.ObjectID{imgC.ID}) || !slices.Equal(imageIDs(dropped), []bson.ObjectID{copyOfB.ID, copyOfA.ID, repeatOfC.ID}) {
		t.Errorf("moved %v, dropped %v; want [C] and [B' A' C2]", imageIDs(moved), imageIDs(dropped))
	}

	if !merged[0].IsImageActive || !merged[2].IsImageActive {
		t.Errorf("active flags %t/%t/%t: A and C should be active because a collapsed copy was", merged[0].IsImageActive, merged[1].IsImageActive, merged[2].IsImageActive)
	}

	//images without a pixel hash are never the same image
	unknownA, unknownB := storedImage(0, true), storedImage(0, true)
	merged, _, moved, _, err = collapseImages(&ImageSetMongo{Image: []ImageInfo{unknownA}}, &ImageSetMongo{Image: []ImageInfo{unknownB}})
	if err != nil {
		t.Fatal(err)
	}

	if len(merged) != 2 || len(moved) != 1 {
		t.Errorf("two images without hashes: merged %d, moved %d; want both kept", len(merged), len(moved))
	}
}

func TestMergeSourcesInto(t *testing.T) {

	imgA, imgB, imgC := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	copyOfB, repeatOfC := bson.NewObjectID(), bson.NewObjectID()
	idMap := map[bson.ObjectID]bson.ObjectID{copyOfB: imgB, imgC: imgC, repeatOfC: imgC}

	earlier, later := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC), time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	survivorDate, absorbedDate := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC), time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	survivor := &ImageSetMongo{
		Title:       "Old",
		Description: "kept",
		Sources:     []SourceInfo{{Name: "zipsrc", SourceID: "501", Title: "Old", AttributedTo: []bson.ObjectID{imgA, imgB}, LastChecked: later, Date: survivorDate}},
	}
	absorbed := []SourceInfo{
		{Name: "zipsrc", SourceID: "501", Title: "New", AttributedTo: []bson.ObjectID{copyOfB, imgC, repeatOfC}, LastChecked: earlier, Date: absorbedDate},
		{Name: "other", SourceID: "601", Description: "other text", Tags: []SourceTag{{Default: "cat"}}, AttributedTo: []bson.ObjectID{imgC}},
	}

	indexes := mergeSourcesInto(survivor, absorbed, idMap)
	if !slices.Equal(indexes, []int{0, 1}) {
		t.Fatalf("indexes = %v, want [0 1]", indexes)
	}

	collapsed := survivor.Sources[0]
	if want := []bson.ObjectID{imgB, imgC, imgA}; !slices.Equal(collapsed.AttributedTo, want) {
		t.Errorf("collapsed attribution = %v, want remapped, deduped and unioned %v", collapsed.AttributedTo, want)
	}

	if collapsed.Title != "New" || survivor.Title != "New" {
		t.Errorf("titles %q/%q, want the absorbed copy's, followed by the set", collapsed.Title, survivor.Title)
	}

	if !collapsed.Date.Equal(survivorDate) {
		t.Errorf("Date = %v, want the survivor's, whose copy was checked later", collapsed.Date)
	}

	appended := survivor.Sources[1]
	if len(appended.Tags) != 0 || !slices.Equal(appended.AttributedTo, []bson.ObjectID{imgC}) || survivor.Description != "kept" {
		t.Errorf("appended source %+v, set description %q; want tags cleared, [C], description untouched", appended, survivor.Description)
	}

	uploads := &ImageSetMongo{Sources: []SourceInfo{{Name: "upload", AttributedTo: []bson.ObjectID{imgA}}}}
	if got := mergeSourcesInto(uploads, []SourceInfo{{Name: "upload", AttributedTo: []bson.ObjectID{imgC}}}, idMap); !slices.Equal(got, []int{0}) {
		t.Errorf("ID-less upload: indexes %v, want it collapsed into the existing upload source", got)
	}
}

func TestCollapseSourceBookkeeping(t *testing.T) {

	t1, t2 := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC), time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	exact := time.Date(2026, 9, 5, 14, 30, 0, 0, time.UTC)
	dayOnly := DayOnlyDate(exact)
	survivorBefore := SourceInfo{LastChecked: t1, Date: exact, LastImageUpdate: t1, LastAppliedUpdate: t2}
	other := SourceInfo{LastChecked: t2, SourceMissing: true, ImageChangeState: ImageChangePending, Date: dayOnly, LastImageUpdate: t2, LastAppliedUpdate: t1}

	kept := survivorBefore
	collapseSourceBookkeeping(&kept, survivorBefore, other)
	if !kept.LastChecked.Equal(t2) || !kept.SourceMissing || kept.ImageChangeState != ImageChangePending {
		t.Errorf("kept %+v: want the later-checked copy's LastChecked, missing flag and state", kept)
	}

	if !kept.Date.Equal(exact) {
		t.Errorf("Date = %v, want the exact date kept over a day-only one on the same day", kept.Date)
	}

	if !kept.LastImageUpdate.Equal(t2) || !kept.LastAppliedUpdate.Equal(t2) {
		t.Errorf("LastImageUpdate %v, LastAppliedUpdate %v; want the later of each", kept.LastImageUpdate, kept.LastAppliedUpdate)
	}

	tie := other
	tie.LastChecked = t1
	kept = survivorBefore
	collapseSourceBookkeeping(&kept, survivorBefore, tie)
	if kept.SourceMissing || kept.ImageChangeState != ImageChangeNone {
		t.Errorf("tie: kept %+v, want the survivor's copy to win", kept)
	}
}

func TestMergeSetFields(t *testing.T) {

	earlier, later := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	survivor := &ImageSetMongo{Description: "one", Other: "kept", DateAdded: later}
	mergeSetFields(survivor, &ImageSetMongo{Description: "two", Itype: "illust", Other: "dropped", DateAdded: earlier})

	if survivor.Description != "one\n\ntwo" || survivor.Itype != "illust" || survivor.Other != "kept" || !survivor.DateAdded.Equal(earlier) {
		t.Errorf("survivor = %+v; want descriptions joined, Itype filled, Other kept, the earlier date", survivor)
	}

	same := &ImageSetMongo{Description: "same"}
	mergeSetFields(same, &ImageSetMongo{Description: "same"})
	if same.Description != "same" {
		t.Errorf("equal descriptions became %q", same.Description)
	}
}

func TestFilesToRemove(t *testing.T) {

	held, gone := storedImage(1, true), storedImage(2, true)
	absorbed := []ImageInfo{held, gone}
	survivorIDs := map[bson.ObjectID]bool{held.ID: true}

	if got := imageIDs(filesToRemove(absorbed, survivorIDs, true)); !slices.Equal(got, []bson.ObjectID{gone.ID}) {
		t.Errorf("same folder: %v, want only the image the survivor doesn't hold", got)
	}

	if got := imageIDs(filesToRemove(absorbed, survivorIDs, false)); !slices.Equal(got, imageIDs(absorbed)) {
		t.Errorf("different folders: %v, want every absorbed image", got)
	}
}

func TestLinkIntoFolder(t *testing.T) {

	from, to := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(from, "page.png"), []byte("pixels"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := linkIntoFolder(from, to, "page.png"); err != nil {
		t.Fatal(err)
	}

	if got, err := os.ReadFile(filepath.Join(to, "page.png")); err != nil || string(got) != "pixels" {
		t.Errorf("linked file = %q, %v; want the original content", got, err)
	}

	if err := os.WriteFile(filepath.Join(to, "taken.png"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(from, "taken.png"), []byte("other"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := linkIntoFolder(from, to, "taken.png"); err == nil {
		t.Error("a link over an existing file was allowed")
	}

	if got, _ := os.ReadFile(filepath.Join(to, "taken.png")); string(got) != "keep" {
		t.Errorf("existing file became %q", got)
	}
}

func TestCombineSetsChecksAttributionFirst(t *testing.T) {

	previous := Tagger
	defer func() { Tagger = previous }()

	newSets := func(attributed []bson.ObjectID) (survivor, absorbed *ImageSetMongo, survivorFolder string) {

		survivorFolder = t.TempDir() + "/"
		imgA, imgC := storedImage(1, true), storedImage(3, true)
		survivor = &ImageSetMongo{ID: bson.NewObjectID(), KscopeUserId: "user1", Path: survivorFolder, Image: []ImageInfo{imgA},
			Sources: []SourceInfo{{Name: "a", SourceID: "1", AttributedTo: []bson.ObjectID{imgA.ID}}}}
		if attributed == nil {
			attributed = []bson.ObjectID{imgC.ID}
		}
		absorbed = &ImageSetMongo{ID: bson.NewObjectID(), KscopeUserId: "user1", Path: t.TempDir() + "/", Image: []ImageInfo{imgC},
			Sources: []SourceInfo{{Name: "b", SourceID: "2", AttributedTo: attributed}}}
		return survivor, absorbed, survivorFolder
	}

	//the absorbed source names an image that doesn't exist, which no combine can repair
	processed := false
	Tagger = fakeTagger{processed: &processed}
	survivor, absorbed, survivorFolder := newSets([]bson.ObjectID{bson.NewObjectID()})
	if item, _ := combineSets("user1", "ref", survivor, absorbed); item.Kind != notification.ItemFailed || processed {
		t.Errorf("broken attribution: item %+v, tags processed %t; want failed before any tag work", item, processed)
	}

	if entries, _ := os.ReadDir(survivorFolder); len(entries) != 0 {
		t.Errorf("%d files were linked before the attribution check", len(entries))
	}

	//control: valid sets get past the check and reach the tag counts, which fail here
	processed = false
	Tagger = fakeTagger{processed: &processed, processErr: errors.New("stop before the database")}
	survivor, absorbed, survivorFolder = newSets(nil)
	if err := os.WriteFile(filepath.Join(absorbed.Path, absorbed.Image[0].Name), []byte("pixels"), 0600); err != nil {
		t.Fatal(err)
	}

	if item, _ := combineSets("user1", "ref", survivor, absorbed); item.Kind != notification.ItemFailed || !processed {
		t.Errorf("valid sets: item %+v, tags processed %t; want them to reach ProcessSourceTags", item, processed)
	}

	if entries, _ := os.ReadDir(survivorFolder); len(entries) != 0 {
		t.Errorf("the failed combine left %d linked files behind", len(entries))
	}
}
