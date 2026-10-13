package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"context"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// AutoTagger avoids an import cycle: tagging needs imageset's types, so this
// interface is owned here and implemented by tagging.AutoTagFunc in main.go.
type AutoTagger interface {
	// ProcessSourceTags mutates set in place: merges fetched into
	// Sources[sourceIdx].Tags, and updates AutoTags/Tags for tags new to
	// the set. Leave Sources[sourceIdx].Tags empty first if sourceIdx has
	// never been recorded before, so every fetched tag counts as new.
	ProcessSourceTags(userID string, set *ImageSetMongo, sourceIdx int, fetched []SourceTag) error
	// RecordDeletion undoes usage counts recorded when the now-deleted set
	// was created or synced: source tags from sources, AutoTags from tags.
	RecordDeletion(userID string, sources []SourceInfo, tags []string) error
	// RecomputeSystemTags reconciles set's computed tags (e.g. Lost Media,
	// Untracked) against its current Sources, mutating AutoTags/Tags in
	// place. Does not persist set - callers own the eventual save.
	RecomputeSystemTags(userID string, set *ImageSetMongo) error
	// RefreshUntaggedCount recounts userID's Untagged AutoTag from the
	// database. Call after a write that changed any set's tags.
	RefreshUntaggedCount(userID string) error
	// ResolveTagTerm resolves term (a tag name or partial name from a search
	// query) to the ids of every AutoTag whose Name contains it. If the
	// reserved Untagged AutoTag matches by name, its id is excluded from ids
	// and matchEmpty is set instead, since its id never appears literally in
	// any set's Tags.
	ResolveTagTerm(userID string, term string) (ids []string, matchEmpty bool, err error)
}

var Tagger AutoTagger

type MediaSource interface {
	Open() (io.ReadSeekCloser, error)
	Name() string
	Size() int64
	ContentType() string
	Remove() bool
}

// Disk file Abstraction to Media Source
type DiskSource struct {
	Path string
}

func (d DiskSource) Open() (io.ReadSeekCloser, error) {
	return os.Open(d.Path)
}

func (d DiskSource) Name() string {
	return filepath.Base(d.Path)
}

func (d DiskSource) Size() int64 {
	info, err := os.Stat(d.Path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func (d DiskSource) ContentType() string {
	// derive from extension
	return mime.TypeByExtension(filepath.Ext(d.Path))
}

func (d DiskSource) Remove() bool {
	err := os.Remove(d.Path)
	if err != nil {
		log.Printf("----Error: %s -----", err)
	}

	return err == nil
}

// Multi Part Abstraction to Media Source
type MultipartSource struct {
	FileHeader *multipart.FileHeader
}

func (m MultipartSource) Open() (io.ReadSeekCloser, error) {
	return m.FileHeader.Open()
}

func (m MultipartSource) Name() string {
	return m.FileHeader.Filename
}

func (m MultipartSource) Size() int64 {
	return m.FileHeader.Size
}

func (m MultipartSource) ContentType() string {
	if ct, ok := m.FileHeader.Header["Content-Type"]; ok && len(ct) > 0 {
		return ct[0]
	}
	return ""
}
func (m MultipartSource) Remove() bool {
	return true
}

// ProcessNewImageSet stores set as a new image set: it hashes images, checks
// userID's library for matches (logged and returned), then saves. Nothing is
// written before the check, and errors carry a notification reason.
func ProcessNewImageSet(set *ImageSetMongo, images []IncomingImage, userID string) (CollisionMap, error) {
	prepared, err := prepareImages(images)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", notification.ErrSaveFailed, err)
	}
	matches, err := findImageMatches(userID, prepared)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", notification.ErrLibraryAccess, err)
	}
	if _, err := addImageSet(set, prepared, userID); err != nil {
		return nil, fmt.Errorf("%w: %w", notification.ErrSaveFailed, err)
	}
	if len(matches) != 0 {
		log.Printf("import [%s]: new set %s matches stored images: %v", userID, set.ID.Hex(), matches)
	}
	return matches, nil
}

// IncomingImage is a file to store as a new image, with the ID its source's
// AttributedTo already uses.
type IncomingImage struct {
	ID    bson.ObjectID
	Media MediaSource
}

