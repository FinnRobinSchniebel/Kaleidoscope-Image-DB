package zipupload

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"
	"log"
	"os"
	"path/filepath"
)

func SaveImageSets(basePath string, fileIsetData []ImageSetFileBundle, user string) {

	//Authority to delete the temparary files is delegated to here
	defer func() {
		err := RemoveFolder(basePath)
		if err != nil {
			log.Print(err)
		}
	}()

	var count int

	result := make(map[string]imageset.CollisionMap)

	//synchronous for now to avoid possible memory issues
	for setIndex := range fileIsetData {

		MedSour := make([]imageset.MediaSource, len(fileIsetData[setIndex].FilePath))

		for i, Path := range fileIsetData[setIndex].FilePath {
			fullPath := filepath.Join(basePath, Path)
			MedSour[i] = imageset.DiskSource{Path: fullPath}
		}

		imageset.PrintISet(fileIsetData[setIndex].Iset)
		log.Print(fileIsetData[setIndex].FilePath)

		hits, iSetDbId, err := imageset.AddImageSet(&fileIsetData[setIndex].Iset, MedSour, user)

		if err.ErrorCode > 299 {
			log.Print(err.ErrorString)
			return
		}
		result[iSetDbId] = hits
		count++

		for _, Path := range fileIsetData[setIndex].FilePath {
			os.Remove(filepath.Join(basePath, Path))
		}
	}

	log.Print(result)
	log.Print(count)

	return
}
