package imageset

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestImageFileNameStaysWithinLimit(t *testing.T) {
	id := bson.NewObjectID()
	for _, unit := range []string{"a", "é", "漢", "😀"} {
		for n := 0; n <= 400; n++ {
			for _, prefix := range []string{"", "thumbnail_", "low_"} {
				title := prefix + strings.Repeat(unit, n)
				name := ImageFileName(title, id, 1234, "png")

				if len(name) > maxFileNameBytes {
					t.Fatalf("title of %d x %q: name is %d bytes, want <= %d", n, unit, len(name), maxFileNameBytes)
				}
				if !utf8.ValidString(name) {
					t.Fatalf("title of %d x %q: name is not valid UTF-8", n, unit)
				}
				if !strings.HasPrefix(name, id.Hex()+"_") || !strings.HasSuffix(name, "_1234.png") {
					t.Fatalf("title of %d x %q: name %q lost its id or index", n, unit, name)
				}
			}
		}
	}
}

func TestImageFileNameKeepsShortTitle(t *testing.T) {
	id := bson.NewObjectID()
	got := ImageFileName("My Title", id, 0, "gif")
	want := id.Hex() + "_My Title_0.gif"
	if got != want {
		t.Errorf("ImageFileName = %q, want %q", got, want)
	}
}

func TestWriteFileAtomicUsesVolumeTempDir(t *testing.T) {
	old := BackendVolumeLocation
	BackendVolumeLocation = t.TempDir()
	defer func() { BackendVolumeLocation = old }()

	leftover := filepath.Join(tempDir(), "kscope-crashed.tmp")
	if err := os.MkdirAll(tempDir(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(leftover, []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ResetTempDir(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(leftover); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("leftover temp file survived ResetTempDir: %v", err)
	}

	dest := filepath.Join(BackendVolumeLocation, "user", "author")
	if err := os.MkdirAll(dest, 0700); err != nil {
		t.Fatal(err)
	}
	encode := func(w io.Writer) error { _, err := w.Write([]byte("data")); return err }
	if err := writeFileAtomic(dest, "img.png", encode); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dest, "img.png")); err != nil || string(got) != "data" {
		t.Errorf("final file = %q, %v; want data", got, err)
	}
	if entries, _ := os.ReadDir(tempDir()); len(entries) != 0 {
		t.Errorf("temp folder not empty after write: %v", entries)
	}
}

func TestTruncateUTF8(t *testing.T) {
	tests := []struct {
		s    string
		max  int
		want string
	}{
		{"abc", 5, "abc"},
		{"abc", 3, "abc"},
		{"abc", 2, "ab"},
		{"abc", 0, ""},
		{"abc", -1, ""},
		{"a漢b", 3, "a"},
		{"a漢b", 4, "a漢"},
		{"漢漢", 5, "漢"},
		{"", 3, ""},
	}
	for _, tt := range tests {
		if got := truncateUTF8(tt.s, tt.max); got != tt.want {
			t.Errorf("truncateUTF8(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.want)
		}
	}
}
