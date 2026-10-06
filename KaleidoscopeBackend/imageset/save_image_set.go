package imageset

import (
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

// unknownAuthor stands in for a set's author until a source provides one.
const unknownAuthor = "unknown"

// ErrNoMedia is returned by AddImageSet when the request contained no media
// files. Callers map it to HTTP 400.
var ErrNoMedia = errors.New("no media attached")

// This function adds the created image set to the DataBase and adds the mediaSource as permanent files to the server.
// On success it returns a nil error; a non-empty CollisionMap means the set was
// saved but duplicate image hashes were detected.
func AddImageSet(imageSet *ImageSetMongo, media []MediaSource, userId string) (CollisionMap, string, error) {

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
	if len(media) == 0 {
		return nil, "", ErrNoMedia
	}

	var err error

	/**		Test FilePath	 **/
	err = testFilePath(BackendVolumeLocation)
	//determine folder path for images and add the path to the imagset before first insert

	imageSet.Path, err = MakeFileDirectoryFromAuthor(userId, imageSet.Authors[0])

	if err != nil {
		return nil, "", fmt.Errorf("creating author directory: %w", err)
	}

	// The insert below is the commit point. Until then nothing is stored, so a
	// failure only has to undo the files and counts recorded in imageSet.
	committed := false
	defer func() {
		if !committed {
			undoNewImageSet(imageSet)
		}
	}()

	// Cleared up front so imageSet only ever holds tags ProcessSourceTags has recorded.
	fetched := make([][]SourceTag, len(imageSet.Sources))
	for idx := range imageSet.Sources {
		fetched[idx] = imageSet.Sources[idx].Tags
		imageSet.Sources[idx].Tags = nil
	}
	for idx := range imageSet.Sources {
		if err := Tagger.ProcessSourceTags(userId, imageSet, idx, fetched[idx]); err != nil {
			return nil, "", fmt.Errorf("processing source tags: %w", err)
		}
	}
	if len(imageSet.Sources) == 0 {
		// ProcessSourceTags never ran, and it is what normally recomputes system tags
		if err := Tagger.RecomputeSystemTags(userId, imageSet); err != nil {
			return nil, "", fmt.Errorf("computing system tags: %w", err)
		}
	}

	hashHits := make(CollisionMap)

	for index := range media {

		//fmt.Println(media[index].Name(), media[index].Size(), media[index].ContentType())

		/**		save media		**/
		fileName := imageSet.Title

		//Need to know the file type to save it in the correct format
		itype, err := getFileTypeFromHeader(media[index])
		if err != nil {

			return nil, "", fmt.Errorf("reading media type: %w", err)
		}

		var ihash string

		//gifs must be handled differently
		if itype == "gif" {
			var igif *gif.GIF
			igif, err = FileHeaderToGif(media[index])
			if err != nil {
				return nil, "", fmt.Errorf("decoding gif: %w", err)
			}
			fileName, ihash, err = SaveGif(igif, imageSet.Path, fileName, imageSet.ID, index)

		} else {
			var inImage image.Image
			inImage, _, err = FileHeaderToImage(media[index])
			if err != nil {
				return nil, "", fmt.Errorf("decoding image: %w", err)
			}
			fileName, ihash, err = SaveImage(inImage, imageSet.Path, fileName, imageSet.ID, index, "png")
		}

		if err != nil {
			return nil, "", fmt.Errorf("saving media: %w", err)
		}

		imageSet.Image = append(imageSet.Image, ImageInfo{Name: fileName, ImageHash: ihash, IsImageActive: true})

		//compare hash in DB

		HitResults, err := findOverlappingHashes(ihash, userId)

		if err != nil {
			return nil, "", fmt.Errorf("checking hash collisions: %w", err)
		}
		if len(HitResults) != 0 {
			fmt.Println("Hash Hit")
			hashHits[index] = HitResults
		}

	}

	log.Print("Files Uploaded")

	imageSet.DateAdded = time.Now()
	if _, err := Collection.InsertOne(context.Background(), imageSet); err != nil {
		return nil, "", fmt.Errorf("inserting image set: %w", err)
	}
	committed = true
	refreshUntagged(userId)

	// After the insert: it writes the thumbnail name to the stored set, and only logs on failure.
	//Note: could be go functioned but that may create a race condition if image is viewed before the save finishes
	CreateThumbnailForNew(imageSet.Path, imageSet.Image[0].Name, imageSet.Title, imageSet.ID)

	log.Println("---Upload complete---")
	//non-empty hashHits signals a successful add with duplicate images detected
	if len(hashHits) != 0 {
		return hashHits, imageSet.ID.Hex(), nil
	}

	return nil, imageSet.ID.Hex(), nil
}

// undoNewImageSet reverses a failed AddImageSet that was never stored: the
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

func CreateThumbnailForNew(path string, existingFileName string, title string, id bson.ObjectID) {

	newimg, _, _, err := GenerateLowResFromHigh(path, existingFileName, 256, 265)
	if err != nil {
		log.Println(err)
	}
	SaveThumbnailLocal(path, title, newimg, id, 0)
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
