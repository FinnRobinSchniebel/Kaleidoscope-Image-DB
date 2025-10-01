package imageset

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type SourceInfo struct {
	Name     string   `json:"name" form:"name"`
	ID       string   `json:"id" form:"id"`
	Title    string   `json:"title" form:"title"`
	SourceID string   `json:"sourceid" form:"sourceid"`
	Tags     []string `json:"tags" form:"tags"`
}
type ImageInfo struct {
	Name          string `json:"images" bson:"images" form:"images"`
	LowResName    string `json:"low_images" bson:"low_images" form:"low_images"`
	IsImageActive bool   `json:"active,omitempty" bson:"active,omitempty" form:"active"`
}

type ImageSetMongo struct {
	ID      bson.ObjectID `json:"id,omitempty" bson:"_id,omitempty" form:"id,omitempty"`
	Title   string        `json:"title" bson:"title,omitempty" form:"title"`
	Tags    []string      `json:"tags" bson:"tags,omitempty" form:"tags"`
	Sources []SourceInfo  `json:"sources" bson:"sources,omitempty" form:"sources"`
	Authors []string      `json:"authors" bson:"authors,omitempty" form:"authors"`
	Path    string        `json:"path" bson:"path,omitempty" form:"path"`
	Image   []ImageInfo   `json:"images,omitempty" bson:"images,omitempty" form:"images"`
	//LowImage         []string      `json:"low_images,omitempty" bson:"low_images,omitempty" form:"low_images"`
	//IsImageActive    []bool   `json:"active,omitempty" bson:"active,omitempty" form:"active"`
	ImageHash        []string `json:"hash" bson:"hash,omitempty" form:"hash"`
	AutoTags         []string `json:"autotags" bson:"autotags,omitempty" form:"autotags"`
	TagRuleOverrides []string `json:"tag_rule_overrides" bson:"tag_rule_overrides,omitempty" form:"tag_rule_overrides"`
	Itype            string   `json:"type" bson:"type,omitempty" form:"type"`
	Description      string   `json:"description" bson:"description,omitempty" form:"description"`
	Other            string   `json:"other" bson:"other,omitempty" form:"other"`
	KscopeUserId     string   `json:"kscope_userid" bson:"kscope_userid" form:"kscope_userid"`
	// API will send file as well but it will not be placed in the struct: `json: media`
}

type InternalResponse struct {
	ErrorCode   int
	ErrorString string
}

var BackendVolumeLocation string

func ImageFileName(imageTitle string, imageId bson.ObjectID, setIndex int, fileEnding string) string {

	var fileName string
	imageTitle = cleanInvalidFileSymbols(imageTitle)
	imageIDString := cleanInvalidFileSymbols(imageId.Hex())
	//test if file name is to long
	if nameLen := len(imageTitle + "_" + imageIDString); nameLen > 240 {
		fileName = imageTitle[0:nameLen-(nameLen-240)] + imageIDString
	} else {
		fileName = imageTitle + imageIDString
	}

	// db folder/ first_author / "File Name"?_id_"image set index".format
	fileName = fmt.Sprintf("%s_%d%s", fileName, setIndex, fileEnding)
	return fileName
}

func getType(file string) string {

	indexOfTypeStart := strings.Index(file, ".")

	if indexOfTypeStart == -1 {
		return ""
	}
	return file[indexOfTypeStart:]
}

func MakeFileDirectory(FirstAuthorName string) (string, error) {

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

func CheckImageSetFileDeletionPermisons(entryToDelete ImageSetMongo) error {
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

		err := os.Remove(path + entry.LowResName)
		if err != nil {
			fmt.Printf("Failed to Find File: %s\n", entry)
			errList = errors.Join(errList, err)
		}
	}
	return errList
}

// function SaveFile() error{

// }

func CleanImagSetForFrontEnd(iSet ...ImageSetMongo) []ImageSetMongo {
	for index, _ := range iSet {
		iSet[index].Image = nil
		//iSet[index].LowImage = nil
		//iSet[index].IsImageActive = nil
		iSet[index].Path = ""
	}
	return iSet
}
