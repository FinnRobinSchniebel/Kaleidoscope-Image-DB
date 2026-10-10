package zipupload

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// basePath is the extracted zip's root, used to resolve the relative file paths in fileIsetData.
// cleanupPath is the temp directory wrapping basePath and is removed wholesale once done.
// Each group's result is added to report, which is then finished and published.
func SaveImageSets(basePath string, cleanupPath string, fileIsetData []ImageSetFileBundle, user string, report *notification.ImportReport) {

	//Authority to delete the temparary files is delegated to here
	defer func() {
		err := RemoveFolder(cleanupPath)
		if err != nil {
			log.Print(err)
		}
	}()

	//synchronous for now to avoid possible memory issues
	for setIndex := range fileIsetData {

		MedSour := make([]imageset.MediaSource, len(fileIsetData[setIndex].FilePath))

		for i, Path := range fileIsetData[setIndex].FilePath {
			fullPath := filepath.Join(basePath, Path)
			MedSour[i] = imageset.DiskSource{Path: fullPath}
		}

		imageset.PrintISet(&fileIsetData[setIndex].Iset)
		log.Print(fileIsetData[setIndex].FilePath)

		hits, _, err := imageset.AddImageSet(&fileIsetData[setIndex].Iset, MedSour, user)

		//AddImageSet undoes its own partial writes, so the remaining groups can still be imported
		if err != nil {
			err = fmt.Errorf("%w: %w", notification.ErrSaveFailed, err)
			report.Record(notification.FailedItem(user, fileIsetData[setIndex].Key, bson.NilObjectID, err))
			continue
		}
		if len(hits) != 0 {
			log.Printf("zip import [%s]: group %q has duplicate images: %v", user, fileIsetData[setIndex].Key, hits)
		}
		report.Record(notification.ItemResult{Kind: notification.ItemAdded, Ref: fileIsetData[setIndex].Key, SetID: fileIsetData[setIndex].Iset.ID})

		for _, Path := range fileIsetData[setIndex].FilePath {
			os.Remove(filepath.Join(basePath, Path))
		}
	}

	report.Finish(notification.OutcomeCompleted, nil)
	if err := notification.Publish(user, report); err != nil {
		log.Printf("------ Warning: zip import [%s]: %s ------", user, err)
	}
}