// PreparedImage is an IncomingImage decoded once to hash and measure it. Its
// pixels aren't kept: addImageSet decodes the file again to save it.
type PreparedImage struct {
	Info  ImageInfo //ID, hashes and size; addImageSet adds the file name
	Media MediaSource
	IsGif bool
}

// prepareImages decodes each incoming file once to hash and measure it, so
// matching can run before anything is written. A file that can't be read
// fails the whole list.
func prepareImages(images []IncomingImage) ([]PreparedImage, error) {
	prepared := make([]PreparedImage, 0, len(images))
	for _, incoming := range images {
		itype, err := getFileTypeFromHeader(incoming.Media)
		if err != nil {
			return nil, fmt.Errorf("reading media type of %s: %w", incoming.Media.Name(), err)
		}
		preparedImage := PreparedImage{Info: ImageInfo{ID: incoming.ID}, Media: incoming.Media, IsGif: itype == "gif"}

		if preparedImage.IsGif {
			decodedGif, err := FileHeaderToGif(incoming.Media)
			if err != nil {
				return nil, fmt.Errorf("decoding gif %s: %w", incoming.Media.Name(), err)
			}
			if preparedImage.Info.ImageHashes, err = HashGif(decodedGif); err != nil {
				return nil, fmt.Errorf("hashing gif %s: %w", incoming.Media.Name(), err)
			}
			preparedImage.Info.Width, preparedImage.Info.Height = decodedGif.Config.Width, decodedGif.Config.Height
		} else {
			decoded, _, err := FileHeaderToImage(incoming.Media)
			if err != nil {
				return nil, fmt.Errorf("decoding image %s: %w", incoming.Media.Name(), err)
			}
			if preparedImage.Info.ImageHashes, err = HashImage(decoded); err != nil {
				return nil, fmt.Errorf("hashing image %s: %w", incoming.Media.Name(), err)
			}
			preparedImage.Info.Width, preparedImage.Info.Height = decoded.Bounds().Dx(), decoded.Bounds().Dy()
		}
		prepared = append(prepared, preparedImage)
	}
	return prepared, nil
}

// unknownAuthor stands in for a set's author until a source provides one.
const unknownAuthor = "unknown"

// ErrNoMedia is returned by addImageSet when the request contained no media
// files. Callers map it to HTTP 400.
var ErrNoMedia = errors.New("no media attached")

