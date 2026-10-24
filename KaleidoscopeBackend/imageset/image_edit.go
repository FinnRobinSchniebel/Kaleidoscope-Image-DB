package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"fmt"
	"image"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
)

// writeFileAtomic writes to a file in tempDir, then renames it to dir/name,
// so readers never see a partial file.
func writeFileAtomic(dir, name string, encode func(io.Writer) error) error {
	tmp, err := os.CreateTemp(tempDir(), "kscope-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename

	if err := encode(tmp); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, name))
}

// maxImagePixels caps an image's width times height (a GIF's canvas), checked
// from its header: a small file can claim a size whose decode exhausts memory.
const maxImagePixels = 200_000_000

// checkImageHeader reads media's header and returns its format: png, jpeg or
// gif, which is also the stored file's extension. Any other format is an
// error, and a size over maxImagePixels is notification.ErrImageTooLarge.
// Nothing is decoded.
func checkImageHeader(media MediaSource) (format string, err error) {

	file, err := media.Open()
	if err != nil {
		return "", fmt.Errorf("could not open uploaded file: %w", err)
	}
	defer file.Close()

	config, format, err := image.DecodeConfig(file)
	if err != nil {
		return "", fmt.Errorf("failed to read image info: %w", err)
	}

	switch format {
	case "png", "jpeg", "gif":
	default:
		return "", fmt.Errorf("unsupported image format %q", format)
	}

	if pixels := int64(config.Width) * int64(config.Height); pixels > maxImagePixels {
		return "", fmt.Errorf("%w: %dx%d pixels, over %d", notification.ErrImageTooLarge, config.Width, config.Height, maxImagePixels)
	}
	return format, nil
}

func FileHeaderToImage(fileHeader MediaSource) (image.Image, string, error) {
	// Open the uploaded file
	file, err := fileHeader.Open()
	if err != nil {
		return nil, "", fmt.Errorf("could not open uploaded file: %w", err)
	}
	defer file.Close()

	imageInfo, itype, err := image.DecodeConfig(file)
	//Important decodeConfig eats the first bytes of the file reader and does not reset to the start
	file.Seek(0, 0)

	if err != nil {
		return nil, "", fmt.Errorf("failed to read image info")
	}

	fmt.Printf("file:  w: %d, h: %d type: %s \n", imageInfo.Width, imageInfo.Height, itype)

	err = CheckImageSize(fileHeader)
	//image is larger then a 500mb
	if err != nil {
		return nil, "", err
	}

	// Decode to image.Image
	img, format, err := image.Decode(file)
	if err != nil {
		return nil, "", fmt.Errorf("could not decode image: %w", err)
	}

	return img, format, nil
}

func FileHeaderToGif(fileHeader MediaSource) (*gif.GIF, error) {

	err := CheckImageSize(fileHeader)
	//image is larger then a 500mb
	if err != nil {
		return nil, err
	}

	// Open the uploaded file
	file, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("could not open uploaded file: %w", err)
	}
	defer file.Close()

	// Decode to image.Image
	gif, err := gif.DecodeAll(file)
	if err != nil {
		return nil, fmt.Errorf("could not decode image: %w", err)
	}

	return gif, nil
}
