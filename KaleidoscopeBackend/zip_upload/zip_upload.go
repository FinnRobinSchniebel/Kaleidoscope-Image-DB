package zipupload

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"
	"fmt"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

type ImageSetFileBundle struct {
	Iset     imageset.ImageSetMongo
	FilePath []string
}

func ProcessZip(fileHeader *multipart.FileHeader, ruleLayers []string, fileLayer string, groupingIndex int, user string) (status int, collisions map[int][]imageset.CollisionResponsePair, skipped []string, errors []string, err error) {

	//check array lengths and Grouping level index in range
	if len(ruleLayers) > 11 {
		return fiber.StatusBadRequest, nil, nil, nil, fmt.Errorf("Too many folder Layers.")
	}
	//only greater than since it could be the file level
	if groupingIndex > len(ruleLayers) {
		return fiber.StatusBadRequest, nil, nil, nil, fmt.Errorf("Grouping index is out of bounds. Please select a valid group index")
	}

	log.Println("RuleLayers: ")
	log.Print(ruleLayers)

	if len(ruleLayers) == 0 {
		return fiber.StatusBadRequest, nil, nil, nil, fmt.Errorf("No rule layers provided, Cannot attain any info")
	}
	if filepath.Ext(fileHeader.Filename) != ".zip" {
		return fiber.StatusBadRequest, nil, nil, nil, fmt.Errorf("Invalid file type. Only .zip allowed")
	}

	//Note: File must be saved before use because Zip reader requires a on disc location (not always the case wih fiber)
	err, pathName := DownloadZip(fileHeader)
	if err != nil {
		return fiber.StatusInternalServerError, nil, nil, nil, fmt.Errorf("Failed to download Zip. Check server disk space.")
	}
	defer RemoveTempZip(pathName)

	//unzip the zip for better access
	folderPathName, err := Unzip(pathName, imageset.BackendVolumeLocation+"/Temp")

	if err != nil {
		return fiber.StatusInternalServerError, nil, nil, nil, fmt.Errorf("zip could not be Processed: %s", err.Error())
	}

	delegatedCleanup := false

	defer func() {
		if delegatedCleanup {
			return
		}
		err := RemoveFolder(folderPathName)
		if err != nil {
			log.Print(err)
		}
	}()

	cont, err := ValidateAndParseFolder(folderPathName, ruleLayers, fileLayer, groupingIndex)
	if err != nil {
		return fiber.StatusBadRequest, nil, nil, nil, fmt.Errorf("failed to parse files: %s", err.Error())
	}

	ISets, skipped, errors, err := createImageSetsFromParsedZipData(folderPathName, cont)

	//log.Print(cont, err)
	log.Print("Sets Print: ")

	delegatedCleanup = true

	go SaveImageSets(folderPathName, ISets, user)

	//cleanup
	//err = RemoveTempZip(pathName)

	return 200, nil, skipped, errors, nil
}

func DownloadZip(fileHeader *multipart.FileHeader) (error, string) {
	var location = imageset.BackendVolumeLocation + "/Temp"
	if err := os.MkdirAll(location, 0755); err != nil {
		return err, ""
	}
	tempFilePath := filepath.Join(location, fileHeader.Filename)
	if err := fasthttp.SaveMultipartFile(fileHeader, tempFilePath); err != nil {
		return err, ""
	}
	return nil, tempFilePath
}

func RemoveTempZip(filePath string) error {
	if filePath == "" {
		return fmt.Errorf("No File path")

	}
	err := os.Remove(filePath)
	if err != nil {
		return err
	}

	return nil
}

//takes in only the parsed Map
// Returns the image sets created from the maps, the skipped items list, the error List, and an error if fatal error occurs

