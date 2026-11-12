package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// fakeTagger changes no tags, so a merge can run without a database.
type fakeTagger struct {
	processed   *bool             //set when ProcessSourceTags runs, if non-nil
	onProcess   func()            //called when ProcessSourceTags runs, if non-nil
	onRecompute func()            //called when RecomputeSystemTags runs, if non-nil
	processErr  error             //returned by ProcessSourceTags
	failSetID   bson.ObjectID     //if set, processErr is returned for this set only
	undone      *[]TagCountChange //each UndoTagCounts call, if non-nil
}

// ProcessSourceTags returns a change whose SourceName is the set's ID, so an
// undo can be traced to its set.
func (f fakeTagger) ProcessSourceTags(_ string, set *ImageSetMongo, _ int, _ []SourceTag) (TagCountChange, error) {
	if f.processed != nil {
		*f.processed = true
	}

	if f.onProcess != nil {
		f.onProcess()
	}

	change := TagCountChange{SourceName: set.ID.Hex()}
	if !f.failSetID.IsZero() && set.ID != f.failSetID {
		return change, nil
	}
	return change, f.processErr
}
func (fakeTagger) CombineTagOverrides(string, *ImageSetMongo, []string) (TagCountChange, error) {
	return TagCountChange{}, nil
}
func (fakeTagger) RecordDeletion(string, []SourceInfo, []string) error { return nil }

// RecomputeSystemTags returns a change naming the set, like ProcessSourceTags.
func (f fakeTagger) RecomputeSystemTags(_ string, set *ImageSetMongo) (TagCountChange, error) {
	if f.onRecompute != nil {
		f.onRecompute()
	}
	return TagCountChange{SourceName: set.ID.Hex()}, nil
}
func (f fakeTagger) UndoTagCounts(_ string, change TagCountChange) error {
	if f.undone != nil {
		*f.undone = append(*f.undone, change)
	}
	return nil
}
func (fakeTagger) RefreshUntaggedCount(string) error { return nil }
func (fakeTagger) ResolveTagTerm(string, string) ([]string, bool, error) {
	return nil, false, nil
}

