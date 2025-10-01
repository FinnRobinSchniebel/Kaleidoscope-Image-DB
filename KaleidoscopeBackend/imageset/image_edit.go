package imageset

import (
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"log"
	"os"

	"github.com/nfnt/resize"
	"go.mongodb.org/mongo-driver/v2/bson"
)

//full res can be over 1080p
//lowres will convert to 1080p
//thumbnail will be a 256p image for quick display

/*
accepts a path to the image on the local file path and two size perameters (x,y) if only one is given it will assume it is x and will scale y relatively. If the first value is 0 then x will be scaled relatively to y.
*/
func GenerateLowResFromHigh(imageLink string, sizeX int, sizeY int) (*image.Image, string, float64, error) {

	//open file and check for sizes
	openFullresImage, err := os.Open(imageLink)
	if err != nil {
		return nil, "", 0, fmt.Errorf("failed to open image from storage")
	}
	defer openFullresImage.Close()

	//reader := base64.NewDecoder(base64.StdEncoding, strings.NewReader(openFulresImage.))
	imageInfo, _, err := image.DecodeConfig(openFullresImage)
	if err != nil {
		return nil, "", 0, fmt.Errorf("failed to read image info")
	}

	//image is larger then a 500mb
	if imageInfo.Height*imageInfo.Width > 500000000/ColorModelBitPerPixelAsInt(imageInfo.ColorModel) {
		return nil, "", 0, fmt.Errorf("the Image is too large")
	}

	//abs of inputs
	if sizeX < 0 {
		sizeX = -sizeX
	}
	if sizeY < 0 {
		sizeY = -sizeY
	}

	decodeImage, fType, err := image.Decode(openFullresImage)
	if err != nil {
		return nil, "", 0, fmt.Errorf("error decoding existing image")
	}

	//image is already smaller then that
	if sizeX > imageInfo.Width && sizeY > imageInfo.Height || sizeY > imageInfo.Height {
		return &decodeImage, fType, 1.0, nil
	}

	downSizedImage := resize.Resize(uint(sizeX), uint(sizeY), decodeImage, resize.Lanczos3)

	scale := float64(downSizedImage.Bounds().Size().X) / float64(imageInfo.Width)

	return &downSizedImage, fType, scale, nil
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

/*
	Used to save images to the  correct location on the file system. It does not modify the imageset

Warning: this function does not save gifs
*/
func SaveImage(imageToSave *image.Image, path string, title string, id bson.ObjectID, index int, fileType string) error {

	/**		Test FilePath	 **/
	_, err := os.Stat(BackendVolumeLocation)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("File or directory does not exist at: %s\n", BackendVolumeLocation)
		} else {
			fmt.Printf("Error accessing path %s: %v\n", BackendVolumeLocation, err)
		}
	} else {
		fmt.Printf("File or directory exists at: %s\n", BackendVolumeLocation)
	}
	/**		save file 	**/
	fileName := ImageFileName(title, id, index, fileType)
	fullPath := fmt.Sprintf("%s%s.%s", path, fileName, fileType)

	OutputFile, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create file")
	}

	switch fileType {
	case "png":
		err = png.Encode(OutputFile, *imageToSave)
	case "jpeg":
		err = jpeg.Encode(OutputFile, *imageToSave, &jpeg.Options{Quality: 100})
	case "gif":
		os.Remove(fullPath)
		log.Println("warning: this function does not create with gifs and will transform it into png")
		err = png.Encode(OutputFile, *imageToSave)
	default:
		os.Remove(fullPath)
		return fmt.Errorf("file type could not be determined")
	}

	if err != nil {
		os.Remove(fullPath)
		return fmt.Errorf("could not write the image to the server file")
	}
	return nil

}

/*
Used only to save gifs at full size. Cannot be used to save dowscaled images and only accepts decoded gifs
*/
func SaveGif(imageToSave *gif.GIF, path string, title string, id bson.ObjectID, index int) error {
	/**		Test FilePath	 **/
	_, err := os.Stat(BackendVolumeLocation)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("File or directory does not exist at: %s\n", BackendVolumeLocation)
		} else {
			fmt.Printf("Error accessing path %s: %v\n", BackendVolumeLocation, err)
		}
	} else {
		fmt.Printf("File or directory exists at: %s\n", BackendVolumeLocation)
	}

	fileName := ImageFileName(title, id, index, "gif")
	fullPath := fmt.Sprintf("%s%s.%s", path, fileName, "gif")

	OutputFile, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create file")
	}
	//err =// imageToSave.i
	err = gif.EncodeAll(OutputFile, imageToSave)

	if err != nil {
		os.Remove(fullPath)
		return fmt.Errorf("could not write the image to the server file")
	}

	return nil
}
