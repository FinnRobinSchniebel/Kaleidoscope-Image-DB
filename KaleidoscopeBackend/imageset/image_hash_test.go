package imageset

import (
	"bytes"
	"crypto/sha256"
	"image"
	"image/color"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

// testPicture draws a picture with some structure for PDQ to see: shapes
// whose layout depends on variant, on white, or on a transparent background
// when transparent is set. The left half of every shape is semi-transparent.
func testPicture(variant int, transparent bool) *image.NRGBA {
	const w, h = 128, 96
	picture := image.NewNRGBA(image.Rect(0, 0, w, h))
	background := color.NRGBA{255, 255, 255, 255}
	if transparent {
		background = color.NRGBA{}
	}
	draw.Draw(picture, picture.Bounds(), image.NewUniform(background), image.Point{}, draw.Src)
	for i := 0; i < 4; i++ {
		x := (i*37 + variant*53) % (w - 30)
		y := (i*23 + variant*29) % (h - 30)
		for py := y; py < y+28; py++ {
			for px := x; px < x+28; px++ {
				alpha := uint8(255)
				if px < x+14 {
					alpha = 160
				}
				picture.SetNRGBA(px, py, color.NRGBA{uint8(40 * i), uint8(200 - 30*i), uint8(90 + variant*40), alpha})
			}
		}
	}
	return picture
}

func pngRoundTrip(t *testing.T, img image.Image) image.Image {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func jpegRoundTrip(t *testing.T, img image.Image) image.Image {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	decoded, err := jpeg.Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

// to16Bit copies img into a 16-bit NRGBA image without changing any value.
func to16Bit(img *image.NRGBA) *image.NRGBA64 {
	out := image.NewNRGBA64(img.Bounds())
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			pixel := img.NRGBAAt(x, y)
			out.SetNRGBA64(x, y, color.NRGBA64{uint16(pixel.R) * 0x101, uint16(pixel.G) * 0x101, uint16(pixel.B) * 0x101, uint16(pixel.A) * 0x101})
		}
	}
	return out
}

func pixelHash(t *testing.T, img image.Image) []byte {
	t.Helper()
	hashes, err := HashImage(img)
	if err != nil {
		t.Fatal(err)
	}
	return hashes.Pixel
}

func TestPixelHashSameAcrossLosslessEncodings(t *testing.T) {
	semiTransparent := testPicture(0, false)

	// every alpha with varied colours: near-transparent pixels (anti-aliased
	// edges) are the ones a premultiplied conversion of 16-bit values rounds off
	alphaRamp := image.NewNRGBA(image.Rect(0, 0, 256, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 256; x++ {
			alphaRamp.SetNRGBA(x, y, color.NRGBA{uint8(x*7 + y*37), uint8(255 - x), uint8(y * 32), uint8(x)})
		}
	}

	opaque := image.NewNRGBA(semiTransparent.Bounds())
	draw.Draw(opaque, opaque.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(opaque, opaque.Bounds(), semiTransparent, image.Point{}, draw.Over)

	paletted := image.NewPaletted(opaque.Bounds(), palette.Plan9)
	draw.Draw(paletted, paletted.Bounds(), opaque, image.Point{}, draw.Src)
	palettedAsRGB := image.NewNRGBA(paletted.Bounds())
	draw.Draw(palettedAsRGB, palettedAsRGB.Bounds(), paletted, image.Point{}, draw.Src)

	gray := image.NewGray(opaque.Bounds())
	draw.Draw(gray, gray.Bounds(), opaque, image.Point{}, draw.Src)
	grayAsRGB := image.NewNRGBA(gray.Bounds())
	draw.Draw(grayAsRGB, grayAsRGB.Bounds(), gray, image.Point{}, draw.Src)

	jpegDecoded := jpegRoundTrip(t, opaque)

	tests := []struct {
		name string
		a, b image.Image
	}{
		{"8-bit vs 16-bit PNG, every alpha", pngRoundTrip(t, alphaRamp), pngRoundTrip(t, to16Bit(alphaRamp))},
		{"paletted vs RGB PNG", pngRoundTrip(t, paletted), pngRoundTrip(t, palettedAsRGB)},
		{"gray vs RGB PNG", pngRoundTrip(t, gray), pngRoundTrip(t, grayAsRGB)},
		// migrated hashes came from the stored PNG; a re-upload hashes the JPEG decode
		{"JPEG decode vs its stored PNG", jpegDecoded, pngRoundTrip(t, jpegDecoded)},
	}
	for _, tt := range tests {
		if !bytes.Equal(pixelHash(t, tt.a), pixelHash(t, tt.b)) {
			t.Errorf("%s: pixel hashes differ", tt.name)
		}
	}
}

func TestPixelHashIncludesSize(t *testing.T) {
	pixels := bytes.Repeat([]byte{10, 20, 30, 255}, 6)
	wide := &image.NRGBA{Pix: pixels, Stride: 3 * 4, Rect: image.Rect(0, 0, 3, 2)}
	tall := &image.NRGBA{Pix: pixels, Stride: 2 * 4, Rect: image.Rect(0, 0, 2, 3)}
	if bytes.Equal(pixelHash(t, wide), pixelHash(t, tall)) {
		t.Error("3x2 and 2x3 images with the same bytes hash the same")
	}
}

func TestHashGifCoversEveryFrame(t *testing.T) {
	frame := func(lit image.Point) *image.Paletted {
		paletted := image.NewPaletted(image.Rect(0, 0, 8, 8), color.Palette{color.Black, color.White})
		paletted.SetColorIndex(lit.X, lit.Y, 1)
		return paletted
	}
	first := frame(image.Pt(2, 2))
	original, err := HashGif(&gif.GIF{Image: []*image.Paletted{first, frame(image.Pt(5, 5))}})
	if err != nil {
		t.Fatal(err)
	}
	secondFrameChanged, err := HashGif(&gif.GIF{Image: []*image.Paletted{first, frame(image.Pt(6, 6))}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original.PDQ, secondFrameChanged.PDQ) {
		t.Error("PDQ differs, but it should only see the first frame")
	}
	if bytes.Equal(original.Pixel, secondFrameChanged.Pixel) {
		t.Error("pixel hash ignores a changed second frame")
	}

	if _, err := HashGif(&gif.GIF{}); err == nil {
		t.Error("a gif with no frames hashed without an error")
	}
}

func TestPDQFlattensTransparency(t *testing.T) {
	onWhite, err := HashImage(testPicture(1, false))
	if err != nil {
		t.Fatal(err)
	}
	transparent, err := HashImage(testPicture(1, true))
	if err != nil {
		t.Fatal(err)
	}
	other, err := HashImage(testPicture(2, false))
	if err != nil {
		t.Fatal(err)
	}

	if d := PDQDistance(onWhite.PDQ, transparent.PDQ); d > 4 {
		t.Errorf("transparent copy is %d from the picture on white, want 4 or less", d)
	}
	if bytes.Equal(onWhite.Pixel, transparent.Pixel) {
		t.Error("pixel hash ignores the transparency")
	}
	if d := PDQDistance(transparent.PDQ, other.PDQ); d <= SimilarDistance {
		t.Errorf("transparent copy is %d from a different picture, want more than %d", d, SimilarDistance)
	}
}

func TestIsPlain(t *testing.T) {

	plainPage := func(c color.Color) image.Image {
		page := image.NewNRGBA(image.Rect(0, 0, 200, 300))
		draw.Draw(page, page.Bounds(), image.NewUniform(c), image.Point{}, draw.Src)
		return page
	}

	tests := []struct {
		name  string
		img   image.Image
		plain bool
	}{
		{"white", plainPage(color.White), true},
		{"black", plainPage(color.Black), true},
		{"mid gray", plainPage(color.Gray{Y: 128}), true},
		{"red", plainPage(color.NRGBA{R: 200, G: 30, B: 30, A: 255}), true},
		{"picture", testPicture(0, false), false},
	}
	for _, tt := range tests {

		hashes, err := HashImage(tt.img)
		if err != nil {
			t.Fatal(err)
		}

		plain, err := isPlain(hashes.PDQ)
		if err != nil {
			t.Fatal(err)
		}

		if plain != tt.plain {
			white, _ := whitePDQ()
			t.Errorf("%s: plain = %t, want %t (distance from white %d)", tt.name, plain, tt.plain, PDQDistance(hashes.PDQ, white))
		}
	}

	if plain, err := isPlain(nil); err != nil || plain {
		t.Errorf("a missing hash: plain = %t, err = %v; want not plain", plain, err)
	}
}

// hiddenType hides an image's concrete type, so writePixels takes its generic branch.
type hiddenType struct{ image.Image }

func TestWritePixelsPalettedMatchesGeneric(t *testing.T) {

	mixed := make(color.Palette, 256)
	for i := range mixed {

		value := uint8(i)
		switch i % 3 {
		case 0:
			mixed[i] = color.RGBA{R: value / 2, G: value / 3, B: value / 4, A: value} //premultiplied and semi-transparent
		case 1:
			mixed[i] = color.NRGBA{R: value, G: 255 - value, B: 7, A: value}
		default:
			mixed[i] = color.Gray{Y: value}
		}
	}

	paletted := image.NewPaletted(image.Rect(3, 5, 40, 21), mixed)
	for i := range paletted.Pix {
		paletted.Pix[i] = uint8(i * 7)
	}

	fast, generic := sha256.New(), sha256.New()
	writePixels(fast, paletted)
	writePixels(generic, hiddenType{paletted})
	if !bytes.Equal(fast.Sum(nil), generic.Sum(nil)) {
		t.Error("the paletted case writes different bytes from the generic conversion")
	}
}
