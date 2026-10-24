package zipupload

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"

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

	//runs before the cleanup above; a panic in finishing or publishing the report is only logged
	defer func() {
		if r := recover(); r != nil {
			log.Printf("------ Warning: zip import [%s] panicked while finishing its report: %v ------\n%s", user, r, debug.Stack())
		}
	}()

	outcome := notification.OutcomeCompleted
	err := importZipGroups(basePath, fileIsetData, user, report)
	if err != nil {
		outcome = notification.OutcomeFailed
	}

	report.Finish(outcome, err)
	if err := notification.Publish(user, report); err != nil {
		log.Printf("------ Warning: zip import [%s]: %s ------", user, err)
	}
}

// importZipGroups imports each group in order. A panic stops the import: it's
// logged, the group it happened in is recorded as failed and the rest as
// skipped, and it's returned as an error.
func importZipGroups(basePath string, groups []ImageSetFileBundle, user string, report *notification.ImportReport) (err error) {

	current := 0
	defer func() {

		r := recover()
		if r == nil {
			return
		}

		log.Printf("------ Warning: zip import [%s] panicked in %s: %v ------\n%s", user, groups[current].Key, r, debug.Stack())
		err = fmt.Errorf("zip import panicked: %v", r)
		report.Record(notification.FailedItem(user, groups[current].Key, bson.NilObjectID, err))
		for _, rest := range groups[current+1:] {
			report.Record(notification.ItemResult{Kind: notification.ItemSkipped, Ref: rest.Key, Reason: "the import stopped after an error"})
		}
	}()

	//synchronous for now to avoid possible memory issues
	for current = range groups {
		bundle := &groups[current]

		files := make([]imageset.MediaSource, len(bundle.FilePath))
		for i, path := range bundle.FilePath {
			files[i] = imageset.DiskSource{Path: filepath.Join(basePath, path)}
		}

		imageset.PrintISet(&bundle.Iset)

		//a failed group leaves nothing behind, so the remaining groups can still be imported
		report.Record(imageset.ProcessImageSet(context.Background(), &bundle.Iset, files, user, bundle.Key)...)

		for _, path := range bundle.FilePath {
			os.Remove(filepath.Join(basePath, path))
		}
	}
	return nil
}
