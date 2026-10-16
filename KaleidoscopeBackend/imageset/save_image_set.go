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
	// never been recorded before, so every fetched tag counts as new. Call
	// with LockUserSets held: it also writes other sets' translations.
	ProcessSourceTags(userID string, set *ImageSetMongo, sourceIdx int, fetched []SourceTag) (TagCountChange, error)
	// RecordDeletion undoes usage counts recorded when the now-deleted set
	// was created or synced: source tags from sources, AutoTags from tags.
	RecordDeletion(userID string, sources []SourceInfo, tags []string) error
	// RecomputeSystemTags reconciles set's computed tags (e.g. Lost Media,
	// Untracked) against its current Sources, mutating AutoTags/Tags in
	// place. Does not persist set - callers own the eventual save.
	RecomputeSystemTags(userID string, set *ImageSetMongo) (TagCountChange, error)
	// UndoTagCounts reverses the counts in change, for a caller whose write
	// of the set failed after ProcessSourceTags or RecomputeSystemTags.
	UndoTagCounts(userID string, change TagCountChange) error
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

// TagCountChange is what one ProcessSourceTags or RecomputeSystemTags call
// counted, so it can be reversed with Tagger.UndoTagCounts.
type TagCountChange struct {
	SourceName string
	SourceTags []SourceTag //usage recorded for SourceName
	TagsBefore []string
	TagsAfter  []string //the set's Tags as of the last count change that landed
}

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

// ProcessImageSet imports files with set's one source (nothing attributed yet)
// into userID's library and returns the import report's items, ref naming the
// import in them. A duplicate, addition or less of stored sets is merged into
// each; otherwise it becomes a new set, with a suggested merge when it only
// partly or similarly matches stored sets. Failures are items carrying a
// notification reason. Everything after decoding holds LockUserSets.
func ProcessImageSet(set *ImageSetMongo, files []MediaSource, userID string, ref string) []notification.ItemResult {

	if len(files) == 0 {
		return []notification.ItemResult{notification.FailedItem(userID, ref, bson.NilObjectID, fmt.Errorf("%w: %w", notification.ErrSaveFailed, ErrNoMedia))}
	}

	if len(set.Sources) != 1 || len(set.Sources[0].AttributedTo) != 0 {
		err := fmt.Errorf("%w: an import needs exactly one unattributed source, got %d", notification.ErrSaveFailed, len(set.Sources))
		return []notification.ItemResult{notification.FailedItem(userID, ref, bson.NilObjectID, err)}
	}

	prepared, err := prepareImages(files)
	if err != nil {
		return []notification.ItemResult{notification.FailedItem(userID, ref, bson.NilObjectID, fmt.Errorf("%w: %w", notification.ErrSaveFailed, err))}
	}

	defer LockUserSets(userID)()

	matches, err := findImageMatches(userID, prepared)
	if err != nil {
		return []notification.ItemResult{notification.FailedItem(userID, ref, bson.NilObjectID, fmt.Errorf("%w: %w", notification.ErrLibraryAccess, err))}
	}

	candidateSets, err := loadOwnedSets(userID, findCandidates(matches, len(prepared)))
	if err != nil {
		return []notification.ItemResult{notification.FailedItem(userID, ref, bson.NilObjectID, fmt.Errorf("%w: %w", notification.ErrLibraryAccess, err))}
	}

	candidates := make([]candidateMatch, len(candidateSets))
	for i := range candidateSets {
		candidates[i] = classifyCandidate(&candidateSets[i], prepared, matches)
	}

	var results []notification.ItemResult
	var newSetID bson.ObjectID
	var changedTargets, written []bson.ObjectID
	var suggested []candidateMatch

	switch chooseOutcome(candidates) {
	case outcomeMerge:
		for _, candidate := range candidates {

			if candidate.scenario == scenarioPartial || candidate.scenario == scenarioSimilar {
				continue //left for the full-set check
			}

			item, changedForLookup := mergeIntoSet(userID, ref, set, prepared, candidate)
			results = append(results, item)
			written = append(written, candidate.set.ID)
			if changedForLookup {
				changedTargets = append(changedTargets, candidate.set.ID)
			}
		}

	case outcomeSuggest:
		suggested = candidates
		fallthrough

	case outcomeNew:
		set.Sources[0].AttributedTo = make([]bson.ObjectID, len(prepared))
		for i, preparedImage := range prepared {
			set.Sources[0].AttributedTo[i] = preparedImage.Info.ID
		}

		if _, err := addImageSet(set, prepared, userID); err != nil {
			return []notification.ItemResult{notification.FailedItem(userID, ref, bson.NilObjectID, fmt.Errorf("%w: %w", notification.ErrSaveFailed, err))}
		}
		newSetID = set.ID
		results = append(results, notification.ItemResult{Kind: notification.ItemAdded, Ref: ref, SetID: newSetID})
	}

	return append(results, suggestMerges(userID, ref, set.Sources[0], newSetID, changedTargets, written, suggested)...)
}

// PreparedImage is an incoming file decoded once to hash and measure it. Its
// pixels aren't kept: addImageSet decodes the file again to save it.
type PreparedImage struct {
	Info  ImageInfo //ID, hashes and size; addImageSet adds the file name
	Media MediaSource
	IsGif bool
	Plain bool //a plain page (see isPlain), which never makes a stored set a merge candidate
}

