package zipupload

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestImportZipGroupsStopsAfterPanic(t *testing.T) {

	//imageset.Collection is nil in tests, so the library lookup panics after the user's lock is taken
	root := t.TempDir()
	for i, path := range []string{"g1/1.png", "g2/1.png"} {

		picture := image.NewNRGBA(image.Rect(0, 0, 32, 32))
		for y := range 32 {
			for x := range 32 {
				picture.SetNRGBA(x, y, color.NRGBA{uint8(x * 8), uint8(y * 8), uint8(i * 100), 255})
			}
		}

		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0700); err != nil {
			t.Fatal(err)
		}
		file, err := os.Create(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(file, picture); err != nil {
			t.Fatal(err)
		}
		file.Close()
	}

	group := func(key, path string) ImageSetFileBundle {
		return ImageSetFileBundle{Key: key, Iset: imageset.ImageSetMongo{Sources: []imageset.SourceInfo{{Name: "upload"}}}, FilePath: []string{path}}
	}
	report := notification.NewImportReport("test.zip")

	err := importZipGroups(root, []ImageSetFileBundle{group("art/g1", "g1/1.png"), group("art/g2", "g2/1.png")}, "zip-panic-user", report)
	if err == nil {
		t.Error("a panicking import returned no error")
	}

	if len(report.Failed) != 1 || report.Failed[0].Ref != "art/g1" || report.Failed[0].Reason != "server error" {
		t.Errorf("failed = %+v, want the panicking group with reason \"server error\"", report.Failed)
	}

	if len(report.Skipped) != 1 || report.Skipped[0].Ref != "art/g2" {
		t.Errorf("skipped = %+v, want the group after the panic", report.Skipped)
	}

	locked := make(chan struct{})
	go func() {
		imageset.LockUserSets("zip-panic-user")()
		close(locked)
	}()
	select {
	case <-locked:
	case <-time.After(time.Second):
		t.Fatal("the panic left the user's set lock held")
	}
}
