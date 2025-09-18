package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"log"
	"mime/multipart"
	"os"
	"strconv"
	"strings"

	"github.com/ajdnik/imghash"
	"github.com/valyala/fasthttp"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type InternalResponse struct {
	errorCode   int
	errorString string
}

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
	for index, entry := range entryToDelete.ImageLinks {
		info, err := os.Stat(entry)
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("No File Exists for image number: " + strconv.Itoa(index))
		}
		if info.Mode().Perm()&0222 == 0 {
			return errors.New("No permission to delete image number: " + strconv.Itoa(index))
		}
	}
	for index, entry := range entryToDelete.LowImageLinks {
		info, err := os.Stat(entry)
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("No File Exists for low-res image number: " + strconv.Itoa(index))
		}
		if info.Mode().Perm()&0222 == 0 {
			return errors.New("No permission to delete low-res image number: " + strconv.Itoa(index))
		}
	}
	return nil
}

func DeleteFileList(links []string) error {
	var errList error
	for _, entry := range links {
		if entry == "" {
			continue
		}

		err := os.Remove(entry)
		if err != nil {
			fmt.Printf("Failed to Find File: %s\n", entry)
			errList = errors.Join(errList, err)
		}
	}
	return errList
}

// function SaveFile() error{

// }

func AddImageSet(imageSet ImageSetMongo, media []*multipart.FileHeader) (InternalResponse, map[int][]CollisionResponsePair) {

	//add to DB
	insertResult, err := collection.InsertOne(context.Background(), imageSet)

	if err != nil {
		return InternalResponse{500, err.Error()}, nil
	}

	imageSet.ID = insertResult.InsertedID.(bson.ObjectID)

	//determine folder path
	filePath, err := MakeFileDirectory(imageSet.Authors[0])
	if err != nil {
		return InternalResponse{500, err.Error()}, nil
	}

	if len(media) == 0 {
		//Todo: send proper feedback
		return InternalResponse{400, "No Media attached"}, nil
	}

	hashHits := make(map[int][]CollisionResponsePair)

	for index, item := range media {
		fmt.Println(item.Filename, item.Size, item.Header["Content-Type"][0])

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

		/**		save media		**/

		fileName := ImageFileName(imageSet.Title, imageSet.ID, index, getType(item.Filename))
		fullPath := fmt.Sprintf("%s%s", filePath, fileName)

		log.Print("FilePath: " + fullPath)
		err = fasthttp.SaveMultipartFile(item, fullPath)
		//err = SaveMultipartFile(item, fullPath)
		if err != nil {
			return InternalResponse{500, err.Error()}, nil
		}
		imageSet.ImageLinks = append(imageSet.ImageLinks, fullPath)

		/** 	get hash 	**/
		file, _ := item.Open()

		img, _, err := image.Decode(file)
		if err != nil {
			os.Remove(fullPath)
			return InternalResponse{500, err.Error()}, nil
		}
		phash := imghash.NewPHash()
		ihash := phash.Calculate(img)
		fmt.Printf("Hashed to: %v\n", ihash)
		imageSet.ImageHash = append(imageSet.ImageHash, ihash.String())
		file.Close()

		//compare hash in DB

		HitResults, err := findOverlappingHashes(ihash.String())

		if err != nil {
			return InternalResponse{500, err.Error()}, nil
		}
		if len(HitResults) != 0 {
			fmt.Println("Hash Hit")
			hashHits[index] = HitResults
		}

		//cursor, err := collection.Find(context.Background(), bson.M{"hash": ihash.String()})

	}

	log.Print("Files Uploaded")

	update := bson.M{"$set": imageSet}
	result, err := collection.UpdateByID(context.Background(), imageSet.ID, update)

	if err != nil {
		fmt.Println("Update Failed")
		return InternalResponse{500, err.Error()}, nil
	}

	if result.MatchedCount == 0 {
		log.Print("COULD NOT UPDATE DB FILE AFTER ADDING INFO")
		return InternalResponse{500, "Error while updating db entry after saving files"}, nil
		//return c.Status(500).SendString()
	}
	log.Println("---Upload complete---")
	//hash conflict detected
	if len(hashHits) != 0 {
		//return InternalErrorHandle{202, "Error while updating db entry after saving files"}
		return InternalResponse{202, "Ok, Hash collision detected"}, hashHits
	}

	return InternalResponse{201, "Ok, Added to DB"}, nil
}