func TestAttachIncomingSource(t *testing.T) {

	imageA, imageB, imageC := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	checked := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	older, newer := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC), time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

	//a zip subset onto a synced copy: the checked copy's order wins, the zip's metadata and LastChecked still apply
	target := &ImageSetMongo{Title: "Old", Sources: []SourceInfo{{Name: "pixiv", SourceID: "7", Title: "Old", AttributedTo: []bson.ObjectID{imageA, imageB, imageC}, LastChecked: checked, Date: older}}}
	incoming := &ImageSetMongo{Itype: "illust", Sources: []SourceInfo{{Name: "pixiv", SourceID: "7", Title: "New", Date: newer}}}
	index, isNew := attachIncomingSource(target, incoming, []bson.ObjectID{imageA, imageC})
	source := target.Sources[index]

	if isNew || index != 0 || !slices.Equal(source.AttributedTo, []bson.ObjectID{imageA, imageB, imageC}) {
		t.Errorf("zip subset: index %d, new %t, attributed %v; want 0, false, [A B C]", index, isNew, source.AttributedTo)
	}

	if source.Title != "New" || target.Title != "New" || !source.LastChecked.IsZero() {
		t.Errorf("zip subset: titles %q/%q, LastChecked %v; want New, New and the incoming zero", source.Title, target.Title, source.LastChecked)
	}

	if target.Itype != "illust" {
		t.Errorf("Itype = %q, want the empty one filled from the import", target.Itype)
	}

	//both copies unchecked (a zip onto a zip-only set): the later Date goes first, and a tie to the stored copy
	orders := []struct {
		name   string
		stored SourceInfo
		want   []bson.ObjectID
	}{
		{"zip Date later", SourceInfo{Date: older, LastAppliedUpdate: checked}, []bson.ObjectID{imageB, imageA}},
		{"equal Date", SourceInfo{Date: newer, LastAppliedUpdate: checked}, []bson.ObjectID{imageA, imageB}},
	}

	for _, tc := range orders {

		tc.stored.Name, tc.stored.SourceID, tc.stored.AttributedTo = "zipsrc", "501", []bson.ObjectID{imageA}
		target := &ImageSetMongo{Sources: []SourceInfo{tc.stored}}
		attachIncomingSource(target, &ImageSetMongo{Sources: []SourceInfo{{Name: "zipsrc", SourceID: "501", Date: newer}}}, []bson.ObjectID{imageB, imageA})
		if got := target.Sources[0].AttributedTo; !slices.Equal(got, tc.want) {
			t.Errorf("%s: attributed %v, want %v", tc.name, got, tc.want)
		}
	}

	//a held copy keeps its Date, while the zip's metadata still applies
	target = &ImageSetMongo{Title: "Old", Sources: []SourceInfo{{Name: "pixiv", SourceID: "7", Title: "Old", AttributedTo: []bson.ObjectID{imageA}, LastChecked: checked, Date: older, ImageChangeState: ImageChangePending}}}
	attachIncomingSource(target, &ImageSetMongo{Sources: []SourceInfo{{Name: "pixiv", SourceID: "7", Title: "New", Date: newer}}}, []bson.ObjectID{imageA})
	if src := target.Sources[0]; !src.Date.Equal(older) || src.Title != "New" || src.ImageChangeState != ImageChangePending {
		t.Errorf("held copy %+v: want its Date kept, the zip's title applied and the state kept", src)
	}

	//a new source: appended with its tags cleared and its description joined
	target = &ImageSetMongo{Itype: "manga", Description: "first", Sources: []SourceInfo{{Name: "upload", AttributedTo: []bson.ObjectID{imageA}}}}
	incoming = &ImageSetMongo{Itype: "illust", Sources: []SourceInfo{{Name: "other", SourceID: "601", Description: "second", Tags: []SourceTag{{Default: "cat"}}}}}
	index, isNew = attachIncomingSource(target, incoming, []bson.ObjectID{imageA})
	source = target.Sources[index]

	if !isNew || index != 1 || len(source.Tags) != 0 || !slices.Equal(source.AttributedTo, []bson.ObjectID{imageA}) {
		t.Errorf("new source: index %d, new %t, tags %v, attributed %v; want 1, true, none, [A]", index, isNew, source.Tags, source.AttributedTo)
	}

	if target.Description != "first\n\nsecond" || target.Itype != "manga" {
		t.Errorf("description %q, Itype %q; want the descriptions joined and the set's own Itype kept", target.Description, target.Itype)
	}

	//an ID-less upload is the same source as upload, but not as upload with an ID
	target = &ImageSetMongo{Sources: []SourceInfo{{Name: "upload", AttributedTo: []bson.ObjectID{imageA}}}}
	if _, isNew := attachIncomingSource(target, &ImageSetMongo{Sources: []SourceInfo{{Name: "upload"}}}, []bson.ObjectID{imageA}); isNew {
		t.Error("upload without an ID was added as a new source beside upload without an ID")
	}

	if _, isNew := attachIncomingSource(target, &ImageSetMongo{Sources: []SourceInfo{{Name: "upload", SourceID: "9"}}}, []bson.ObjectID{imageA}); !isNew {
		t.Error("upload with an ID was treated as the ID-less upload source")
	}
}

func TestMergeIntoSetChecksAttributionFirst(t *testing.T) {

	folder := t.TempDir() + "/"
	imageA, missing := bson.NewObjectID(), bson.NewObjectID()
	target := &ImageSetMongo{
		ID:      bson.NewObjectID(),
		Path:    folder,
		Image:   []ImageInfo{{ID: imageA}},
		Sources: []SourceInfo{{Name: "zipsrc", SourceID: "501", AttributedTo: []bson.ObjectID{imageA, missing}}},
	}
	incoming := &ImageSetMongo{Sources: []SourceInfo{{Name: "zipsrc", SourceID: "501"}}}
	prepared := []PreparedImage{preparedWithPixels(1), preparedWithPixels(2)}
	match := mergeCandidate{set: target, scenario: scenarioAddition, exactOf: [][]bson.ObjectID{{imageA}, nil}}

	//no Tagger here, so getting past the check would crash in ProcessSourceTags
	item, _ := mergeIntoSet("user1", "group", incoming, prepared, match)
	if item.Kind != notification.ItemFailed {
		t.Errorf("item = %+v, want a failed item for the broken attribution", item)
	}

	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 0 {
		t.Errorf("%d files were written before the attribution check", len(entries))
	}
}

