package imageset

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"math/bits"
	"sync"

	"github.com/ajdnik/imghash/v2"
	"github.com/ajdnik/imghash/v2/hashtype"
)

// ImageHashes identifies an image's content: Pixel says "same image" (equal
// decoded pixels), PDQ says "same picture" (a resize, re-encode or small crop).
type ImageHashes struct {
	Pixel []byte `json:"pixel_hash" bson:"pixel_hash" form:"pixel_hash"` //SHA-256 of the size and the 8-bit NRGBA pixels
	PDQ   []byte `json:"pdq_hash" bson:"pdq_hash" form:"pdq_hash"`       //256-bit PDQ of the picture flattened onto white
}

// SimilarDistance is the largest PDQ distance (of 256 bits) that still counts
// as the same picture, about 88%. Re-encodes, resizes and previews measured
// 0-4, 1% crops up to 22, and different works 72 or more.
const SimilarDistance = 31

var pdqHasher = sync.OnceValues(func() (imghash.PDQ, error) { return imghash.NewPDQ() })

// flattenOnWhite returns img drawn over white, or img itself when it has no
// transparency. The PDQ library would draw transparent pixels as black, so a
// copy with a transparent background could never match its original.
func flattenOnWhite(img image.Image) image.Image {
	if opaque, ok := img.(interface{ Opaque() bool }); ok && opaque.Opaque() {
		return img
	}
	bounds := img.Bounds()
	flattened := image.NewRGBA(bounds)
	draw.Draw(flattened, bounds, image.White, image.Point{}, draw.Src)
	draw.Draw(flattened, bounds, img, bounds.Min, draw.Over)
	return flattened
}

// pdqHash returns the 32-byte PDQ hash of img flattened onto white.
func pdqHash(img image.Image) ([]byte, error) {
	hasher, err := pdqHasher()
	if err != nil {
		return nil, fmt.Errorf("creating PDQ hasher: %w", err)
	}
	calculated, err := hasher.Calculate(flattenOnWhite(img))
	if err != nil {
		return nil, fmt.Errorf("calculating PDQ hash: %w", err)
	}
	binaryHash, ok := calculated.(hashtype.Binary)
	if !ok {
		return nil, fmt.Errorf("PDQ hash has unexpected type %T", calculated)
	}
	return []byte(binaryHash), nil
}

// writePixels writes img's width, height and pixels as 8-bit NRGBA to hasher,
// so the same pixels hash the same whatever format they were decoded from.
// note: 16-bit NRGBA keeps each value's high byte, since color.NRGBAModel
// premultiplies first and would round off near-transparent pixels.
func writePixels(hasher hash.Hash, img image.Image) {
	bounds := img.Bounds()
	var size [8]byte
	binary.BigEndian.PutUint32(size[:4], uint32(bounds.Dx()))
	binary.BigEndian.PutUint32(size[4:], uint32(bounds.Dy()))
	hasher.Write(size[:])

	row := make([]byte, 4*bounds.Dx())
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		switch typed := img.(type) {
		case *image.NRGBA:
			start := typed.PixOffset(bounds.Min.X, y)
			copy(row, typed.Pix[start:start+len(row)])
		case *image.NRGBA64:
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				pixel := typed.NRGBA64At(x, y)
				offset := 4 * (x - bounds.Min.X)
				row[offset], row[offset+1], row[offset+2], row[offset+3] = uint8(pixel.R>>8), uint8(pixel.G>>8), uint8(pixel.B>>8), uint8(pixel.A>>8)
			}
		default:
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				pixel := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
				offset := 4 * (x - bounds.Min.X)
				row[offset], row[offset+1], row[offset+2], row[offset+3] = pixel.R, pixel.G, pixel.B, pixel.A
			}
		}
		hasher.Write(row)
	}
}

func HashImage(img image.Image) (ImageHashes, error) {
	pdq, err := pdqHash(img)
	if err != nil {
		return ImageHashes{}, err
	}
	hasher := sha256.New()
	writePixels(hasher, img)
	return ImageHashes{Pixel: hasher.Sum(nil), PDQ: pdq}, nil
}

// HashGif returns decodedGif's pixel hash over every frame and where it sits,
// and the PDQ hash of its first frame.
func HashGif(decodedGif *gif.GIF) (ImageHashes, error) {
	if len(decodedGif.Image) == 0 {
		return ImageHashes{}, fmt.Errorf("gif has no frames")
	}
	pdq, err := pdqHash(decodedGif.Image[0])
	if err != nil {
		return ImageHashes{}, err
	}
	hasher := sha256.New()
	for _, frame := range decodedGif.Image {
		var position [8]byte
		binary.BigEndian.PutUint32(position[:4], uint32(frame.Rect.Min.X))
		binary.BigEndian.PutUint32(position[4:], uint32(frame.Rect.Min.Y))
		hasher.Write(position[:])
		writePixels(hasher, frame)
	}
	return ImageHashes{Pixel: hasher.Sum(nil), PDQ: pdq}, nil
}

// Lengths of the two hashes; anything else is a missing or malformed hash.
const (
	pixelHashBytes = sha256.Size
	pdqHashBytes   = 32
)

// PDQDistance returns how many of the 256 bits differ between two PDQ
// hashes. A missing or malformed hash returns 256, so it never matches.
func PDQDistance(first, second []byte) int {
	if len(first) != pdqHashBytes || len(second) != pdqHashBytes {
		return 8 * pdqHashBytes
	}
	distance := 0
	for i := range first {
		distance += bits.OnesCount8(first[i] ^ second[i])
	}
	return distance
}
