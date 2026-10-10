package zipupload

import (
	"archive/zip"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUnzipWithoutFolderEntries(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "art.zip")
	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	// Only a file entry, with no permission bits and no entries for its folders.
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "alice/work/1.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("text")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()

	root, err := Unzip(src, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatalf("Unzip: %v", err)
	}
	file := filepath.Join(root, "alice", "work", "1.txt")
	if data, err := os.ReadFile(file); err != nil || string(data) != "text" {
		t.Fatalf("read back %q, %v; want the entry's text", data, err)
	}

	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits aren't reported on Windows")
	}
	if info, err := os.Stat(filepath.Dir(file)); err != nil || info.Mode().Perm() != 0700 {
		t.Errorf("folder mode = %v (%v), want 0700", info.Mode().Perm(), err)
	}
	if info, err := os.Stat(file); err != nil || info.Mode().Perm() != 0600 {
		t.Errorf("file mode = %v (%v), want 0600", info.Mode().Perm(), err)
	}
}
