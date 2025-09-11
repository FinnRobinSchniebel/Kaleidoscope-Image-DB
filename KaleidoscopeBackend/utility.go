package main

import (
	"fmt"
	"os"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func ImageFileName(imageTitle string, imageId bson.ObjectID, setIndex int) string {
	var fileName string
	if nameLen := len(imageTitle + "_" + imageId.Hex()); nameLen > 240 {

		fileName = imageTitle[0 : nameLen-(nameLen-240)]
	} else {
		fileName = imageTitle + imageId.Hex()
	}

	// db folder/ first_author / "File Name"?_id_"image set index".format
	fileName = fmt.Sprintf("%s_%d", fileName, setIndex)
	return fileName
}

func MakeFileDirectory(FirstAuthorName string) error {
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
		return err
	}

	//fileInfo, _ = os.Stat(filePath)
	fmt.Println("Folder Created")

	return nil
}

// function SaveFile() error{

// }
