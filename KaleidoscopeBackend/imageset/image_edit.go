package imageset

import (
	"fmt"
	"image"
	"image/color"
	"os"

	"github.com/nfnt/resize"
)

//full res can be over 1080p
//lowres will convert to 1080p
//thumbnail will be a 256p image for quick display

/*
accepts a path to the image on the local file path and two size perameters (x,y) if only one is given it will assume it is x and will scale y relatively. If the first value is 0 then x will be scaled relatively to y.
Warning: This function must be followed by a differ close file
*/
func GenerateLowResFromHigh(imageLink string, sizeX int, sizeY int) (*image.Image, float64, error) {

	//open file and check for sizes
	openFullresImage, err := os.Open(imageLink)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open image from storage.")
	}
	defer openFullresImage.Close()

	//reader := base64.NewDecoder(base64.StdEncoding, strings.NewReader(openFulresImage.))
	imageInfo, _, err := image.DecodeConfig(openFullresImage)

	//image is larger then a 500mb
	if imageInfo.Height*imageInfo.Width > 500000000/ColorModelBitPerPixelAsInt(imageInfo.ColorModel) {
		return nil, 0, fmt.Errorf("The Image is too large.")
	}

	//abs of inputs
	if sizeX < 0 {
		sizeX = -sizeX
	}
	if sizeY < 0 {
		sizeY = -sizeY
	}

	decodeImage, _, err := image.Decode(openFullresImage)
	if err != nil {
		return nil, 0, fmt.Errorf("error decoding existing image")
	}

	//image is already smaller then that
	if sizeX > imageInfo.Width && sizeY > imageInfo.Height || sizeY > imageInfo.Height {
		return &decodeImage, 1.0, nil
	}

	downSizedImage := resize.Resize(uint(sizeX), uint(sizeY), decodeImage, resize.Lanczos3)

	scale := float64(downSizedImage.Bounds().Size().X) / float64(imageInfo.Width)

	return &downSizedImage, scale, nil
}

func ColorModelBitPerPixelAsInt(model color.Model) int {
	switch model {
	case color.GrayModel, color.AlphaModel:
		return 8
	case color.Gray16Model, color.Alpha16Model:
		return 16
	case color.RGBAModel, color.NRGBAModel:
		return 32
	case color.RGBA64Model, color.NRGBA64Model:
		return 64
	default:
		return 64
	}

}
