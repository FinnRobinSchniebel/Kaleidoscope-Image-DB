package zipupload

import (
	"image"
	"image/png"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
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
	if len(skipped) != 2 {
		t.Errorf("skipped = %v, want readme.txt and cover.png", skipped)
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
	if len(skipped) != 1 || !strings.HasPrefix(skipped[0], "art/A/B") {
		t.Errorf("skipped = %v, want the conflicting group", skipped)
	}
}

func TestBuildImageSetBundle(t *testing.T) {
	root := newZipRoot(t)
	writeFiles(t, root, "g/notes.txt", "g/1.png", "g/data.json", "g/2.png", "g/3.png")

	entries := []ParsedFolderInfo{
		{Path: "g/notes.txt", FileType: ".txt"},
		{Path: "g/1.png", FileType: ".png", Values: map[string]string{"Title": "First", "Author": "alice"}},
		{Path: "g/data.json", FileType: ".json", Values: map[string]string{}},
		{Path: "g/2.png", FileType: ".png", Values: map[string]string{"Title": "Second", "Author": "alice", "Source": "pixiv", "ID": "42"}},
		{Path: "g/3.png", FileType: ".png", Values: map[string]string{"Title": "First", "Author": "alice"}},
	}
	bundle, skipped, errs := buildImageSetBundle(root, "art/g", entries)
	if bundle == nil {
		t.Fatalf("group skipped: %v %v", skipped, errs)
	}
	set := bundle.Iset

	if !slices.Equal(skipped, []string{"g/data.json (not an image)"}) {
		t.Errorf("skipped = %v", skipped)
	}
	if set.Title != "First" {
		t.Errorf("Title = %q, want the first source's title", set.Title)
	}
	if !slices.Equal(set.Authors, []string{"alice"}) {
		t.Errorf("Authors = %v, want [alice]", set.Authors)
	}
	if set.Description != "text" {
		t.Errorf("Description = %q, want the .txt contents", set.Description)
	}
	if len(set.Sources) != 2 {
		t.Fatalf("Sources = %+v, want 2", set.Sources)
	}
	first, second := set.Sources[0], set.Sources[1]
	if first.Name != uploadSourceName || !slices.Equal(first.AttributedTo, []int{0, 2}) {
		t.Errorf("first source = %s %v, want %s [0 2]", first.Name, first.AttributedTo, uploadSourceName)
	}
	if second.Name != "pixiv" || second.SourceID != "42" || !slices.Equal(second.AttributedTo, []int{1}) {
		t.Errorf("second source = %s/%s %v, want pixiv/42 [1]", second.Name, second.SourceID, second.AttributedTo)
	}
	if first.Description != "text" || second.Description != "" {
		t.Errorf("descriptions = %q / %q, want the .txt only on the matching upload source", first.Description, second.Description)
	}
	if !first.LastChecked.IsZero() || !second.LastChecked.IsZero() {
		t.Error("zip sources must not get a LastChecked")
	}
	if !slices.Equal(bundle.FilePath, []string{"g/1.png", "g/2.png", "g/3.png"}) {
		t.Errorf("FilePath = %v", bundle.FilePath)
	}
}

func TestBuildImageSetBundleAttachesDescriptionsByNameAndID(t *testing.T) {
	root := newZipRoot(t)
	writeFiles(t, root, "g/1.png", "g/2.png", "g/pixiv.txt", "g/other.txt")

	entries := []ParsedFolderInfo{
		{Path: "g/pixiv.txt", FileType: ".txt", Values: map[string]string{"Source": "pixiv", "ID": "42"}},
		{Path: "g/other.txt", FileType: ".txt", Values: map[string]string{"Source": "pixiv", "ID": "99"}},
		{Path: "g/1.png", FileType: ".png", Values: map[string]string{"Title": "A"}},
		{Path: "g/2.png", FileType: ".png", Values: map[string]string{"Title": "A", "Source": "pixiv", "ID": "42"}},
	}
	bundle, _, errs := buildImageSetBundle(root, "art/g", entries)
	if bundle == nil {
		t.Fatal("group skipped")
	}
	upload, pixiv := bundle.Iset.Sources[0], bundle.Iset.Sources[1]

	if pixiv.Description != "text" {
		t.Errorf("pixiv/42 description = %q, want its .txt", pixiv.Description)
	}
	if upload.Description != "text" {
		t.Errorf("unmatched .txt should fall back to the first source, got %q", upload.Description)
	}
	if len(errs) != 1 || !strings.Contains(errs[0], "g/other.txt") {
		t.Errorf("errors = %v, want the unmatched .txt reported", errs)
	}
	if bundle.Iset.Description != "text\n\ntext" {
		t.Errorf("set Description = %q, want both source descriptions joined", bundle.Iset.Description)
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
	if len(skipped) != 1 || !strings.Contains(skipped[0], "unreadable image g/2.png") {
		t.Errorf("skipped = %v", skipped)
	}
}
