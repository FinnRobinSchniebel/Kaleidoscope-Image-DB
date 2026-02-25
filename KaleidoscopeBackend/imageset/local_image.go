package imageset

import (
	"errors"
	"fmt"
	"image"
	"image/gif"
	"log"
	"os"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

var BackendVolumeLocation string

var LowResPathAppend = "low/"

func CheckImageSize(m MediaSource) error {
	//image is larger then a 500mb
	if m.Size() > 500000000 {
		return fmt.Errorf("the Image is too large")
	}
	return nil

}

func ImageFileName(imageTitle string, imageId bson.ObjectID, setIndex int, fileEnding string) string {

	var fileName string
	imageTitle = cleanInvalidFileSymbols(imageTitle)
	imageIDString := cleanInvalidFileSymbols(imageId.Hex())
	//test if file name is to long
	if nameLen := len(imageTitle + "_" + imageIDString); nameLen > 240 {
		fileName = fmt.Sprintf("%s_%s", imageTitle[0:nameLen-(nameLen-240)], imageIDString)
	} else {
		fileName = fmt.Sprintf("%s_%s", imageIDString, imageTitle)
	}

	// db folder/ first_author / "File Name"?_id_"image set index".format
	fileName = fmt.Sprintf("%s_%d.%s", fileName, setIndex, fileEnding)
	return fileName
}

func RetrieveLocalImage(path string, name string, low bool) (image.Image, *gif.GIF, error) {

	var FullPath string
	if low {
		FullPath = fmt.Sprintf("%s%s%s", path, LowResPathAppend, name)
	} else {
		FullPath = fmt.Sprintf("%s%s", path, name)
	}

	f, err := os.Open(FullPath)
	if err != nil {
		log.Printf("failed to open: %s", fmt.Sprintf("%s%s", path, name))
		return nil, nil, fmt.Errorf("no file found")
	}
	defer f.Close()

	img, format, err := image.Decode(f)
	if err != nil {
		log.Printf("failed to decode: %s", fmt.Sprintf("%s%s", path, name))
		return nil, nil, fmt.Errorf("could not decode image")
	}

	if format == "gif" {
		img = nil
		//important decode may not reset the file reader
		f.Seek(0, 0)
		retgif, err := gif.DecodeAll(f)
		if err != nil {
			log.Printf("failed to decode: %s", fmt.Sprintf("%s%s", path, name))
			return nil, nil, fmt.Errorf("could not decode gif")
		}
		return nil, retgif, nil
	} else {
		return img, nil, nil
	}

}

func DeleteFilesFromInfoList(path string, info []ImageInfo) error {
	var errList error
	for _, entry := range info {
		if entry.Name == "" {
			continue
		}

		err := os.Remove(path + entry.Name)
		if err != nil {
			fmt.Printf("Failed to Find File: %s\n", entry)
			errList = errors.Join(errList, err)
		}
	}
	for _, entry := range info {
		if entry.LowResName == "" {
			continue
		}

		err := os.Remove(path + LowResPathAppend + entry.LowResName)
		if err != nil {
			fmt.Printf("Failed to Find File: %s\n", entry)
			errList = errors.Join(errList, err)
		}
	}
	return errList
}

func CheckImageSetFileDeletionPermissions(entryToDelete ImageSetMongo) error {
	//check if the file paths in the imageset are valid and deletable
	for index, entry := range entryToDelete.Image {
		info, err := os.Stat(entryToDelete.Path + entry.Name)
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("No File Exists for image number: " + strconv.Itoa(index))
		}
		if info.Mode().Perm()&0222 == 0 {
			return errors.New("No permission to delete image number: " + strconv.Itoa(index))
		}
	}
	for index, entry := range entryToDelete.Image {
		if entry.LowResName == "" {
			continue
		}
		info, err := os.Stat(entryToDelete.Path + entry.LowResName)
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("No File Exists for low-res image number: " + strconv.Itoa(index))
		}
		if info.Mode().Perm()&0222 == 0 {
			return errors.New("No permission to delete low-res image number: " + strconv.Itoa(index))
		}
	}
	return nil
}

func MakeFileDirectoryFromAuthor(FirstAuthorName string) (string, error) {

	FirstAuthorName = cleanInvalidFileSymbols(FirstAuthorName)
	var fileAuthorName string
	if len(FirstAuthorName) > 0 {
		fileAuthorName = FirstAuthorName

	} else {
		fileAuthorName = "unknown"
	}

	filePath := BackendVolumeLocation + "/" + fileAuthorName + "/"
	//print state
	fileInfo, _ := os.Stat(BackendVolumeLocation)
	fmt.Println(fileInfo.Mode())

	//create folder
	err := os.MkdirAll(filePath, 0700)
	if err != nil {
		return "", err
	}

	//fileInfo, _ = os.Stat(filePath)
	fmt.Println("Folder Created")

	return filePath, nil
}

func cleanInvalidFileSymbols(name string) string {
	//name = strings.ToValidUTF8(name, "")

	r := strings.NewReplacer(
		"[", "",
		"]", "",
		"!", "",
		".", "",
		"#", "",
		"{", "",
		"}", "",
		"\\", "",
		"<", "",
		">", "",
	)
	return r.Replace(name)
}

func getType(file string) string {

	indexOfTypeStart := strings.Index(file, ".")

	if indexOfTypeStart == -1 {
		return ""
	}
	return file[indexOfTypeStart:]
}