func TestMergeIntoSetUnchanged(t *testing.T) {

	previous := Tagger
	Tagger = fakeTagger{}
	defer func() { Tagger = previous }()

	imageA, imageB := bson.NewObjectID(), bson.NewObjectID()
	target := &ImageSetMongo{
		ID:      bson.NewObjectID(),
		Title:   "Title",
		Authors: []string{unknownAuthor},
		Image:   []ImageInfo{{ID: imageA}, {ID: imageB}},
		Sources: []SourceInfo{{Name: "zipsrc", SourceID: "501", Title: "Title", AttributedTo: []bson.ObjectID{imageA, imageB}}},
	}
	incoming := &ImageSetMongo{Sources: []SourceInfo{{Name: "zipsrc", SourceID: "501", Title: "Title"}}}
	prepared := []PreparedImage{preparedWithPixels(1), preparedWithPixels(2)}
	match := mergeCandidate{set: target, scenario: scenarioDuplicate, exactOf: [][]bson.ObjectID{{imageA}, {imageB}}}

	//no database here, so attempting the write would crash on the nil Collection
	item, outcome := mergeIntoSet("user1", "group", incoming, prepared, match)
	if item.Kind != notification.ItemUnchanged || outcome.imagesAdded {
		t.Errorf("item = %+v, imagesAdded %t; want unchanged with nothing added", item, outcome.imagesAdded)
	}
}

func TestMergeIntoSetResetsHeldCopiesOnAddition(t *testing.T) {

	previous, volume := Tagger, BackendVolumeLocation
	BackendVolumeLocation = t.TempDir()
	defer func() { Tagger, BackendVolumeLocation = previous, volume }()
	if err := os.MkdirAll(tempDir(), 0700); err != nil {
		t.Fatal(err)
	}

	//ProcessSourceTags always runs, so failing it stops each merge before the database
	Tagger = fakeTagger{processErr: errors.New("stop before the database")}
	newPage := preparedWithPixels(3)
	newPage.Media, newPage.Format = writeTestFile(t, "new.png", []byte("pixels")), "png"

	checked := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	heldDate, zipDate := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC), time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		scenario mergeScenario
		new      bool
		want     []ImageChangeState
	}{
		{"addition", scenarioAddition, true, []ImageChangeState{ImageChangeNone, ImageChangeNone}},
		{"less", scenarioLess, false, []ImageChangeState{ImageChangePending, ImageChangeDeclined}},
	}

	for _, tc := range tests {

		folder := filepath.Join(BackendVolumeLocation, tc.name) + "/"
		if err := os.MkdirAll(folder, 0700); err != nil {
			t.Fatal(err)
		}

		imageA, imageB := bson.NewObjectID(), bson.NewObjectID()
		target := &ImageSetMongo{
			ID:    bson.NewObjectID(),
			Path:  folder,
			Image: []ImageInfo{{ID: imageA}, {ID: imageB}},
			Sources: []SourceInfo{
				{Name: "zipsrc", SourceID: "501", AttributedTo: []bson.ObjectID{imageA, imageB}, LastChecked: checked, Date: heldDate, ImageChangeState: ImageChangePending},
				{Name: "other", SourceID: "601", AttributedTo: []bson.ObjectID{imageA}, ImageChangeState: ImageChangeDeclined},
			},
		}
		incoming := &ImageSetMongo{Sources: []SourceInfo{{Name: "zipsrc", SourceID: "501", Date: zipDate}}}
		prepared := []PreparedImage{preparedWithPixels(1)}
		match := mergeCandidate{set: target, scenario: tc.scenario, exactOf: [][]bson.ObjectID{{imageA}}}
		if tc.new {
			prepared, match.exactOf = append(prepared, preparedWithPixels(2), newPage), [][]bson.ObjectID{{imageA}, {imageB}, nil}
		}

		item, outcome := mergeIntoSet("user1", "group", incoming, prepared, match)
		if item.Kind != notification.ItemFailed {
			t.Fatalf("%s: item %+v, want the failed tag step", tc.name, item)
		}

		//the target is never restored, so it shows what the write would have stored
		got := []ImageChangeState{target.Sources[0].ImageChangeState, target.Sources[1].ImageChangeState}
		if !slices.Equal(got, tc.want) || !target.Sources[0].Date.Equal(heldDate) {
			t.Errorf("%s: states %q, held copy's Date %v; want %q and its Date kept", tc.name, got, target.Sources[0].Date, tc.want)
		}

		//the write never landed, so the copies are still held where they're stored
		if len(outcome.heldCopies) != 0 {
			t.Errorf("%s: the failed write reported held-copy changes %+v", tc.name, outcome.heldCopies)
		}

		if entries, _ := os.ReadDir(folder); len(entries) != 0 {
			t.Errorf("%s: the failed merge left %d files behind", tc.name, len(entries))
		}
	}
}
