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

	images, idMap, err := collapseMergedSetImages(survivor, absorbed, false)
	if err != nil {
		t.Fatal(err)
	}

	if want := []bson.ObjectID{imgA.ID, imgB.ID, imgC.ID}; !slices.Equal(imageIDs(images), want) {
		t.Errorf("images = %v, want [A B C] with C keeping its ID: %v", imageIDs(images), want)
	}

	wantMap := map[bson.ObjectID]bson.ObjectID{copyOfB.ID: imgB.ID, imgC.ID: imgC.ID, copyOfA.ID: imgA.ID, repeatOfC.ID: imgC.ID}
	for from, to := range wantMap {
		if idMap[from] != to {
			t.Errorf("idMap[%v] = %v, want %v", from, idMap[from], to)
		}
	}

	if images[0].IsImageActive || images[2].IsImageActive {
		t.Errorf("A and C active %t/%t; want their stored flags, since the active-if-either rule is the plan's", images[0].IsImageActive, images[2].IsImageActive)
	}

	if !slices.Equal(imageIDs(survivor.Image), []bson.ObjectID{imgA.ID, imgB.ID}) || survivor.Image[0].IsImageActive {
		t.Errorf("the survivor's own images changed: %+v", survivor.Image)
	}

	//fresh IDs: the kept C becomes a copy under a new ID that its repeat follows, with C's own flag
	images, idMap, err = collapseMergedSetImages(survivor, absorbed, true)
	if err != nil {
		t.Fatal(err)
	}

	freshC := idMap[imgC.ID]
	if freshC == imgC.ID || idMap[repeatOfC.ID] != freshC || idMap[copyOfB.ID] != imgB.ID || idMap[copyOfA.ID] != imgA.ID {
		t.Errorf("fresh idMap = %v; want C and its repeat on one new ID, collapsed copies unchanged", idMap)
	}

	if len(images) != 3 || images[2].ID != freshC || images[2].IsImageActive {
		t.Errorf("fresh images %v; want C's copy last under the new ID, still inactive", imageIDs(images))
	}

	//images without a pixel hash are never the same image
	unknownA, unknownB := storedImage(0, true), storedImage(0, true)
	images, idMap, err = collapseMergedSetImages(&ImageSetMongo{Image: []ImageInfo{unknownA}}, &ImageSetMongo{Image: []ImageInfo{unknownB}}, false)
	if err != nil {
		t.Fatal(err)
	}

	if len(images) != 2 || idMap[unknownB.ID] != unknownB.ID {
		t.Errorf("two images without hashes: %d images, the absorbed one kept as %v; want both kept", len(images), idMap[unknownB.ID])
	}
}

func TestMakeAutoMergePlan(t *testing.T) {

	imgA, imgB := storedImage(1, false), storedImage(2, true)
	copyOfA, imgC, repeatOfC, imgD := storedImage(1, true), storedImage(3, false), storedImage(3, true), storedImage(4, false)
	survivor := &ImageSetMongo{ID: bson.NewObjectID(), Description: "one", Image: []ImageInfo{imgA, imgB}}
	absorbed := &ImageSetMongo{ID: bson.NewObjectID(), Description: "two", Image: []ImageInfo{copyOfA, imgC, repeatOfC, imgD}}

	plan, err := makeAutoMergePlan(survivor, absorbed, false)
	if err != nil {
		t.Fatal(err)
	}

	if want := []bson.ObjectID{imgA.ID, imgC.ID, imgD.ID, imgB.ID}; plan.survivor != survivor || plan.absorbed != absorbed || !slices.Equal(plan.order, want) {
		t.Errorf("order = %v, want the automatic [A C D B]: %v", plan.order, want)
	}

	if len(plan.copiedFrom) != 0 {
		t.Errorf("copiedFrom = %v; kept IDs make no copies, not even for a repeat", plan.copiedFrom)
	}

	if len(plan.inactive) != 1 || !plan.inactive[imgD.ID] {
		t.Errorf("inactive = %v; want only D, since A and C have an active copy", plan.inactive)
	}

	if plan.description != "one\n\ntwo" {
		t.Errorf("description = %q, want the two joined", plan.description)
	}

	if survivor.Image[0].IsImageActive || survivor.Description != "one" || absorbed.Image[1].IsImageActive {
		t.Errorf("a set changed: survivor %+v, absorbed %+v", survivor, absorbed)
	}

	fresh, err := makeAutoMergePlan(survivor, absorbed, true)
	if err != nil {
		t.Fatal(err)
	}

	wantCopies := map[bson.ObjectID]bson.ObjectID{fresh.idMap[imgC.ID]: imgC.ID, fresh.idMap[imgD.ID]: imgD.ID}
	if !maps.Equal(fresh.copiedFrom, wantCopies) {
		t.Errorf("copiedFrom = %v, want each fresh ID mapped to the absorbed image it copies: %v", fresh.copiedFrom, wantCopies)
	}

	same, err := makeAutoMergePlan(&ImageSetMongo{Description: "same"}, &ImageSetMongo{Description: "same"}, false)
	if err != nil {
		t.Fatal(err)
	}

	if same.description != "same" {
		t.Errorf("equal descriptions became %q", same.description)
	}
}

