package zipupload

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"image"
	"image/png"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// writeFiles creates each relative path under root: .png paths get a real
// 1x1 image, everything else gets a few text bytes.
func writeFiles(t *testing.T, root string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(full)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(p, ".png") {
			err = png.Encode(f, image.NewRGBA(image.Rect(0, 0, 1, 1)))
		} else {
			_, err = f.WriteString("text")
		}
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}

// newZipRoot returns an extracted-zip root folder named "art".
func newZipRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "art")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestParseKeepsSimilarFoldersApart(t *testing.T) {
	root := newZipRoot(t)
	writeFiles(t, root, "ab/c/1.png", "a/bc/1.png")

	groups, _, err := ValidateAndParseFolder(root, []string{"", "", ""}, "[Order]", 2)
	if err != nil {
		t.Fatal(err)
	}
	keys := slices.Sorted(maps.Keys(groups))
	if want := []string{"art/a/bc", "art/ab/c"}; !slices.Equal(keys, want) {
		t.Errorf("group keys = %v, want %v", keys, want)
	}
}

func TestParseCollapsedFileUsesFolderTemplate(t *testing.T) {
	root := newZipRoot(t)
	writeFiles(t, root, "alice/Sunset/page_1.png", "alice/Moon.png")

	groups, _, err := ValidateAndParseFolder(root, []string{"", "[Author]", "[Title]"}, "page_[Order]", 2)
	if err != nil {
		t.Fatalf("collapsed file rejected: %v", err)
	}
	moon, ok := groups["art/alice/Moon.png"]
	if !ok || len(moon) != 1 {
		t.Fatalf("collapsed file not its own group: %v", groups)
	}
	if moon[0].Values["Title"] != "Moon" || moon[0].Values["Author"] != "alice" {
		t.Errorf("collapsed file values = %v, want Title=Moon Author=alice", moon[0].Values)
	}
	if len(groups["art/alice/Sunset"]) != 1 {
		t.Errorf("folder group missing: %v", groups)
	}
}

func TestParseSkipsFilesAboveGroupingLevel(t *testing.T) {
	root := newZipRoot(t)
	writeFiles(t, root, "readme.txt", "cover.png", "a/b/1.png")

	groups, skipped, err := ValidateAndParseFolder(root, []string{"", "", ""}, "[Order]", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 2 || skipped[0].Reason != "above grouping level" {
		t.Errorf("skipped = %v, want readme.txt and cover.png above the grouping level", skipped)
	}
	if len(groups) != 1 {
		t.Errorf("groups = %v, want only art/a/b", groups)
	}
}

func TestParseRepeatedFieldMustAgree(t *testing.T) {
	root := newZipRoot(t)
	writeFiles(t, root, "Same/Same/1.png", "A/B/1.png")

	groups, _, err := ValidateAndParseFolder(root, []string{"", "[Title]", "[Title]"}, "[Order]", 2)
	if err != nil {
		t.Fatal(err)
	}
	if c := groups["art/Same/Same"][0].Conflict; c != "" {
		t.Errorf("equal values flagged as conflict: %s", c)
	}
	if c := groups["art/A/B"][0].Conflict; !strings.Contains(c, "[Title]") {
		t.Errorf("conflict = %q, want a [Title] conflict", c)
	}

	sets, skipped, _, _ := createImageSetsFromParsedZipData(root, groups)
	if len(sets) != 1 || sets[0].Key != "art/Same/Same" {
		t.Errorf("sets = %v, want only art/Same/Same", sets)
	}
	if len(skipped) != 1 || skipped[0].Ref != "art/A/B" {
		t.Errorf("skipped = %v, want the conflicting group", skipped)
	}
}

func TestBuildImageSetBundle(t *testing.T) {
	root := newZipRoot(t)
	writeFiles(t, root, "g/notes.txt", "g/1.png", "g/data.json", "g/2.png", "g/3.png", "g/more.txt")

	work := func(extra map[string]string) map[string]string {
		values := map[string]string{"Source": "pixiv", "ID": "42"}
		for k, v := range extra {
			values[k] = v
		}
		return values
	}
	entries := []ParsedFolderInfo{
		{Path: "g/notes.txt", FileType: ".txt", Values: work(nil)},
		{Path: "g/1.png", FileType: ".png", Values: work(map[string]string{"Title": "First", "Author": "alice", "Source": " Pixiv ", "Date": "08-27-2026"})},
		{Path: "g/data.json", FileType: ".json", Values: work(nil)},
		{Path: "g/2.png", FileType: ".png", Values: work(nil)},
		{Path: "g/3.png", FileType: ".png", Values: work(map[string]string{"Title": "First", "AuthorId": "9"})},
		{Path: "g/more.txt", FileType: ".txt", Values: map[string]string{}},
	}
	bundle, skipped, errs := buildImageSetBundle(root, "art/g", entries)
	if bundle == nil {
		t.Fatalf("group skipped: %v %v", skipped, errs)
	}
	set := bundle.Iset

	if !slices.Equal(skipped, []notification.ItemResult{{Kind: notification.ItemSkipped, Ref: "g/data.json", Reason: "not an image"}}) {
		t.Errorf("skipped = %v", skipped)
	}
	if len(set.Sources) != 1 {
		t.Fatalf("Sources = %+v, want one", set.Sources)
	}
	src := set.Sources[0]
	if src.Name != "pixiv" || src.SourceID != "42" || !slices.Equal(src.AttributedTo, []int{0, 1, 2}) {
		t.Errorf("source = %s/%s %v, want pixiv/42 [0 1 2]", src.Name, src.SourceID, src.AttributedTo)
	}
	if src.Title != "First" || src.SourceAuthor != "alice" || src.AuthorID != "9" {
		t.Errorf("source fields = %q/%q/%q, want files to fill each other's empty fields", src.Title, src.SourceAuthor, src.AuthorID)
	}
	if want := imageset.DayOnlyDate(time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)); !src.Date.Equal(want) {
		t.Errorf("source Date = %v, want the day-only %v", src.Date, want)
	}
	if src.Description != "text\n\ntext" || set.Description != src.Description {
		t.Errorf("descriptions = source %q, set %q; want both .txt files on the one source", src.Description, set.Description)
	}
	if set.Title != "First" || !slices.Equal(set.Authors, []string{"alice"}) {
		t.Errorf("set Title = %q, Authors = %v", set.Title, set.Authors)
	}
	if !src.LastChecked.IsZero() {
		t.Error("zip sources must not get a LastChecked")
	}
	if !slices.Equal(bundle.FilePath, []string{"g/1.png", "g/2.png", "g/3.png"}) {
		t.Errorf("FilePath = %v", bundle.FilePath)
	}
}

