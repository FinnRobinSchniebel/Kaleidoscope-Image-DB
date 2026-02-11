package zipupload

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"

	"github.com/valyala/fasthttp"
)

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

func buildLayerRegex(layer string) *regexp.Regexp {
	return nil
}

func ValidateStructure() {

}

func saveFiles() {

}
