package imageset

import (
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/ajdnik/imghash"
	"go.mongodb.org/mongo-driver/v2/bson"
)

/*
Used to save images to the  correct location on the file system. It does not modify the imageset
Out: fileName, file hash, error

Warning: this function does not save gifs
*/
func SaveImage(imageToSave image.Image, path string, title string, id bson.ObjectID, index int, fileType string) (string, string, error) {

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
	log.Print("FilePath: " + path + fileName)

	var encode func(io.Writer) error
	switch fileType {
	case "png", "PNG":
		encode = func(w io.Writer) error { return png.Encode(w, imageToSave) }
	case "jpeg", "jpg":
		encode = func(w io.Writer) error { return jpeg.Encode(w, imageToSave, &jpeg.Options{Quality: 100}) }
	case "gif":
		// TODO: gif writing belongs here once supported; use SaveGif until then
		return "", "", fmt.Errorf("SaveImage does not write gifs yet")
	default:
		return "", "", fmt.Errorf("file type could not be determined")
	}

	if err := writeFileAtomic(path, fileName, encode); err != nil {
		return "", "", fmt.Errorf("could not write the image to the server file: %w", err)
	}

	/** 	get hash 	**/
	ihash := HashImage(imageToSave)
	fmt.Printf("Image Saved\n Hashed to: %v\n", ihash)

	return fileName, ihash, nil

}

// writeFileAtomic writes to a file in tempDir, then renames it to dir/name,
// so readers never see a partial file.
func writeFileAtomic(dir, name string, encode func(io.Writer) error) error {
	tmp, err := os.CreateTemp(tempDir(), "kscope-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename

	if err := encode(tmp); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, name))
}

// HashImage computes the same perceptual hash stored in ImageInfo.ImageHash,
// so a freshly decoded image can be compared against what was saved.
func HashImage(img image.Image) string {
	phash := imghash.NewPHash()
	return phash.Calculate(img).String()
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

	if len(imageToSave.Image) == 0 {
		return "", "", fmt.Errorf("empty gif")
	}

	fileName := ImageFileName(title, id, index, "gif")
	log.Print("FilePath: " + path + fileName)

	encode := func(w io.Writer) error { return gif.EncodeAll(w, imageToSave) }
	if err := writeFileAtomic(path, fileName, encode); err != nil {
		return "", "", fmt.Errorf("could not write the image to the server file: %w", err)
	}

	/** 	get hash 	**/
	ihash := HashImage(imageToSave.Image[0])
	fmt.Printf("Image Saved\n Hashed to: %v\n", ihash)

	return fileName, ihash, nil
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

func FileHeaderToImage(fileHeader MediaSource) (image.Image, string, error) {
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

	err = CheckImageSize(fileHeader)
	//image is larger then a 500mb
	if err != nil {
		return nil, "", err
	}

	// Decode to image.Image
	img, format, err := image.Decode(file)
	if err != nil {
		return nil, "", fmt.Errorf("could not decode image: %w", err)
	}

	return img, format, nil
}

func FileHeaderToGif(fileHeader MediaSource) (*gif.GIF, error) {

	err := CheckImageSize(fileHeader)
	//image is larger then a 500mb
	if err != nil {
		return nil, err
	}

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
