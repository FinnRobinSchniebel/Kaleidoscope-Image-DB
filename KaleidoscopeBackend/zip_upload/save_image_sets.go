package zipupload

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

type SetAndImage struct {
	Iset       imageset.ImageSetMongo
	ImagePaths []string
}

func SaveImageSets(basePath string, fileIsetData []ImageSetFileBundle, user string) (map[string]imageset.CollisionMap, int, error) {

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

		hits, iSetDbId, err := imageset.AddImageSet(&fileIsetData[setIndex].Iset, MedSour, user)

		if err.ErrorCode > 299 {
			return nil, count, fmt.Errorf("%s", err.ErrorString)
		}
		result[iSetDbId] = hits
		count++

		for _, Path := range fileIsetData[setIndex].FilePath {
			os.Remove(filepath.Join(basePath, Path))
		}
	}

	return result, count, nil
}