func createImageSetsFromParsedZipData(BaseFolderPath string, parsedDataMap map[string][]ParsedFolderInfo) ([]ImageSetFileBundle, []string, []string, error) {

	var skippedList []string
	var errorList []string
	var err error
	var result []ImageSetFileBundle

	//for each item in the Data map make them a separate ImageSet
	for groupingKey := range parsedDataMap {

		//temp variable to keep track of titles
		var titles []string

		containsImage := false
		for i := range parsedDataMap[groupingKey] {
			if IsValidImageExtension(parsedDataMap[groupingKey][i].FileType) {
				containsImage = true
				break
			}
		}
		//skip all that don't have a valid file format for supported images/video
		if !containsImage {
			skippedList = append(skippedList, groupingKey)
			continue
		}

		//for each Item in the imageSet check its contents for info and text file discription
		var newISet imageset.ImageSetMongo
		var Paths []string

		for dataIndex := range parsedDataMap[groupingKey] {

			//If it is a text file add the contents as a description to the image Set
			if parsedDataMap[groupingKey][dataIndex].FileType == ".txt" {
				disc, err := readTxtAsDescription(BaseFolderPath, parsedDataMap[groupingKey][dataIndex].Path)
				if err != nil {
					errorList = append(errorList, "Could Not read: "+parsedDataMap[groupingKey][dataIndex].Path+" error: "+err.Error())
				} else {
					if newISet.Description == "" {
						newISet.Description = disc
					} else {
						newISet.Description = newISet.Description + "\n\n" + disc
					}
				}
				continue
			}

			Author := parsedDataMap[groupingKey][dataIndex].Values["Author"]

			//TODO: Will be removed and replaced when better author attribution is there
			if !slices.Contains(newISet.Authors, Author) && Author != "" {
				newISet.Authors = append(newISet.Authors, Author)
			}

			//Combine all titles into one string (a duplicate exists separated in sources so this one can be changed by the user)
			newTitle := parsedDataMap[groupingKey][dataIndex].Values["Title"]
			if !slices.Contains(titles, newTitle) {
				titles = append(titles, newTitle)
				if newISet.Title != "" {
					newISet.Title = newISet.Title + " | "
				}
				newISet.Title = newISet.Title + newTitle
			}

			var newSource imageset.SourceInfo
			newSource.Name = parsedDataMap[groupingKey][dataIndex].Values["Source"]
			newSource.SourceID = parsedDataMap[groupingKey][dataIndex].Values["ID"]
			newSource.AuthorID = parsedDataMap[groupingKey][dataIndex].Values["AuthorId"]
			newSource.Title = parsedDataMap[groupingKey][dataIndex].Values["Title"]
			newSource.SourceAuthor = Author
			newSource.AttributedTo = append(newSource.AttributedTo, dataIndex)
			Paths = append(Paths, parsedDataMap[groupingKey][dataIndex].Path)

			//add Date to data set (accepts format with - and _)
			if parsedDataMap[groupingKey][dataIndex].Values["Date"] != "" {
				newSource.Date, err = dateParse(parsedDataMap[groupingKey][dataIndex].Values["Date"])
				if err != nil {
					log.Print("Could not parse date: " + parsedDataMap[groupingKey][dataIndex].Values["Date"])
					errorList = append(errorList, err.Error())
				}
			}

			//add new source entry if needed else add index to the existing source
			containsSourceAt := false
			for i := range newISet.Sources {
				if imageset.SourceInfoEqual(newSource, newISet.Sources[i]) {
					newISet.Sources[i].AttributedTo = append(newISet.Sources[i].AttributedTo, dataIndex)
					containsSourceAt = true
					break
				}
			}
			if !containsSourceAt {
				newISet.Sources = append(newISet.Sources, newSource)
			}
		}
		Combined := ImageSetFileBundle{Iset: newISet, FilePath: Paths}
		result = append(result, Combined)
	}
	return result, skippedList, errorList, nil
}

func dateParse(Date string) (time.Time, error) {
	Date = strings.ReplaceAll(Date, "+", "") // in case they left it in
	Date = strings.ReplaceAll(Date, "_", "-")
	Date = strings.ReplaceAll(Date, "/", "-")
	return time.Parse("01-02-2006", Date)
}

func IsValidImageExtension(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))

	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif":
		return true
	default:
		return false
	}
}

func readTxtAsDescription(baseFolderPath string, relativeFilePath string) (string, error) {

	fullPath := filepath.Join(baseFolderPath, relativeFilePath)

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}

	return string(data), nil
}
