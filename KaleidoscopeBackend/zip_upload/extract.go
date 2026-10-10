package zipupload

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/yeka/zip"
)

func Unzip(src, dest string) (string, error) {
	r, err := zip.OpenReader(src)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := r.Close(); err != nil {
			log.Print(err)
		}
	}()

	zipBase := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	extractRoot := filepath.Join(dest, zipBase)

	err = os.MkdirAll(extractRoot, 0755)
	if err != nil {
		return "", err
	}

	// Closure to address file descriptors issue with all the deferred .Close() methods
	extractAndWriteFile := func(f *zip.File) (err error) {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer func() {
			err = errors.Join(err, rc.Close())
		}()

		path := filepath.Join(extractRoot, f.Name)

		// Check for ZipSlip (Directory traversal)
		if !strings.HasPrefix(path, filepath.Clean(extractRoot)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", path)
		}

		// The zip's own permission bits are ignored: a path's folders may have no
		// entry of their own, and an upload must not choose modes on this server.
		if f.FileInfo().IsDir() {
			os.MkdirAll(path, 0700)
		} else {
			os.MkdirAll(filepath.Dir(path), 0700)
			var out *os.File
			out, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
			if err != nil {
				return err
			}
			defer func() {
				err = errors.Join(err, out.Close())
			}()

			_, err = io.Copy(out, rc)
			if err != nil {
				return err
			}
		}
		return nil
	}

	for _, f := range r.File {
		err := extractAndWriteFile(f)
		if err != nil {
			return "", err
		}
	}

	return extractRoot, nil
}

func RemoveFolder(loc string) error {
	if loc == "" {
		return fmt.Errorf("No File path")

	}
	err := os.RemoveAll(loc)
	if err != nil {
		return err
	}

	return nil

}
