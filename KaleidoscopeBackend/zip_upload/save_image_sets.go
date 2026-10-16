package zipupload

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"log"
	"os"
	"path/filepath"
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

		files := make([]imageset.MediaSource, len(bundle.FilePath))
		for i, path := range bundle.FilePath {
			files[i] = imageset.DiskSource{Path: filepath.Join(basePath, path)}
		}

		imageset.PrintISet(&bundle.Iset)

		//a failed group leaves nothing behind, so the remaining groups can still be imported
		report.Record(imageset.ProcessImageSet(&bundle.Iset, files, user, bundle.Key)...)

		for _, path := range bundle.FilePath {
			os.Remove(filepath.Join(basePath, path))
		}
	}

	report.Finish(notification.OutcomeCompleted, nil)
	if err := notification.Publish(user, report); err != nil {
		log.Printf("------ Warning: zip import [%s]: %s ------", user, err)
	}
}
