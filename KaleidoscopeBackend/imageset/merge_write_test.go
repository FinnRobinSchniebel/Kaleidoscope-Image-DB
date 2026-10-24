package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"os"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// fakeTagger records nothing and changes no tags, so a merge can run without a database.
type fakeTagger struct {
	processed  *bool  //set when ProcessSourceTags runs, if non-nil
	onProcess  func() //called when ProcessSourceTags runs, if non-nil
	processErr error  //returned by ProcessSourceTags
}

func (f fakeTagger) ProcessSourceTags(string, *ImageSetMongo, int, []SourceTag) (TagCountChange, error) {
	if f.processed != nil {
		*f.processed = true
	}

	if f.onProcess != nil {
		f.onProcess()
	}
	return TagCountChange{}, f.processErr
}
func (fakeTagger) CombineTagOverrides(string, *ImageSetMongo, []string) (TagCountChange, error) {
	return TagCountChange{}, nil
}
func (fakeTagger) RecordDeletion(string, []SourceInfo, []string) error { return nil }
func (fakeTagger) RecomputeSystemTags(string, *ImageSetMongo) (TagCountChange, error) {
	return TagCountChange{}, nil
}
func (fakeTagger) UndoTagCounts(string, TagCountChange) error { return nil }
func (fakeTagger) RefreshUntaggedCount(string) error          { return nil }
func (fakeTagger) ResolveTagTerm(string, string) ([]string, bool, error) {
	return nil, false, nil
}

func TestAttachIncomingSource(t *testing.T) {

	imageA, imageB := bson.NewObjectID(), bson.NewObjectID()
	checked := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

	//the same source: attribution extended in page order, metadata applied, LastChecked copied
	target := &ImageSetMongo{Title: "Old", Sources: []SourceInfo{{Name: "zipsrc", SourceID: "501", Title: "Old", AttributedTo: []bson.ObjectID{imageA}, LastChecked: checked}}}
	incoming := &ImageSetMongo{Itype: "illust", Sources: []SourceInfo{{Name: "zipsrc", SourceID: "501", Title: "New"}}}
	index, isNew := attachIncomingSource(target, incoming, []bson.ObjectID{imageB, imageA})
	source := target.Sources[index]

	if isNew || index != 0 || !slices.Equal(source.AttributedTo, []bson.ObjectID{imageB, imageA}) {
		t.Errorf("same source: index %d, new %t, attributed %v; want 0, false, [B A]", index, isNew, source.AttributedTo)
	}

	if source.Title != "New" || target.Title != "New" || !source.LastChecked.IsZero() {
		t.Errorf("same source: titles %q/%q, LastChecked %v; want New, New and the incoming zero", source.Title, target.Title, source.LastChecked)
	}

	if target.Itype != "illust" {
		t.Errorf("Itype = %q, want the empty one filled from the import", target.Itype)
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
	item, _, _ := mergeIntoSet("user1", "group", incoming, prepared, match)
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
	item, imagesAdded, sourceAdded := mergeIntoSet("user1", "group", incoming, prepared, match)
	if item.Kind != notification.ItemUnchanged || imagesAdded || sourceAdded {
		t.Errorf("item = %+v, imagesAdded %t, sourceAdded %t; want unchanged with nothing added", item, imagesAdded, sourceAdded)
	}
}