// addImageSet saves each image (from prepareImages) as a file named by its ID,
// then inserts imageSet and returns its ID as hex. A failure undoes its own
// partial writes.
func addImageSet(imageSet *ImageSetMongo, images []PreparedImage, userId string) (string, error) {

	//TODO: Test if image size is to large

	//clean file paths to avoid unauthorized access
	imageSet.Image = nil
	imageSet.ThumbNail = ""

	imageSet.KscopeUserId = ""
	// non-nil: a nil slice would marshal as BSON null instead of [], which
	// breaks tagging's $addToSet/$pull against this field later.
	imageSet.AutoTags = []bson.ObjectID{}
	imageSet.Tags = nil
	imageSet.TagRuleOverrides = nil

	//set the author in case of none given to avoid issues with file path creation
	if len(imageSet.Authors) == 0 || (imageSet.Authors[0] == "") {
		imageSet.Authors = []string{unknownAuthor}
	}
	//add userId (done as seperate step to avoid exploits if changes are made)
	imageSet.KscopeUserId = userId
	// generated here rather than by the insert, so files can be named before the set is stored
	imageSet.ID = bson.NewObjectID()

	//check media count first to avoid empty imagsets in db
	if len(images) == 0 {
		return "", ErrNoMedia
	}
	//sources attribute images by ID, so an image without one could never be attributed
	for _, preparedImage := range images {
		if preparedImage.Info.ID.IsZero() {
			return "", fmt.Errorf("image %s has no ID", preparedImage.Media.Name())
		}
	}

	var err error

	/**		Test FilePath	 **/
	err = testFilePath(BackendVolumeLocation)
	//determine folder path for images and add the path to the imagset before first insert

	imageSet.Path, err = MakeFileDirectoryFromAuthor(userId, imageSet.Authors[0])

	if err != nil {
		return "", fmt.Errorf("creating author directory: %w", err)
	}

	// The insert below is the commit point. Until then nothing is stored, so a
	// failure only has to undo the files and counts recorded in imageSet.
	committed := false
	defer func() {
		if !committed {
			undoNewImageSet(imageSet)
		}
	}()

	imageSet.DateAdded = time.Now()
	for idx := range imageSet.Sources {
		imageSet.Sources[idx].LastImageUpdate = imageSet.DateAdded
		imageSet.Sources[idx].LastAppliedUpdate = imageSet.DateAdded
	}

	// Cleared up front so imageSet only ever holds tags ProcessSourceTags has recorded.
	fetched := make([][]SourceTag, len(imageSet.Sources))
	for idx := range imageSet.Sources {
		fetched[idx] = imageSet.Sources[idx].Tags
		imageSet.Sources[idx].Tags = nil
	}
	for idx := range imageSet.Sources {
		if err := Tagger.ProcessSourceTags(userId, imageSet, idx, fetched[idx]); err != nil {
			return "", fmt.Errorf("processing source tags: %w", err)
		}
	}
	if len(imageSet.Sources) == 0 {
		// ProcessSourceTags never ran, and it is what normally recomputes system tags
		if err := Tagger.RecomputeSystemTags(userId, imageSet); err != nil {
			return "", fmt.Errorf("computing system tags: %w", err)
		}
	}

	for _, preparedImage := range images {

		/**		save media		**/
		var fileName string

		//gifs must be handled differently; decoded again since prepareImages keeps no pixels
		if preparedImage.IsGif {
			var decodedGif *gif.GIF
			decodedGif, err = FileHeaderToGif(preparedImage.Media)
			if err != nil {
				return "", fmt.Errorf("decoding gif: %w", err)
			}
			fileName, err = SaveGif(decodedGif, imageSet.Path, imageSet.Title, preparedImage.Info.ID)

		} else {
			var decoded image.Image
			decoded, _, err = FileHeaderToImage(preparedImage.Media)
			if err != nil {
				return "", fmt.Errorf("decoding image: %w", err)
			}
			fileName, err = SaveImage(decoded, imageSet.Path, imageSet.Title, preparedImage.Info.ID, "png")
		}

		if err != nil {
			return "", fmt.Errorf("saving media: %w", err)
		}

		info := preparedImage.Info
		info.Name = fileName
		info.IsImageActive = true
		imageSet.Image = append(imageSet.Image, info)
	}

	log.Print("Files Uploaded")

	if _, err := Collection.InsertOne(context.Background(), imageSet); err != nil {
		return "", fmt.Errorf("inserting image set: %w", err)
	}
	committed = true
	refreshUntagged(userId)

	// After the insert: it writes the thumbnail name to the stored set, and only logs on failure.
	//Note: could be go functioned but that may create a race condition if image is viewed before the save finishes
	CreateThumbnailForNew(imageSet.Path, imageSet.Image[0], imageSet.Title, imageSet.ID)

	log.Println("---Upload complete---")
	return imageSet.ID.Hex(), nil
}

// undoNewImageSet reverses a failed addImageSet that was never stored: the
// image files saved so far and the tag counts recorded for its sources, both
// read from set. Errors are logged, not returned.
func undoNewImageSet(set *ImageSetMongo) {
	if err := DeleteFilesFromInfoList(set.Path, set.Image); err != nil {
		log.Printf("------ Warning: removing files of failed upload: %s ------", err)
	}
	if err := Tagger.RecordDeletion(set.KscopeUserId, set.Sources, set.Tags); err != nil {
		log.Printf("------ Warning: undoing tag counts of failed upload: %s ------", err)
	}
}

// CreateThumbnailForNew makes a new set's thumbnail from first, its first
// image. Call it after the insert: the name is recorded on the stored set.
func CreateThumbnailForNew(path string, first ImageInfo, title string, id bson.ObjectID) {

	newimg, _, _, err := GenerateLowResFromHigh(path, first.Name, 256, 265)
	if err != nil {
		log.Println(err)
	}
	SaveThumbnailLocal(path, title, newimg, id, first.ID)
}

func testFilePath(BackendVolumeLocation string) error {
	_, err := os.Stat(BackendVolumeLocation)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("File or directory does not exist at: %s\n", BackendVolumeLocation)
			return err
		} else {
			fmt.Printf("Error accessing path %s: %v\n", BackendVolumeLocation, err)
			return err
		}
	}
	return nil

}
