package imageset

import (
	"errors"
	"fmt"
	"image"
	"image/gif"
	"io/fs"
	"log"
	"os"
	"path/filepath"
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

	if name != filepath.Base(name) {
		return nil, nil, fmt.Errorf("invalid file name")
	}

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

// DeleteFilesFromInfoList removes each entry's full-res and low-res file.
// A file that is already gone counts as removed, so a retry can finish.
func DeleteFilesFromInfoList(path string, info []ImageInfo) error {
	var errList error
	for _, entry := range info {
		if entry.Name == "" {
			continue
		}

		err := os.Remove(path + entry.Name)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			fmt.Printf("Failed to Find File: %s\n", entry.Name)
			errList = errors.Join(errList, err)
		}
	}
	for _, entry := range info {
		if entry.LowResName == "" {
			continue
		}

		err := os.Remove(path + LowResPathAppend + entry.LowResName)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			fmt.Printf("Failed to Find File: %s\n", entry.LowResName)
			errList = errors.Join(errList, err)
		}
	}
	return errList
}

// DeleteThumbnailFile removes a set's thumbnail from path's low-res folder.
// An empty or already-missing file is fine; a name with a path component is
// logged and skipped, never followed.
func DeleteThumbnailFile(path, name string) error {
	if name == "" {
		return nil
	}
	if name != filepath.Base(name) {
		log.Printf("------ Warning: not deleting thumbnail with a path component: %q ------", name)
		return nil
	}
	err := os.Remove(path + LowResPathAppend + name)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
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

// creates the directory for a user to store image by an author in.
// Sets folder directory to be accessible by current user only
func MakeFileDirectoryFromAuthor(userId string, FirstAuthorName string) (string, error) {

	userId = cleanInvalidFileSymbols(userId)
	FirstAuthorName = cleanInvalidFileSymbols(FirstAuthorName)
	var fileAuthorName string
	if len(FirstAuthorName) > 0 {
		fileAuthorName = FirstAuthorName

	} else {
		fileAuthorName = "unknown"
	}

	filePath := BackendVolumeLocation + "/" + userId + "/" + fileAuthorName + "/"

	//create folder
	err := os.MkdirAll(filePath, 0700)
	if err != nil {
		return "", err
	}

	fmt.Println("New Folder Created")

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
		"/", "",
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
