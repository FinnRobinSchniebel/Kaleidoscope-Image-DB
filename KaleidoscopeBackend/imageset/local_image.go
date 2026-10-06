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
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/v2/bson"
)

var BackendVolumeLocation string

var LowResPathAppend = "low/"

// tempDir holds in-progress file writes. It is on the volume so renaming a
// finished file out of it stays atomic (a rename can't cross filesystems).
func tempDir() string {
	return filepath.Join(BackendVolumeLocation, ".tmp")
}

// ResetTempDir empties tempDir. Call once at startup, before anything writes:
// whatever is left there is a write a crash interrupted.
func ResetTempDir() error {
	if err := os.RemoveAll(tempDir()); err != nil {
		return fmt.Errorf("clearing temp folder: %w", err)
	}
	if err := os.MkdirAll(tempDir(), 0700); err != nil {
		return fmt.Errorf("creating temp folder: %w", err)
	}
	return nil
}

// ErrImageFileMissing is returned when a stored image name has no file on
// disk. A cached low-res or thumbnail can be regenerated; a full-res can't.
var ErrImageFileMissing = errors.New("image file missing")

func CheckImageSize(m MediaSource) error {
	//image is larger then a 500mb
	if m.Size() > 500000000 {
		return fmt.Errorf("the Image is too large")
	}
	return nil

}

// maxFileNameBytes caps a whole file name; ext4 allows 255 bytes.
const maxFileNameBytes = 240

// ImageFileName builds "<id>_<title>_<index>.<ext>", shortening only the
// title so the name never exceeds maxFileNameBytes.
func ImageFileName(imageTitle string, imageId bson.ObjectID, setIndex int, fileEnding string) string {
	prefix := imageId.Hex() + "_"
	suffix := fmt.Sprintf("_%d.%s", setIndex, fileEnding)
	budget := maxFileNameBytes - len(prefix) - len(suffix)
	return prefix + truncateUTF8(cleanInvalidFileSymbols(imageTitle), budget) + suffix
}

// truncateUTF8 cuts s to at most max bytes without splitting a character.
func truncateUTF8(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
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
		log.Printf("failed to open: %s", FullPath)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, fmt.Errorf("%w: %s", ErrImageFileMissing, name)
		}
		return nil, nil, fmt.Errorf("could not open image file")
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

// DeleteLowResFile removes name (a low-res copy or the thumbnail) from path's
// low-res folder. An empty or already-missing file is fine; a name with a
// path component is logged and skipped, never followed.
func DeleteLowResFile(path, name string) error {
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
		fileAuthorName = unknownAuthor
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
		"\x00", "",
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
