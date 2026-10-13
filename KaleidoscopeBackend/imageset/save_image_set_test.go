package imageset

import (
	"bytes"
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

	pngID, gifID := bson.NewObjectID(), bson.NewObjectID()
	prepared, err := prepareImages([]IncomingImage{{ID: pngID, Media: DiskSource{Path: pngPath}}, {ID: gifID, Media: DiskSource{Path: gifPath}}})
	if err != nil {
		t.Fatal(err)
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
		id            bson.ObjectID
		isGif         bool
		width, height int
		hashes        ImageHashes
	}{
		{"png", prepared[0], pngID, false, 128, 96, wantPNG},
		{"gif", prepared[1], gifID, true, 10, 12, wantGif},
	}
	for _, tt := range tests {
		info := tt.got.Info
		if info.ID != tt.id || tt.got.IsGif != tt.isGif || info.Width != tt.width || info.Height != tt.height {
			t.Errorf("%s: ID %v, IsGif %t, size %dx%d; want %v, %t, %dx%d", tt.name, info.ID, tt.got.IsGif, info.Width, info.Height, tt.id, tt.isGif, tt.width, tt.height)
		}
		if !bytes.Equal(info.Pixel, tt.hashes.Pixel) || !bytes.Equal(info.PDQ, tt.hashes.PDQ) {
			t.Errorf("%s: hashes differ from hashing the decoded file directly", tt.name)
		}
	}

	notAnImage := filepath.Join(dir, "broken.png")
	if err := os.WriteFile(notAnImage, []byte("not an image"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareImages([]IncomingImage{{ID: bson.NewObjectID(), Media: DiskSource{Path: notAnImage}}}); err == nil {
		t.Error("an unreadable file prepared without an error")
	}
}
