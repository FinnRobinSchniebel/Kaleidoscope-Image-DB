package imageset

import (
	"bytes"
	"errors"
	"maps"
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

func TestCollapseMergedSetImages(t *testing.T) {

	imgA, imgB := storedImage(1, false), storedImage(2, true)
	copyOfB, imgC, copyOfA, repeatOfC := storedImage(2, true), storedImage(3, false), storedImage(1, true), storedImage(3, true)
	survivor := &ImageSetMongo{ID: bson.NewObjectID(), Image: []ImageInfo{imgA, imgB}}
	absorbed := &ImageSetMongo{ID: bson.NewObjectID(), Image: []ImageInfo{copyOfB, imgC, copyOfA, repeatOfC}}

	images, idMap, err := collapseMergedSetImages(survivor, absorbed)
	if err != nil {
		t.Fatal(err)
	}

	freshC := idMap[imgC.ID]
	if freshC.IsZero() || freshC == imgC.ID || idMap[repeatOfC.ID] != freshC {
		t.Errorf("C kept as %v, its repeat as %v; want both on one fresh ID, not C's own %v", freshC, idMap[repeatOfC.ID], imgC.ID)
	}

	if idMap[copyOfB.ID] != imgB.ID || idMap[copyOfA.ID] != imgA.ID {
		t.Errorf("idMap = %v; want the collapsed copies on the survivor's B and A", idMap)
	}

	if want := []bson.ObjectID{imgA.ID, imgB.ID, freshC}; !slices.Equal(imageIDs(images), want) {
		t.Errorf("images = %v, want [A B C'] with C's copy after B: %v", imageIDs(images), want)
	}

	if images[0].IsImageActive || images[2].IsImageActive || !bytes.Equal(images[2].Pixel, imgC.Pixel) || images[2].Name != imgC.Name {
		t.Errorf("A and C' active %t/%t, C' %+v; want the stored flags (the active-if-either rule is the plan's), and C's hashes and name", images[0].IsImageActive, images[2].IsImageActive, images[2])
	}

	if !slices.Equal(imageIDs(survivor.Image), []bson.ObjectID{imgA.ID, imgB.ID}) || survivor.Image[0].IsImageActive {
		t.Errorf("the survivor's own images changed: %+v", survivor.Image)
	}

	//images without a pixel hash are never the same image
	unknownA, unknownB := storedImage(0, true), storedImage(0, true)
	images, idMap, err = collapseMergedSetImages(&ImageSetMongo{Image: []ImageInfo{unknownA}}, &ImageSetMongo{Image: []ImageInfo{unknownB}})
	if err != nil {
		t.Fatal(err)
	}

	if kept := idMap[unknownB.ID]; len(images) != 2 || kept.IsZero() || kept == unknownB.ID {
		t.Errorf("two images without hashes: %d images, the absorbed one kept as %v; want both, the absorbed one under a fresh ID", len(images), kept)
	}
}

func TestMakeAutoMergePlan(t *testing.T) {

	imgA, imgB := storedImage(1, false), storedImage(2, true)
	copyOfA, imgC, repeatOfC, imgD := storedImage(1, true), storedImage(3, false), storedImage(3, true), storedImage(4, false)
	survivor := &ImageSetMongo{ID: bson.NewObjectID(), Description: "one", Image: []ImageInfo{imgA, imgB}}
	absorbed := &ImageSetMongo{ID: bson.NewObjectID(), Description: "two", Image: []ImageInfo{copyOfA, imgC, repeatOfC, imgD}}

	plan, err := makeAutoMergePlan(survivor, absorbed)
	if err != nil {
		t.Fatal(err)
	}

	freshC, freshD := plan.idMap[imgC.ID], plan.idMap[imgD.ID]
	if want := []bson.ObjectID{imgA.ID, freshC, freshD, imgB.ID}; plan.survivor != survivor || plan.absorbed != absorbed || !slices.Equal(plan.order, want) {
		t.Errorf("order = %v, want the automatic [A C' D' B]: %v", plan.order, want)
	}

	wantCopies := map[bson.ObjectID]bson.ObjectID{freshC: imgC.ID, freshD: imgD.ID}
	if freshC == imgC.ID || freshD == imgD.ID || !maps.Equal(plan.copiedFrom, wantCopies) {
		t.Errorf("copiedFrom = %v, want each fresh ID mapped to the absorbed image it copies, the repeat not again: %v", plan.copiedFrom, wantCopies)
	}

	if len(plan.inactive) != 1 || !plan.inactive[freshD] {
		t.Errorf("inactive = %v; want only D', since A and C have an active copy", plan.inactive)
	}

	if plan.description != "one\n\ntwo" {
		t.Errorf("description = %q, want the two joined", plan.description)
	}

	if survivor.Image[0].IsImageActive || survivor.Description != "one" || absorbed.Image[1].IsImageActive {
		t.Errorf("a set changed: survivor %+v, absorbed %+v", survivor, absorbed)
	}

	same, err := makeAutoMergePlan(&ImageSetMongo{Description: "same"}, &ImageSetMongo{Description: "same"})
	if err != nil {
		t.Fatal(err)
	}

	if same.description != "same" {
		t.Errorf("equal descriptions became %q", same.description)
	}
}

func TestOrderedImages(t *testing.T) {

	imgA, imgB, imgC := storedImage(1, true), storedImage(2, true), storedImage(3, false)
	copyOfC := bson.NewObjectID()
	plan := mergePlan{
		survivor:   &ImageSetMongo{Image: []ImageInfo{imgA, imgB}},
		absorbed:   &ImageSetMongo{Image: []ImageInfo{imgC}},
		order:      []bson.ObjectID{copyOfC, imgB.ID, imgA.ID},
		inactive:   map[bson.ObjectID]bool{imgB.ID: true},
		copiedFrom: map[bson.ObjectID]bson.ObjectID{copyOfC: imgC.ID},
	}

	images, err := plan.orderedImages()
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(imageIDs(images), plan.order) || images[1].Name != imgB.Name {
		t.Errorf("images %v, B named %q; want the plan's order and the survivor's entry %q", imageIDs(images), images[1].Name, imgB.Name)
	}

	if !images[0].IsImageActive || images[1].IsImageActive || !images[2].IsImageActive {
		t.Errorf("active flags %t/%t/%t; want them from inactive, not the stored flags", images[0].IsImageActive, images[1].IsImageActive, images[2].IsImageActive)
	}

	//an absorbed image under its own ID would make the survivor list the absorbed set's file
	ownID := plan
	ownID.order = append(slices.Clone(plan.order), imgC.ID)
	if _, err := ownID.orderedImages(); err == nil {
		t.Error("an absorbed image under its own ID was accepted")
	}

	plan.order = append(plan.order, bson.NewObjectID())
	if _, err := plan.orderedImages(); err == nil {
		t.Error("an ID in neither set was accepted")
	}

	//a copy: the absorbed entry under its fresh ID, named for the survivor with the file's own extension
	animated := storedImage(4, true)
	animated.Name, animated.LowResName = "orig_title.gif", "orig-low.png"
	copyID := bson.NewObjectID()
	copyPlan := mergePlan{
		survivor:   &ImageSetMongo{Title: "Kept", Image: []ImageInfo{imgA}},
		absorbed:   &ImageSetMongo{Image: []ImageInfo{animated}},
		order:      []bson.ObjectID{imgA.ID, copyID},
		copiedFrom: map[bson.ObjectID]bson.ObjectID{copyID: animated.ID},
	}

	images, err = copyPlan.orderedImages()
	if err != nil {
		t.Fatal(err)
	}

	copied := images[1]
	if copied.ID != copyID || !bytes.Equal(copied.Pixel, animated.Pixel) || copied.Name != ImageFileName("Kept", copyID, "gif") || copied.LowResName != "" {
		t.Errorf("copy = %+v; want the absorbed hashes under the fresh ID, a new .gif name and no low-res", copied)
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

func TestCollapseMergedSourceBookkeeping(t *testing.T) {

	t1, t2 := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC), time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	exact := time.Date(2026, 9, 5, 14, 30, 0, 0, time.UTC)
	dayOnly := DayOnlyDate(exact)
	survivorBefore := SourceInfo{LastChecked: t1, Date: exact, LastImageUpdate: t1, LastAppliedUpdate: t2}
	other := SourceInfo{LastChecked: t2, SourceMissing: true, ImageChangeState: ImageChangePending, Date: dayOnly, LastImageUpdate: t2, LastAppliedUpdate: t1}

	kept := survivorBefore
	collapseMergedSourceBookkeeping(&kept, survivorBefore, other)
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
	collapseMergedSourceBookkeeping(&kept, survivorBefore, tie)
	if kept.SourceMissing || kept.ImageChangeState != ImageChangeNone {
		t.Errorf("tie: kept %+v, want the survivor's copy to win", kept)
	}
}

func TestMergeSetFields(t *testing.T) {

	earlier, later := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	survivor := &ImageSetMongo{Description: "one", Other: "kept", DateAdded: later}
	mergeSetFields(survivor, &ImageSetMongo{Description: "two", Itype: "illust", Other: "dropped", DateAdded: earlier})

	if survivor.Description != "one" || survivor.Itype != "illust" || survivor.Other != "kept" || !survivor.DateAdded.Equal(earlier) {
		t.Errorf("survivor = %+v; want the description untouched, Itype filled, Other kept, the earlier date", survivor)
	}
}

func TestLinkIntoFolder(t *testing.T) {

	from, to := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(from, "page.png"), []byte("pixels"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := linkIntoFolder(from, "page.png", to, "page.png"); err != nil {
		t.Fatal(err)
	}

	if got, err := os.ReadFile(filepath.Join(to, "page.png")); err != nil || string(got) != "pixels" {
		t.Errorf("linked file = %q, %v; want the original content", got, err)
	}

	if err := linkIntoFolder(from, "page.png", from, "renamed.png"); err != nil {
		t.Fatal(err)
	}

	if got, err := os.ReadFile(filepath.Join(from, "renamed.png")); err != nil || string(got) != "pixels" {
		t.Errorf("same-folder link = %q, %v; want the original content under the new name", got, err)
	}

	if err := os.WriteFile(filepath.Join(to, "taken.png"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(from, "taken.png"), []byte("other"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := linkIntoFolder(from, "taken.png", to, "taken.png"); err == nil {
		t.Error("a link over an existing file was allowed")
	}

	if got, _ := os.ReadFile(filepath.Join(to, "taken.png")); string(got) != "keep" {
		t.Errorf("existing file became %q", got)
	}
}

// twoSetPlan returns the automatic plan for a survivor [A] and an absorbed [C] in
// separate folders, with C's file written. attributed replaces the absorbed
// source's attribution when non-nil.
func twoSetPlan(t *testing.T, attributed []bson.ObjectID) mergePlan {

	imgA, imgC := storedImage(1, true), storedImage(3, true)
	imgA.LowResName, imgC.LowResName = "a-low.png", "c-low.png"
	if attributed == nil {
		attributed = []bson.ObjectID{imgC.ID}
	}

	survivor := &ImageSetMongo{ID: bson.NewObjectID(), KscopeUserId: "user1", Path: t.TempDir() + "/", Image: []ImageInfo{imgA},
		Sources: []SourceInfo{{Name: "a", SourceID: "1", AttributedTo: []bson.ObjectID{imgA.ID}}}}
	absorbed := &ImageSetMongo{ID: bson.NewObjectID(), KscopeUserId: "user1", Path: t.TempDir() + "/", Image: []ImageInfo{imgC},
		Sources: []SourceInfo{{Name: "b", SourceID: "2", AttributedTo: attributed}}}
	if err := os.WriteFile(filepath.Join(absorbed.Path, imgC.Name), []byte("pixels"), 0600); err != nil {
		t.Fatal(err)
	}

	plan, err := makeAutoMergePlan(survivor, absorbed)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestApplyMergePlanRejectsBadPlans(t *testing.T) {

	previous := Tagger
	defer func() { Tagger = previous }()

	brokenAttribution := twoSetPlan(t, []bson.ObjectID{bson.NewObjectID()})
	missingImage := twoSetPlan(t, nil)
	missingImage.order = missingImage.order[:1]
	self := twoSetPlan(t, nil)
	self, err := makeAutoMergePlan(self.survivor, self.survivor)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		plan mergePlan
	}{
		{"the absorbed source attributes an image that doesn't exist", brokenAttribution},
		{"the order misses one of the plan's images", missingImage},
		{"a set merged with itself", self},
	}
	for _, tt := range tests {

		//the absorbed file exists and the tagger stops the merge, so a skipped check would reach the tags
		processed := false
		Tagger = fakeTagger{processed: &processed, processErr: errors.New("stop before the database")}
		if _, _, err := applyMergePlan("user1", tt.plan); err == nil || processed {
			t.Errorf("%s: err %v, tags processed %t; want an error before any tag work", tt.name, err, processed)
		}

		if entries, _ := os.ReadDir(tt.plan.survivor.Path); len(entries) != 0 {
			t.Errorf("%s: %d files were linked", tt.name, len(entries))
		}
	}

	//sets sharing an image are refused: removing the absorbed set would delete the shared file
	folder := t.TempDir() + "/"
	shared, imgC := storedImage(2, true), storedImage(3, true)
	survivor := &ImageSetMongo{ID: bson.NewObjectID(), KscopeUserId: "user1", Path: folder, Image: []ImageInfo{shared},
		Sources: []SourceInfo{{Name: "a", SourceID: "1", AttributedTo: []bson.ObjectID{shared.ID}}}}
	absorbed := &ImageSetMongo{ID: bson.NewObjectID(), KscopeUserId: "user1", Path: folder, Image: []ImageInfo{shared, imgC},
		Sources: []SourceInfo{{Name: "b", SourceID: "2", AttributedTo: []bson.ObjectID{shared.ID, imgC.ID}}}}
	for _, name := range []string{shared.Name, imgC.Name} {
		if err := os.WriteFile(filepath.Join(folder, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}

	plan, err := makeAutoMergePlan(survivor, absorbed)
	if err != nil {
		t.Fatal(err)
	}

	processed := false
	Tagger = fakeTagger{processed: &processed, processErr: errors.New("stop before the database")}
	if _, _, err := applyMergePlan("user1", plan); err == nil || processed {
		t.Errorf("shared image: err %v, tags processed %t; want it refused before any link or tag work", err, processed)
	}

	if entries, _ := os.ReadDir(folder); len(entries) != 2 {
		t.Errorf("shared image: %d files in the folder, want the 2 untouched", len(entries))
	}
}

func TestApplyMergePlanAppliesPlan(t *testing.T) {

	previous := Tagger
	defer func() { Tagger = previous }()

	processed := false
	Tagger = fakeTagger{processed: &processed, processErr: errors.New("stop before the database")}
	plan := twoSetPlan(t, nil)
	survivor, imgA, imgC := plan.survivor, plan.survivor.Image[0], plan.absorbed.Image[0]
	copyOfC := plan.idMap[imgC.ID]
	plan.order = []bson.ObjectID{imgA.ID, copyOfC}
	plan.inactive = map[bson.ObjectID]bool{copyOfC: true}
	plan.description = "chosen"

	if _, _, err := applyMergePlan("user1", plan); err == nil || !processed {
		t.Fatalf("err %v, tags processed %t; want the merge to reach ProcessSourceTags", err, processed)
	}

	if !slices.Equal(imageIDs(survivor.Image), plan.order) || !survivor.Image[0].IsImageActive || survivor.Image[1].IsImageActive || survivor.Description != "chosen" {
		t.Errorf("survivor %v, active %t/%t, description %q; want the plan's [A C'], C' inactive, \"chosen\"",
			imageIDs(survivor.Image), survivor.Image[0].IsImageActive, survivor.Image[1].IsImageActive, survivor.Description)
	}

	if copied := survivor.Image[1]; survivor.Image[0].LowResName != "a-low.png" || copied.LowResName != "" || copied.Name != ImageFileName("", copyOfC, "png") {
		t.Errorf("A's low-res %q, copy %+v; want A's kept, and the copy named for its fresh ID with no low-res", survivor.Image[0].LowResName, copied)
	}

	if entries, _ := os.ReadDir(survivor.Path); len(entries) != 0 {
		t.Errorf("the failed merge left %d linked files behind", len(entries))
	}

	//in one folder, the copy is a second name for the absorbed file; the survivor never lists the absorbed name
	folder := t.TempDir() + "/"
	imgA, imgC = storedImage(1, true), storedImage(3, true)
	survivor = &ImageSetMongo{ID: bson.NewObjectID(), KscopeUserId: "user1", Path: folder, Title: "Kept", Image: []ImageInfo{imgA},
		Sources: []SourceInfo{{Name: "a", SourceID: "1", AttributedTo: []bson.ObjectID{imgA.ID}}}}
	absorbed := &ImageSetMongo{ID: bson.NewObjectID(), KscopeUserId: "user1", Path: folder, Image: []ImageInfo{imgC},
		Sources: []SourceInfo{{Name: "b", SourceID: "2", AttributedTo: []bson.ObjectID{imgC.ID}}}}
	if err := os.WriteFile(filepath.Join(folder, imgC.Name), []byte("pixels"), 0600); err != nil {
		t.Fatal(err)
	}

	plan, err := makeAutoMergePlan(survivor, absorbed)
	if err != nil {
		t.Fatal(err)
	}
	copyName := ImageFileName("Kept", plan.idMap[imgC.ID], "png")

	checked := false
	Tagger = fakeTagger{processErr: errors.New("stop before the database"), onProcess: func() {

		checked = true
		original, originalErr := os.Stat(filepath.Join(folder, imgC.Name))
		linked, linkedErr := os.Stat(filepath.Join(folder, copyName))
		if originalErr != nil || linkedErr != nil || !os.SameFile(original, linked) {
			t.Errorf("while linked: absorbed file %v, copy %v; want the copy to be a hard link of the absorbed file", originalErr, linkedErr)
		}

		for _, img := range survivor.Image {
			if img.Name == imgC.Name {
				t.Errorf("the survivor lists the absorbed file %s", img.Name)
			}
		}
	}}

	if _, _, err := applyMergePlan("user1", plan); err == nil || !checked {
		t.Errorf("same folder: err %v, checked %t; want the merge to reach ProcessSourceTags", err, checked)
	}

	if entries, _ := os.ReadDir(folder); len(entries) != 1 || entries[0].Name() != imgC.Name {
		t.Errorf("after the failure: %v; want only the absorbed file left", entries)
	}

	if got, _ := os.ReadFile(filepath.Join(folder, imgC.Name)); string(got) != "pixels" {
		t.Errorf("the absorbed file became %q", got)
	}

	//a copy is linked under its new name even within one folder, so a file already at that name stops the merge
	folder = t.TempDir() + "/"
	imgA, imgC = storedImage(1, true), storedImage(3, true)
	survivor = &ImageSetMongo{ID: bson.NewObjectID(), KscopeUserId: "user1", Path: folder, Title: "Kept", Image: []ImageInfo{imgA},
		Sources: []SourceInfo{{Name: "a", SourceID: "1", AttributedTo: []bson.ObjectID{imgA.ID}}}}
	absorbed = &ImageSetMongo{ID: bson.NewObjectID(), KscopeUserId: "user1", Path: folder, Image: []ImageInfo{imgC},
		Sources: []SourceInfo{{Name: "b", SourceID: "2", AttributedTo: []bson.ObjectID{imgC.ID}}}}
	if plan, err = makeAutoMergePlan(survivor, absorbed); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{imgC.Name, ImageFileName("Kept", plan.idMap[imgC.ID], "png")} {
		if err := os.WriteFile(filepath.Join(folder, name), []byte("pixels"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	processed = false
	Tagger = fakeTagger{processed: &processed, processErr: errors.New("stop before the database")}
	if _, _, err := applyMergePlan("user1", plan); err == nil || processed {
		t.Errorf("taken name: err %v, tags processed %t; want the link to its taken name to fail first", err, processed)
	}
}
