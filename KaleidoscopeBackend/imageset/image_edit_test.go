package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// pngClaiming returns a small PNG whose header claims width x height, with the
// header's CRC recomputed so it still reads as valid. Only its header is usable.
func pngClaiming(t *testing.T, width, height uint32) []byte {

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}

	//IHDR: length at 8, type at 12, width at 16, height at 20, CRC of type and data at 29
	claimed := encoded.Bytes()
	binary.BigEndian.PutUint32(claimed[16:], width)
	binary.BigEndian.PutUint32(claimed[20:], height)
	binary.BigEndian.PutUint32(claimed[29:], crc32.ChecksumIEEE(claimed[12:29]))
	return claimed
}

// writeTestFile writes content to a new file in a temp folder and returns it as a source.
func writeTestFile(t *testing.T, name string, content []byte) DiskSource {

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	return DiskSource{Path: path}
}

func TestCheckImageHeader(t *testing.T) {

	if format, err := checkImageHeader(writeTestFile(t, "limit.png", pngClaiming(t, 20000, 10000))); err != nil || format != "png" {
		t.Errorf("exactly %d pixels: format %q, err %v; want png and no error", maxImagePixels, format, err)
	}

	if _, err := checkImageHeader(writeTestFile(t, "over.png", pngClaiming(t, 20000, 10001))); !errors.Is(err, notification.ErrImageTooLarge) {
		t.Errorf("one row over the limit: err %v, want ErrImageTooLarge", err)
	}

	if _, err := checkImageHeader(writeTestFile(t, "broken.png", []byte("not an image"))); err == nil || errors.Is(err, notification.ErrImageTooLarge) {
		t.Errorf("not an image: err %v, want a read error", err)
	}

	//a decodable format outside png, jpeg and gif would be stored under its own extension, so it's refused
	image.RegisterFormat("kscopetest", "KSCOPETEST", func(io.Reader) (image.Image, error) { return nil, errors.New("unused") },
		func(io.Reader) (image.Config, error) { return image.Config{Width: 1, Height: 1}, nil })
	if _, err := checkImageHeader(writeTestFile(t, "other.img", []byte("KSCOPETEST"))); err == nil {
		t.Error("a format outside the allowlist was accepted")
	}
}