func TestOrderedImages(t *testing.T) {

	imgA, held, imgC := storedImage(1, true), storedImage(2, true), storedImage(3, false)
	absorbedCopy := held
	absorbedCopy.Name = "absorbed-copy.png"
	plan := mergePlan{
		survivor: &ImageSetMongo{Image: []ImageInfo{imgA, held}},
		absorbed: &ImageSetMongo{Image: []ImageInfo{absorbedCopy, imgC}},
		order:    []bson.ObjectID{imgC.ID, held.ID, imgA.ID},
		inactive: map[bson.ObjectID]bool{held.ID: true},
	}

	images, err := plan.orderedImages()
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(imageIDs(images), plan.order) || images[1].Name != held.Name {
		t.Errorf("images %v, held named %q; want the plan's order and the survivor's entry %q", imageIDs(images), images[1].Name, held.Name)
	}

	if !images[0].IsImageActive || images[1].IsImageActive || !images[2].IsImageActive {
		t.Errorf("active flags %t/%t/%t; want them from inactive, not the stored flags", images[0].IsImageActive, images[1].IsImageActive, images[2].IsImageActive)
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

func TestDoesMergeNeedNewImageIDs(t *testing.T) {

	duplicate, addition, less := mergeCandidate{scenario: scenarioDuplicate}, mergeCandidate{scenario: scenarioAddition}, mergeCandidate{scenario: scenarioLess}

	if doesMergeNeedNewImageIDs([]mergeCandidate{duplicate, addition, less}) {
		t.Error("one gaining target asked for fresh IDs")
	}

	if !doesMergeNeedNewImageIDs([]mergeCandidate{addition, duplicate, addition}) {
		t.Error("two gaining targets didn't ask for fresh IDs")
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

func TestMergedSetFilesToRemove(t *testing.T) {

	folder, otherFolder := t.TempDir()+"/", t.TempDir()+"/"
	moved, collapsed := storedImage(1, true), storedImage(2, true)
	absorbed := &ImageSetMongo{Path: folder, Image: []ImageInfo{moved, collapsed}}
	sameFolder := &ImageSetMongo{Path: folder, Image: []ImageInfo{moved}}
	elsewhere := &ImageSetMongo{Path: otherFolder, Image: []ImageInfo{collapsed}}

	got, err := mergedSetFilesToRemove(absorbed, []*ImageSetMongo{sameFolder, elsewhere})
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(imageIDs(got), []bson.ObjectID{collapsed.ID}) {
		t.Errorf("files to remove = %v; want all but the image a same-folder survivor now holds", imageIDs(got))
	}

	if got, err := mergedSetFilesToRemove(absorbed, []*ImageSetMongo{elsewhere}); err != nil || !slices.Equal(imageIDs(got), imageIDs(absorbed.Image)) {
		t.Errorf("only another folder's survivor: %v, %v; want every absorbed image", imageIDs(got), err)
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

	plan, err := makeAutoMergePlan(survivor, absorbed, false)
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
	self, err := makeAutoMergePlan(self.survivor, self.survivor, false)
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
}

func TestApplyMergePlanAppliesPlan(t *testing.T) {

	previous := Tagger
	defer func() { Tagger = previous }()

	processed := false
	Tagger = fakeTagger{processed: &processed, processErr: errors.New("stop before the database")}
	plan := twoSetPlan(t, nil)
	survivor, imgA, imgC := plan.survivor, plan.survivor.Image[0], plan.absorbed.Image[0]
	plan.order = []bson.ObjectID{imgA.ID, imgC.ID}
	plan.inactive = map[bson.ObjectID]bool{imgC.ID: true}
	plan.description = "chosen"

	if _, _, err := applyMergePlan("user1", plan); err == nil || !processed {
		t.Fatalf("err %v, tags processed %t; want the merge to reach ProcessSourceTags", err, processed)
	}

	if !slices.Equal(imageIDs(survivor.Image), plan.order) || !survivor.Image[0].IsImageActive || survivor.Image[1].IsImageActive || survivor.Description != "chosen" {
		t.Errorf("survivor %v, active %t/%t, description %q; want the plan's [A C], C inactive, \"chosen\"",
			imageIDs(survivor.Image), survivor.Image[0].IsImageActive, survivor.Image[1].IsImageActive, survivor.Description)
	}

	if survivor.Image[0].LowResName != "a-low.png" || survivor.Image[1].LowResName != "" {
		t.Errorf("low-res names %q/%q; want A's kept and the moved C's cleared", survivor.Image[0].LowResName, survivor.Image[1].LowResName)
	}

	if entries, _ := os.ReadDir(survivor.Path); len(entries) != 0 {
		t.Errorf("the failed merge left %d linked files behind", len(entries))
	}

	//an image both sets already hold, as after an interrupted merge, isn't linked again
	held := storedImage(2, true)
	survivor = &ImageSetMongo{ID: bson.NewObjectID(), KscopeUserId: "user1", Path: t.TempDir() + "/", Image: []ImageInfo{held},
		Sources: []SourceInfo{{Name: "a", SourceID: "1", AttributedTo: []bson.ObjectID{held.ID}}}}
	absorbed := &ImageSetMongo{ID: bson.NewObjectID(), KscopeUserId: "user1", Path: t.TempDir() + "/", Image: []ImageInfo{held},
		Sources: []SourceInfo{{Name: "b", SourceID: "2", AttributedTo: []bson.ObjectID{held.ID}}}}
	for folder, content := range map[string]string{survivor.Path: "kept", absorbed.Path: "copy"} {
		if err := os.WriteFile(filepath.Join(folder, held.Name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}

	plan, err := makeAutoMergePlan(survivor, absorbed, false)
	if err != nil {
		t.Fatal(err)
	}

	processed = false
	if _, _, err := applyMergePlan("user1", plan); err == nil || !processed {
		t.Errorf("already held: err %v, tags processed %t; want it to reach ProcessSourceTags without linking again", err, processed)
	}

	if got, _ := os.ReadFile(filepath.Join(survivor.Path, held.Name)); string(got) != "kept" {
		t.Errorf("the survivor's own file became %q", got)
	}

	//a copy is linked under its new name even within one folder, so a file already at that name stops the merge
	folder := t.TempDir() + "/"
	imgA, imgC = storedImage(1, true), storedImage(3, true)
	survivor = &ImageSetMongo{ID: bson.NewObjectID(), KscopeUserId: "user1", Path: folder, Title: "Kept", Image: []ImageInfo{imgA},
		Sources: []SourceInfo{{Name: "a", SourceID: "1", AttributedTo: []bson.ObjectID{imgA.ID}}}}
	absorbed = &ImageSetMongo{ID: bson.NewObjectID(), KscopeUserId: "user1", Path: folder, Image: []ImageInfo{imgC},
		Sources: []SourceInfo{{Name: "b", SourceID: "2", AttributedTo: []bson.ObjectID{imgC.ID}}}}
	if plan, err = makeAutoMergePlan(survivor, absorbed, true); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{imgC.Name, ImageFileName("Kept", plan.idMap[imgC.ID], "png")} {
		if err := os.WriteFile(filepath.Join(folder, name), []byte("pixels"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	processed = false
	if _, _, err := applyMergePlan("user1", plan); err == nil || processed {
		t.Errorf("same-folder copy: err %v, tags processed %t; want the link to its taken name to fail first", err, processed)
	}
}
