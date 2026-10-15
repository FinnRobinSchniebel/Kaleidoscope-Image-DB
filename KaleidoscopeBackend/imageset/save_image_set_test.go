package imageset

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestPrepareImages(t *testing.T) {
	dir := t.TempDir()

	picture := testPicture(0, false)
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, picture); err != nil {
		t.Fatal(err)
	}
	pngPath := filepath.Join(dir, "page.png")
	if err := os.WriteFile(pngPath, pngBytes.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	//frames smaller than the canvas, so the size must come from the canvas
	framePalette := color.Palette{color.Black, color.White}
	animation := &gif.GIF{
		Image:  []*image.Paletted{image.NewPaletted(image.Rect(0, 0, 8, 8), framePalette), image.NewPaletted(image.Rect(0, 0, 8, 8), framePalette)},
		Delay:  []int{10, 10},
		Config: image.Config{ColorModel: framePalette, Width: 10, Height: 12},
	}
	animation.Image[1].SetColorIndex(3, 3, 1)
	var gifBytes bytes.Buffer
	if err := gif.EncodeAll(&gifBytes, animation); err != nil {
		t.Fatal(err)
	}
	gifPath := filepath.Join(dir, "anim.gif")
	if err := os.WriteFile(gifPath, gifBytes.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	prepared, err := prepareImages([]MediaSource{DiskSource{Path: pngPath}, DiskSource{Path: gifPath}})
	if err != nil {
		t.Fatal(err)
	}
	if prepared[0].Info.ID.IsZero() || prepared[0].Info.ID == prepared[1].Info.ID {
		t.Errorf("image IDs %v and %v, want two distinct new IDs", prepared[0].Info.ID, prepared[1].Info.ID)
	}

	decodedPNG, err := png.Decode(bytes.NewReader(pngBytes.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	wantPNG, err := HashImage(decodedPNG)
	if err != nil {
		t.Fatal(err)
	}
	decodedGif, err := gif.DecodeAll(bytes.NewReader(gifBytes.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	wantGif, err := HashGif(decodedGif)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name          string
		got           PreparedImage
		isGif         bool
		width, height int
		hashes        ImageHashes
	}{
		{"png", prepared[0], false, 128, 96, wantPNG},
		{"gif", prepared[1], true, 10, 12, wantGif},
	}
	for _, tt := range tests {
		info := tt.got.Info
		if tt.got.IsGif != tt.isGif || info.Width != tt.width || info.Height != tt.height {
			t.Errorf("%s: IsGif %t, size %dx%d; want %t, %dx%d", tt.name, tt.got.IsGif, info.Width, info.Height, tt.isGif, tt.width, tt.height)
		}
		if !bytes.Equal(info.Pixel, tt.hashes.Pixel) || !bytes.Equal(info.PDQ, tt.hashes.PDQ) {
			t.Errorf("%s: hashes differ from hashing the decoded file directly", tt.name)
		}
	}

	notAnImage := filepath.Join(dir, "broken.png")
	if err := os.WriteFile(notAnImage, []byte("not an image"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareImages([]MediaSource{DiskSource{Path: notAnImage}}); err == nil {
		t.Error("an unreadable file prepared without an error")
	}
}

func TestAddImageSetChecksAttributionFirst(t *testing.T) {
	old := BackendVolumeLocation
	BackendVolumeLocation = t.TempDir()
	defer func() { BackendVolumeLocation = old }()

	//no Tagger, DB or file here, so getting past the check would crash or fail differently
	set := &ImageSetMongo{Sources: []SourceInfo{{Name: "pixiv", SourceID: "1"}}}
	images := []PreparedImage{{Info: ImageInfo{ID: bson.NewObjectID()}}}
	if _, err := addImageSet(set, images, "user1"); !errors.Is(err, errBrokenAttribution) {
		t.Errorf("err = %v, want errBrokenAttribution: the image list must be built and checked first", err)
	}
	if set.Path != "" {
		t.Errorf("Path = %q: the author folder was made before the check", set.Path)
	}
}

func TestProcessNewImageSetRejectsBadInput(t *testing.T) {
	//a readable file, so a missing check would run on into the lookup (no DB here) and crash
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, testPicture(0, false)); err != nil {
		t.Fatal(err)
	}
	pngPath := filepath.Join(t.TempDir(), "page.png")
	if err := os.WriteFile(pngPath, pngBytes.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	files := []MediaSource{DiskSource{Path: pngPath}}

	tests := []struct {
		name    string
		sources []SourceInfo
		files   []MediaSource
	}{
		{"no files", []SourceInfo{{Name: "pixiv"}}, nil},
		{"no source", nil, files},
		{"two sources", []SourceInfo{{Name: "pixiv"}, {Name: "upload"}}, files},
		{"source already attributed", []SourceInfo{{Name: "pixiv", AttributedTo: []bson.ObjectID{bson.NewObjectID()}}}, files},
	}
	for _, tt := range tests {
		set := &ImageSetMongo{Sources: tt.sources}
		if _, err := ProcessNewImageSet(set, tt.files, "user1"); err == nil {
			t.Errorf("%s: accepted", tt.name)
		}
	}

	if _, err := ProcessNewImageSet(&ImageSetMongo{Sources: []SourceInfo{{Name: "pixiv"}}}, nil, "user1"); !errors.Is(err, ErrNoMedia) {
		t.Errorf("no files: err = %v, want ErrNoMedia so callers can answer 400", err)
	}
}