// prepareImages gives each file a new image ID and decodes it once to hash
// and measure it, so matching can run before anything is written. A file
// that can't be read fails the whole list.
func prepareImages(files []MediaSource) ([]PreparedImage, error) {
	prepared := make([]PreparedImage, 0, len(files))
	for _, file := range files {
		itype, err := getFileTypeFromHeader(file)
		if err != nil {
			return nil, fmt.Errorf("reading media type of %s: %w", file.Name(), err)
		}
		preparedImage := PreparedImage{Info: ImageInfo{ID: bson.NewObjectID()}, Media: file, IsGif: itype == "gif"}

		if preparedImage.IsGif {
			decodedGif, err := FileHeaderToGif(file)
			if err != nil {
				return nil, fmt.Errorf("decoding gif %s: %w", file.Name(), err)
			}
			if preparedImage.Info.ImageHashes, err = HashGif(decodedGif); err != nil {
				return nil, fmt.Errorf("hashing gif %s: %w", file.Name(), err)
			}
			preparedImage.Info.Width, preparedImage.Info.Height = decodedGif.Config.Width, decodedGif.Config.Height
		} else {
			decoded, _, err := FileHeaderToImage(file)
			if err != nil {
				return nil, fmt.Errorf("decoding image %s: %w", file.Name(), err)
			}
			if preparedImage.Info.ImageHashes, err = HashImage(decoded); err != nil {
				return nil, fmt.Errorf("hashing image %s: %w", file.Name(), err)
			}
			preparedImage.Info.Width, preparedImage.Info.Height = decoded.Bounds().Dx(), decoded.Bounds().Dy()
		}

		if preparedImage.Plain, err = isPlain(preparedImage.Info.PDQ); err != nil {
			return nil, fmt.Errorf("checking %s for a plain page: %w", file.Name(), err)
		}

		prepared = append(prepared, preparedImage)
	}
	return prepared, nil
}

// unknownAuthor stands in for a set's author until a source provides one.
const unknownAuthor = "unknown"

// ErrNoMedia is the failure for an import with no media files: ProcessImageSet
// reports it as a failed item, and PostImageSet answers HTTP 400 with it.
var ErrNoMedia = errors.New("no media attached")

// addImageSet saves each image (from prepareImages) as a file named by its ID,
// then inserts imageSet and returns its ID as hex. Attribution is checked
// before anything is written, and a later failure undoes its own partial writes.
func addImageSet(imageSet *ImageSetMongo, images []PreparedImage, userId string) (string, error) {

	//TODO: Test if image size is to large

	//built from the prepared images only, so no client-sent path survives; names are set as each file is saved
	imageSet.Image = make([]ImageInfo, len(images))
	for i, preparedImage := range images {
		imageSet.Image[i] = preparedImage.Info
		imageSet.Image[i].IsImageActive = true
	}
	if err := imageSet.checkAttribution(); err != nil {
		return "", err
	}
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
		//the returned change is ignored: undoNewImageSet removes every count this set recorded
		if _, err := Tagger.ProcessSourceTags(userId, imageSet, idx, fetched[idx]); err != nil {
			return "", fmt.Errorf("processing source tags: %w", err)
		}
	}
	if len(imageSet.Sources) == 0 {
		// ProcessSourceTags never ran, and it is what normally recomputes system tags
		if _, err := Tagger.RecomputeSystemTags(userId, imageSet); err != nil {
			return "", fmt.Errorf("computing system tags: %w", err)
		}
	}

	toWrite := make(map[bson.ObjectID]PreparedImage, len(images))
	for _, preparedImage := range images {
		toWrite[preparedImage.Info.ID] = preparedImage
	}
	if err := writePreparedImages(imageSet, toWrite); err != nil {
		return "", err
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

// writePreparedImages saves each image in toWrite (keyed by image ID) into
// set's folder, decoding it again since prepareImages keeps no pixels, and
// records its file name on that ID's entry in set.Image. A failure leaves the
// files already written named in set.Image, for the caller's cleanup.
func writePreparedImages(set *ImageSetMongo, toWrite map[bson.ObjectID]PreparedImage) error {

	for i := range set.Image {

		preparedImage, ok := toWrite[set.Image[i].ID]
		if !ok {
			continue
		}

		var fileName string
		var err error
		if preparedImage.IsGif {

			var decodedGif *gif.GIF
			if decodedGif, err = FileHeaderToGif(preparedImage.Media); err != nil {
				return fmt.Errorf("decoding gif %s: %w", preparedImage.Media.Name(), err)
			}
			fileName, err = SaveGif(decodedGif, set.Path, set.Title, set.Image[i].ID)

		} else {

			var decoded image.Image
			if decoded, _, err = FileHeaderToImage(preparedImage.Media); err != nil {
				return fmt.Errorf("decoding image %s: %w", preparedImage.Media.Name(), err)
			}
			fileName, err = SaveImage(decoded, set.Path, set.Title, set.Image[i].ID, "png")
		}

		if err != nil {
			return fmt.Errorf("saving media: %w", err)
		}
		set.Image[i].Name = fileName
	}
	return nil
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
		return //GetThumbnail regenerates it on first view
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