func TestBuildImageSetBundleSkipsGroupWithOneWorkPerSource(t *testing.T) {
	root := newZipRoot(t)
	writeFiles(t, root, "g/1.png", "g/2.png")

	tests := []struct {
		name   string
		second map[string]string
		reason string
	}{
		{"different sources", map[string]string{"Source": "pixiv", "ID": "42"}, "combines different sources: upload and pixiv/42 in g/2.png"},
		{"conflicting titles", map[string]string{"Title": "B"}, `conflicting [Title] "A" vs "B" in g/2.png`},
	}
	for _, tt := range tests {
		entries := []ParsedFolderInfo{
			{Path: "g/1.png", FileType: ".png", Values: map[string]string{"Title": "A"}},
			{Path: "g/2.png", FileType: ".png", Values: tt.second},
		}
		bundle, skipped, _ := buildImageSetBundle(root, "art/g", entries)
		if bundle != nil {
			t.Errorf("%s: group was imported", tt.name)
		}
		if len(skipped) != 1 || skipped[0].Ref != "art/g" || skipped[0].Reason != tt.reason {
			t.Errorf("%s: skipped = %v, want %q", tt.name, skipped, tt.reason)
		}
	}
}

func TestParseTxtTakesFolderFieldsOnly(t *testing.T) {
	root := newZipRoot(t)
	writeFiles(t, root, "pixiv_42/page_1.png", "pixiv_42/description.txt")

	groups, _, err := ValidateAndParseFolder(root, []string{"", "[Source]_[ID]"}, "page_[Order]", 1)
	if err != nil {
		t.Fatalf(".txt name was matched against the file template: %v", err)
	}
	for _, e := range groups["art/pixiv_42"] {
		if e.FileType == ".txt" && (e.Values["Source"] != "pixiv" || e.Values["ID"] != "42") {
			t.Errorf(".txt values = %v, want its folder's Source and ID", e.Values)
		}
	}
}

func TestUnreadableTxtNote(t *testing.T) {
	root := newZipRoot(t)
	writeFiles(t, root, "g/1.png")

	entries := []ParsedFolderInfo{
		{Path: "g/1.png", FileType: ".png", Values: map[string]string{}},
		{Path: "g/notes.txt", FileType: ".txt", Values: map[string]string{}},
	}
	bundle, _, errs := buildImageSetBundle(root, "art/g", entries)
	if bundle == nil {
		t.Fatal("group with a missing .txt was skipped")
	}
	if !slices.Equal(errs, []string{"couldn't read g/notes.txt"}) {
		t.Errorf("notes = %q, want only the file's path inside the zip", errs)
	}
}

func TestBuildImageSetBundleSkipsGroupWithUnreadableImage(t *testing.T) {
	root := newZipRoot(t)
	writeFiles(t, root, "g/1.png")
	if err := os.WriteFile(filepath.Join(root, "g", "2.png"), []byte("not a png"), 0600); err != nil {
		t.Fatal(err)
	}

	entries := []ParsedFolderInfo{
		{Path: "g/1.png", FileType: ".png", Values: map[string]string{}},
		{Path: "g/2.png", FileType: ".png", Values: map[string]string{}},
	}
	bundle, skipped, _ := buildImageSetBundle(root, "art/g", entries)
	if bundle != nil {
		t.Error("group with an unreadable image was imported")
	}
	if len(skipped) != 1 || skipped[0].Ref != "art/g" || skipped[0].Reason != "unreadable image g/2.png" {
		t.Errorf("skipped = %v", skipped)
	}
}
