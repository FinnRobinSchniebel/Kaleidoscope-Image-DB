package main

import (
	"fmt"
	"os"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

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

// function SaveFile() error{

// }
