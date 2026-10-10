package zipupload

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"fmt"
	"image"
	"log"
	"maps"
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
	Key      string //grouping folder path, for reporting
	Iset     imageset.ImageSetMongo
	FilePath []string
}

func ProcessZip(fileHeader *multipart.FileHeader, ruleLayers []string, fileLayer string, groupingIndex int, user string) (status int, collisions map[int][]imageset.CollisionResponsePair, skipped []string, errors []string, err error) {

	//check array lengths and Grouping level index in range
	if len(ruleLayers) > 11 {
		return fiber.StatusBadRequest, nil, nil, nil, fmt.Errorf("Too many folder Layers.")
	}
	//only greater than since it could be the file level
	if groupingIndex < 0 || groupingIndex > len(ruleLayers) {
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
	//each request gets its own temp dir so concurrent uploads can never collide on the zip's base name
	unzipTempDir, err := os.MkdirTemp("", "kaleidoscope_unzip_*")
	if err != nil {
		return fiber.StatusInternalServerError, nil, nil, nil, fmt.Errorf("Failed to create temp directory.")
	}

	folderPathName, err := Unzip(pathName, unzipTempDir)

	if err != nil {
		os.RemoveAll(unzipTempDir)
		return fiber.StatusInternalServerError, nil, nil, nil, fmt.Errorf("zip could not be Processed: %s", err.Error())
	}

	delegatedCleanup := false

	defer func() {
		if delegatedCleanup {
			return
		}
		err := RemoveFolder(unzipTempDir)
		if err != nil {
			log.Print(err)
		}
	}()

	cont, skippedFiles, err := ValidateAndParseFolder(folderPathName, ruleLayers, fileLayer, groupingIndex)
	if err != nil {
		return fiber.StatusBadRequest, nil, nil, nil, fmt.Errorf("failed to parse files: %s", err.Error())
	}

	ISets, skippedGroups, errors, err := createImageSetsFromParsedZipData(folderPathName, cont)
	skips := append(skippedFiles, skippedGroups...)
	for _, s := range skips {
		skipped = append(skipped, skipMessage(s))
	}

	report := notification.NewImportReport(filepath.Base(fileHeader.Filename))
	report.Record(skips...)
	report.Notes = errors

	//log.Print(cont, err)
	log.Print("Sets Print: ")

	delegatedCleanup = true

	go SaveImageSets(folderPathName, unzipTempDir, ISets, user, report)

	//cleanup
	//err = RemoveTempZip(pathName)

	return 200, nil, skipped, errors, nil
}

func DownloadZip(fileHeader *multipart.FileHeader) (error, string) {
	location, err := os.MkdirTemp("", "kaleidoscope_zip_*")
	if err != nil {
		return err, ""
	}
	//sanitize the client-supplied filename so it cannot escape the temp dir (e.g. "../../etc/foo.zip")
	tempFilePath := filepath.Join(location, filepath.Base(fileHeader.Filename))
	if err := fasthttp.SaveMultipartFile(fileHeader, tempFilePath); err != nil {
		return err, ""
	}
	return nil, tempFilePath
}

// RemoveTempZip removes the entire per-upload temp directory created by DownloadZip.
func RemoveTempZip(filePath string) error {
	if filePath == "" {
		return fmt.Errorf("No File path")

	}
	err := os.RemoveAll(filepath.Dir(filePath))
	if err != nil {
		return err
	}

	return nil
}

//takes in only the parsed Map
// Returns the image sets created from the maps, the skipped items list, the error List, and an error if fatal error occurs

func createImageSetsFromParsedZipData(BaseFolderPath string, parsedDataMap map[string][]ParsedFolderInfo) ([]ImageSetFileBundle, []notification.ItemResult, []string, error) {

	var skippedList []notification.ItemResult
	var errorList []string
	var result []ImageSetFileBundle

	//for each item in the Data map make them a separate ImageSet (sorted so results are stable)
	for _, groupingKey := range slices.Sorted(maps.Keys(parsedDataMap)) {
		bundle, skipped, errs := buildImageSetBundle(BaseFolderPath, groupingKey, parsedDataMap[groupingKey])
		skippedList = append(skippedList, skipped...)
		errorList = append(errorList, errs...)
		if bundle != nil {
			result = append(result, *bundle)
		}
	}
	return result, skippedList, errorList, nil
}

// buildImageSetBundle turns one group's files into an image set whose parsed
// fields belong to its sources. A nil bundle means the whole group was skipped:
// a work is never imported with pieces missing.
func buildImageSetBundle(basePath string, key string, entries []ParsedFolderInfo) (*ImageSetFileBundle, []notification.ItemResult, []string) {
	var skipped []notification.ItemResult
	var errorList []string
	var set imageset.ImageSetMongo
	var paths []string
	var descriptions []ParsedFolderInfo //.txt files, attached once every source exists

	for _, entry := range entries {
		if entry.Conflict != "" {
			return nil, append(skipped, notification.ItemResult{Kind: notification.ItemSkipped, Ref: key, Reason: fmt.Sprintf("%s in %s", entry.Conflict, entry.Path)}), errorList
		}

		if entry.FileType == ".txt" {
			descriptions = append(descriptions, entry)
			continue
		}

		if !IsValidImageExtension(entry.FileType) {
			skipped = append(skipped, notification.ItemResult{Kind: notification.ItemSkipped, Ref: entry.Path, Reason: "not an image"})
			continue
		}
		if err := probeImage(filepath.Join(basePath, entry.Path)); err != nil {
			return nil, append(skipped, notification.ItemResult{Kind: notification.ItemSkipped, Ref: key, Reason: "unreadable image " + entry.Path}), errorList
		}

		source, err := sourceFromValues(entry.Values)
		if err != nil {
			errorList = append(errorList, err.Error())
		}

		//index among this set's images only, so .txt and skipped files never shift attribution
		imageIndex := len(paths)
		paths = append(paths, entry.Path)

		//a group is one work from one source; its files may only add details the others left empty
		switch {
		case len(set.Sources) == 0:
			source.AttributedTo = []int{imageIndex}
			set.Sources = []imageset.SourceInfo{source}
		case imageset.SameSource(source, set.Sources[0]):
			if conflict := mergeSourceFields(&set.Sources[0], source); conflict != "" {

				return nil, append(skipped, notification.ItemResult{Kind: notification.ItemSkipped, Ref: key, Reason: fmt.Sprintf("%s in %s", conflict, entry.Path)}), errorList
			}
			set.Sources[0].AttributedTo = append(set.Sources[0].AttributedTo, imageIndex)
		default:
			//a set built from several different sources is not supported
			return nil, append(skipped, notification.ItemResult{Kind: notification.ItemSkipped, Ref: key, Reason: fmt.Sprintf("combines different sources: %s and %s in %s", sourceLabel(set.Sources[0]), sourceLabel(source), entry.Path)}), errorList
		}
	}

	if len(paths) == 0 {
		return nil, append(skipped, notification.ItemResult{Kind: notification.ItemSkipped, Ref: key, Reason: "no images"}), errorList
	}

	for _, entry := range descriptions {
		text, err := readTxtAsDescription(basePath, entry.Path)
		if err != nil {
			log.Printf("zip import: reading %s: %v", entry.Path, err)
			errorList = append(errorList, "couldn't read "+entry.Path)
			continue
		}
		//the group's only source is the one every description belongs to
		set.Sources[0].Description = imageset.JoinDescriptions(set.Sources[0].Description, text)
	}

	imageset.DeriveFromSources(&set)
	return &ImageSetFileBundle{Key: key, Iset: set, FilePath: paths}, skipped, errorList
}

// mergeSourceFields fills dst's empty Title, Author, AuthorId and Date from
// src. Returns a description of the first field where both are set but
// differ, or "" when they agree.
func mergeSourceFields(dst *imageset.SourceInfo, src imageset.SourceInfo) string {
	textFields := []struct {
		name string
		dst  *string
		src  string
	}{
		{"Title", &dst.Title, src.Title},
		{"Author", &dst.SourceAuthor, src.SourceAuthor},
		{"AuthorId", &dst.AuthorID, src.AuthorID},
	}
	for _, f := range textFields {
		switch {
		case f.src == "" || f.src == *f.dst:
		case *f.dst == "":
			*f.dst = f.src
		default:
			return fmt.Sprintf("conflicting [%s] %q vs %q", f.name, *f.dst, f.src)
		}
	}

	switch {
	case src.Date.IsZero() || src.Date.Equal(dst.Date):
	case dst.Date.IsZero():
		dst.Date = src.Date
	default:
		return fmt.Sprintf("conflicting [Date] %s vs %s", dst.Date.Format(time.DateOnly), src.Date.Format(time.DateOnly))
	}
	return ""
}

// skipMessage formats a skip for the upload response as "ref (reason)".
func skipMessage(r notification.ItemResult) string {
	return fmt.Sprintf("%s (%s)", r.Ref, r.Reason)
}

// sourceLabel names a source for messages: "pixiv/42", or just "upload" without an id.
func sourceLabel(s imageset.SourceInfo) string {
	if s.SourceID == "" {
		return s.Name
	}
	return s.Name + "/" + s.SourceID
}

// sourceFromValues builds a source from one file's parsed fields. LastChecked
// is left zero so a matching service sync still fills in missing metadata.
func sourceFromValues(values map[string]string) (imageset.SourceInfo, error) {
	source := imageset.SourceInfo{
		Name:         imageset.NormalizeSourceName(values["Source"]),
		SourceID:     values["ID"],
		AuthorID:     values["AuthorId"],
		Title:        values["Title"],
		SourceAuthor: values["Author"],
	}

	//add Date to data set (accepts format with - and _)
	if date := values["Date"]; date != "" {
		parsed, err := dateParse(date)
		if err != nil {
			return source, fmt.Errorf("couldn't read [Date] %q (expected MM-DD-YYYY)", date)
		}
		source.Date = parsed
	}
	return source, nil
}

// probeImage reports whether path decodes as a supported image.
func probeImage(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, _, err = image.DecodeConfig(f)
	return err
}

func dateParse(Date string) (time.Time, error) {
	Date = strings.ReplaceAll(Date, "+", "") // in case they left it in
	Date = strings.ReplaceAll(Date, "_", "-")
	Date = strings.ReplaceAll(Date, "/", "-")
	parsed, err := time.Parse("01-02-2006", Date)
	if err != nil {
		return time.Time{}, err
	}
	return imageset.DayOnlyDate(parsed), nil
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
