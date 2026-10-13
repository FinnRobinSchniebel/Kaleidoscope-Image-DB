package zipupload

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
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
		bundle := &fileIsetData[setIndex]

		incoming := make([]imageset.IncomingImage, len(bundle.Images))
		for i, bundleImage := range bundle.Images {
			incoming[i] = imageset.IncomingImage{ID: bundleImage.ID, Media: imageset.DiskSource{Path: filepath.Join(basePath, bundleImage.Path)}}
		}

		imageset.PrintISet(&bundle.Iset)

		//a failed group leaves nothing behind, so the remaining groups can still be imported
		if _, err := imageset.ProcessNewImageSet(&bundle.Iset, incoming, user); err != nil {
			report.Record(notification.FailedItem(user, bundle.Key, bson.NilObjectID, err))
			continue
		}
		report.Record(notification.ItemResult{Kind: notification.ItemAdded, Ref: bundle.Key, SetID: bundle.Iset.ID})

		for _, bundleImage := range bundle.Images {
			os.Remove(filepath.Join(basePath, bundleImage.Path))
		}
	}

	report.Finish(notification.OutcomeCompleted, nil)
	if err := notification.Publish(user, report); err != nil {
		log.Printf("------ Warning: zip import [%s]: %s ------", user, err)
	}
}
