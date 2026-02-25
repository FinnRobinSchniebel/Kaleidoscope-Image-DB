package imageset

import (
	"context"
	"fmt"
	"image"
	"image/draw"
	"log"
	"math"
	"os"

	"github.com/nfnt/resize"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// thumbnails are considered a lowres Path for purposes of folder navigation
func SaveThumbNailLocal(path string, name string, img image.Image, ImageSetID bson.ObjectID, generatedFromIndex int) {

	lowresFullPath := path + LowResPathAppend

	if path == "" || name == "" {
		log.Println("No file name or path given to save thumbnail file with")
		return
	}

	err := os.MkdirAll(lowresFullPath, 0700)
	if err != nil {
		log.Println("Add Lowres failed to make dir: " + err.Error())
		return
	}

	filename, _, err := SaveImage(img, lowresFullPath, name, ImageSetID, generatedFromIndex, "png")
	if err != nil {
		log.Println("Add Lowres: could not save image: " + err.Error())
		return
	}

	filter := bson.M{"_id": ImageSetID}

	update := bson.M{
		"$set": bson.M{
			"thumbnail": filename,
		},
	}
	result, err := Collection.UpdateOne(context.Background(), filter, update)
	if err != nil || result.ModifiedCount == 0 {
		if err != nil {
			log.Println("Mango Error: " + err.Error())
		}
		err = os.Remove(fmt.Sprintf("%s%s", lowresFullPath, name))
		if err != nil {
			log.Println("Add Lowres: could not make changes to db...\n COULD NOT remove image from disk: " + err.Error())
		} else {
			log.Println("Add Lowres: could not make changes to db...\n removed image from disk")
		}
		return
	}

}

//full res can be over 1080p
//lowres will convert to 1080p
//thumbnail will be a 256p image for quick display

/*
accepts a path to the image on the local file path and two size perameters (x,y) if only one is given it will assume it is x and will scale y relatively. If the first value is 0 then x will be scaled relatively to y.
output: image pointer, file type, new Scale, error
*/
func GenerateLowResFromHigh(path string, imageName string, sizeX int, sizeY int) (newImage image.Image, fileType string, XScale float64, err error) {

	imageLink := fmt.Sprintf("%s%s", path, imageName)

	source := DiskSource{imageLink}

	err = CheckImageSize(source)
	//image is larger then a 500mb
	if err != nil {
		return nil, "", 0, err
	}

	//open file
	openFullresImage, err := source.Open()
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
	if sizeX > imageInfo.Width || sizeY > imageInfo.Height {
		return decodeImage, fType, 1.0, nil
	}

	downSizedImage := ResizeAndCropCenter(decodeImage, sizeX, sizeY)

	scaleX := float64(downSizedImage.Bounds().Size().X) / float64(imageInfo.Width)
	//scaleY := float64(downSizedImage.Bounds().Size().Y) / float64(imageInfo.Height)

	return downSizedImage, fType, scaleX, nil
}

// This function is a helper to make sure that any stretching is corrected to crop the result centered
func ResizeAndCropCenter(src image.Image, targetW, targetH int) image.Image {

	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	scaleX := float64(targetW) / float64(srcW)
	scaleY := float64(targetH) / float64(srcH)

	scale := math.Max(scaleX, scaleY)

	resizeW := int(math.Ceil(float64(srcW) * scale))
	resizeH := int(math.Ceil(float64(srcH) * scale))

	// resize
	resized := resize.Resize(
		uint(resizeW),
		uint(resizeH),
		src,
		resize.Lanczos3,
	)
	if targetW == 0 || targetH == 0 {
		return resized
	}

	//center crop
	offsetX := (resizeW - targetW) / 2
	offsetY := (resizeH - targetH) / 2

	cropRect := image.Rect(
		offsetX,
		offsetY,
		offsetX+targetW,
		offsetY+targetH,
	)
	cropped := image.NewRGBA(cropRect)
	draw.Draw(
		cropped,
		cropped.Bounds(),
		resized,
		image.Point{X: offsetX, Y: offsetY},
		draw.Src,
	)

	return cropped
}

/*This function accepts a low res version of an image and stores it to the local storage for future retrieval */

func AddLowresToSetAndStorage(path string, name string, img image.Image, imageset ImageSetMongo, index int) {

	if index < 0 || index > len(imageset.Image) {
		log.Println("Add Lowres: index out of bounds")
		return
	}
	lowresFullPath := path + LowResPathAppend

	err := os.MkdirAll(lowresFullPath, 0700)
	if err != nil {
		log.Println("Add Lowres failed to make dir: " + err.Error())
		return
	}

	filename, _, err := SaveImage(img, lowresFullPath, name+"_ThumbNail_", imageset.ID, index, "png")
	if err != nil {
		log.Println("Add Lowres: could not save image: " + err.Error())
		return
	}

	filter := bson.M{"_id": imageset.ID}

	update := bson.M{
		"$set": bson.M{
			fmt.Sprintf("images.%d.low_images", index): filename,
		},
	}
	result, err := Collection.UpdateOne(context.Background(), filter, update)
	if err != nil || result.ModifiedCount == 0 {
		if err != nil {
			log.Println("Mango Error: " + err.Error())
		}
		err = os.Remove(fmt.Sprintf("%s%s", lowresFullPath, name))
		if err != nil {
			log.Println("Add Lowres: could not make changes to db...\n COULD NOT remove image from disk: " + err.Error())
		} else {
			log.Println("Add Lowres: could not make changes to db...\n removed image from disk")
		}
		return
	}

}
