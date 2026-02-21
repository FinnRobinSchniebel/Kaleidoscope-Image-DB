package imageset

import (
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"log"
	"os"

	"github.com/ajdnik/imghash"
	"github.com/nfnt/resize"
	"go.mongodb.org/mongo-driver/v2/bson"
)

//full res can be over 1080p
//lowres will convert to 1080p
//thumbnail will be a 256p image for quick display

/*
accepts a path to the image on the local file path and two size perameters (x,y) if only one is given it will assume it is x and will scale y relatively. If the first value is 0 then x will be scaled relatively to y.
output: image pointer, file type, new Scale, error
*/
func GenerateLowResFromHigh(path string, imageName string, sizeX int, sizeY int) (*image.Image, string, float64, error) {

	imageLink := fmt.Sprintf("%s%s", path, imageName)
	//open file and check for sizes
	openFullresImage, err := os.Open(imageLink)
	if err != nil {
		return nil, "", 0, fmt.Errorf("failed to open image from storage")
	}
	defer openFullresImage.Close()

	//reader := base64.NewDecoder(base64.StdEncoding, strings.NewReader(openFulresImage.))
	imageInfo, _, err := image.DecodeConfig(openFullresImage)
	//Important decodeConfig eats the first bytes of the file reader and does not reset to the start
	openFullresImage.Seek(0, 0)

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
Out: fileName, file hash, error

Warning: this function does not save gifs
*/
func SaveImage(imageToSave *image.Image, path string, title string, id bson.ObjectID, index int, fileType string) (string, string, error) {

	/**		Test FilePath	 **/
	_, err := os.Stat(BackendVolumeLocation)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("File or directory does not exist at: %s\n", BackendVolumeLocation)
		} else {
			fmt.Printf("Error accessing path %s: %v\n", BackendVolumeLocation, err)
		}
	} else {
		log.Printf("File or directory exists at: %s\n", BackendVolumeLocation)
	}
	/**		save file 	**/
	fileName := ImageFileName(title, id, index, fileType)
	fullPath := fmt.Sprintf("%s%s", path, fileName)
	log.Print("FilePath: " + fullPath)

	OutputFile, err := os.Create(fullPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to create file: %s", fullPath)
	}

	switch fileType {
	case "png", "PNG":
		err = png.Encode(OutputFile, *imageToSave)
	case "jpeg", "jpg":
		err = jpeg.Encode(OutputFile, *imageToSave, &jpeg.Options{Quality: 100})
	case "gif":
		os.Remove(fullPath)
		log.Println("warning: this function does not create with gifs and will transform it into png")
		err = png.Encode(OutputFile, *imageToSave)
	default:
		os.Remove(fullPath)
		return "", "", fmt.Errorf("file type could not be determined")
	}

	if err != nil {
		os.Remove(fullPath)
		return "", "", fmt.Errorf("could not write the image to the server file")
	}

	/** 	get hash 	**/
	phash := imghash.NewPHash()
	ihash := phash.Calculate(*imageToSave)
	fmt.Printf("Image Saved\n Hashed to: %v\n", ihash)

	return fileName, ihash.String(), nil

}

/*
Used only to save gifs at full size. Cannot be used to save dowscaled images and only accepts decoded gifs
Out: fileName, file hash, error
*/
func SaveGif(imageToSave *gif.GIF, path string, title string, id bson.ObjectID, index int) (string, string, error) {
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
	fullPath := fmt.Sprintf("%s%s", path, fileName)
	log.Print("FilePath: " + fullPath)

	OutputFile, err := os.Create(fullPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to create file")
	}
	//err =// imageToSave.i
	err = gif.EncodeAll(OutputFile, imageToSave)

	if err != nil {
		os.Remove(fullPath)
		return "", "", fmt.Errorf("could not write the image to the server file")
	}

	/** 	get hash 	**/
	phash := imghash.NewPHash()
	if len(imageToSave.Image) == 0 {
		return "", "", fmt.Errorf("empty gif")
	}
	ihash := phash.Calculate(imageToSave.Image[0])
	fmt.Printf("Image Saved\n Hashed to: %v\n", ihash)

	return fileName, ihash.String(), nil
}

func getFileTypeFromHeader(MediaSource MediaSource) (string, error) {
	file, err := MediaSource.Open()
	if err != nil {
		return "", fmt.Errorf("could not open uploaded file: %w", err)
	}
	defer file.Close()

	_, ftype, err := image.DecodeConfig(file)
	//Important decodeConfig eats the first bytes of the file reader and does not reset to the start
	file.Seek(0, 0)

	if err != nil {
		return "", fmt.Errorf("failed to read image info")
	}
	return ftype, nil
}

func FileHeaderToImage(fileHeader MediaSource) (*image.Image, string, error) {
	// Open the uploaded file
	file, err := fileHeader.Open()
	if err != nil {
		return nil, "", fmt.Errorf("could not open uploaded file: %w", err)
	}
	defer file.Close()

	imageInfo, itype, err := image.DecodeConfig(file)
	//Important decodeConfig eats the first bytes of the file reader and does not reset to the start
	file.Seek(0, 0)

	if err != nil {
		return nil, "", fmt.Errorf("failed to read image info")
	}

	fmt.Printf("file:  w: %d, h: %d type: %s \n", imageInfo.Width, imageInfo.Height, itype)

	//image is larger then a 500mb
	if imageInfo.Height*imageInfo.Width > 500000000/ColorModelBitPerPixelAsInt(imageInfo.ColorModel) {
		return nil, "", fmt.Errorf("the Image is too large")
	}

	// Decode to image.Image
	img, format, err := image.Decode(file)
	if err != nil {
		return nil, "", fmt.Errorf("could not decode image: %w", err)
	}

	return &img, format, nil
}

func FileHeaderToGif(fileHeader MediaSource) (*gif.GIF, error) {
	// Open the uploaded file
	file, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("could not open uploaded file: %w", err)
	}
	defer file.Close()

	// Decode to image.Image
	gif, err := gif.DecodeAll(file)
	if err != nil {
		return nil, fmt.Errorf("could not decode image: %w", err)
	}

	return gif, nil
}
