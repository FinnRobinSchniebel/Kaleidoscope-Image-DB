package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/tagging"
	"context"
	"fmt"
	"image"
	"image/gif"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

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

// This function adds the created image set to the DataBase and adds the mediaSource as permanent files to the server.
func AddImageSet(imageSet *ImageSetMongo, media []MediaSource, userId string) (CollisionMap, string, InternalResponse) {

	//TODO: Test if image size is to large

	//clean file paths to avoid unauthorized access
	imageSet.Image = nil

	imageSet.KscopeUserId = ""

	//set the author in case of none given to avoid issues with file path creation
	if len(imageSet.Authors) == 0 || (imageSet.Authors[0] == "") {
		imageSet.Authors = []string{"unknown"}
	}
	//add userId (done as seperate step to avoid exploits if changes are made)
	imageSet.KscopeUserId = userId

	//derive main tag list suggestions from each source's own tags, merging into
	//whatever tags were already set without duplicating any
	for _, src := range imageSet.Sources {
		for _, t := range tagging.AutoTag(userId, src.Name, src.Tags) {
			if !slices.Contains(imageSet.Tags, t) {
				imageSet.Tags = append(imageSet.Tags, t)
			}
		}
	}

	//check media count first to avoid empty imagsets in db
	if len(media) == 0 {
		return nil, "", InternalResponse{400, "No Media attached"}
	}

	var err error

	/**		Test FilePath	 **/
	err = testFilePath(BackendVolumeLocation)
	//determine folder path for images and add the path to the imagset before first insert

	imageSet.Path, err = MakeFileDirectoryFromAuthor(userId, imageSet.Authors[0])

	if err != nil {
		return nil, "", InternalResponse{500, err.Error()}
	}

	imageSet.DateAdded = time.Now()

	//add to DB
	insertResult, err := Collection.InsertOne(context.Background(), imageSet)

	CreatedSuccessfully := false

	if err != nil {
		return nil, "", InternalResponse{500, err.Error()}
	}
	//In case the creation fails, remove the entry to avoid empty data
	defer func() {
		if CreatedSuccessfully {
			return
		}
		err = DeleteImageSetInDB(insertResult.InsertedID.(bson.ObjectID))
		if err != nil {
			log.Printf("------ Warning: %s ------", err.Error())
		}
	}()

	imageSet.ID = insertResult.InsertedID.(bson.ObjectID)

	hashHits := make(CollisionMap)

	for index := range media {

		//fmt.Println(media[index].Name(), media[index].Size(), media[index].ContentType())

		/**		save media		**/
		fileName := imageSet.Title

		//Need to know the file type to save it in the correct format
		itype, err := getFileTypeFromHeader(media[index])
		if err != nil {

			return nil, "", InternalResponse{500, err.Error()}
		}

		var ihash string

		//gifs must be handled differently
		if itype == "gif" {
			var igif *gif.GIF
			igif, err = FileHeaderToGif(media[index])
			if err != nil {
				return nil, "", InternalResponse{500, err.Error()}
			}
			fileName, ihash, err = SaveGif(igif, imageSet.Path, fileName, imageSet.ID, index)

		} else {
			var inImage image.Image
			inImage, _, err = FileHeaderToImage(media[index])
			if err != nil {
				return nil, "", InternalResponse{500, err.Error()}
			}
			fileName, ihash, err = SaveImage(inImage, imageSet.Path, fileName, imageSet.ID, index, "png")
		}

		if err != nil {
			return nil, "", InternalResponse{500, err.Error()}
		}

		imageSet.Image = append(imageSet.Image, ImageInfo{Name: fileName, ImageHash: ihash, IsImageActive: true})

		//compare hash in DB

		HitResults, err := findOverlappingHashes(ihash, userId)

		if err != nil {
			return nil, "", InternalResponse{500, err.Error()}
		}
		if len(HitResults) != 0 {
			fmt.Println("Hash Hit")
			hashHits[index] = HitResults
		}

	}

	//Note: could be go functioned but that may create a race condition if image is viewed before the save finishes
	CreateThumbnailForNew(imageSet.Path, imageSet.Image[0].Name, imageSet.Title, imageSet.ID)

	log.Print("Files Uploaded")

	update := bson.M{"$set": imageSet}
	result, err := Collection.UpdateByID(context.Background(), imageSet.ID, update)

	if err != nil {
		fmt.Println("Update Failed")
		return nil, "", InternalResponse{500, err.Error()}
	}

	if result.MatchedCount == 0 {
		log.Print("COULD NOT UPDATE DB FILE AFTER ADDING INFO")
		return nil, "", InternalResponse{500, "Error while updating db entry after saving files"}
		//return c.Status(500).SendString()
	}
	CreatedSuccessfully = true

	log.Println("---Upload complete---")
	//hash conflict detected
	if len(hashHits) != 0 {
		//return InternalErrorHandle{202, "Error while updating db entry after saving files"}
		return hashHits, imageSet.ID.Hex(), InternalResponse{202, "Ok, Hash collision detected"}
	}

	return nil, imageSet.ID.Hex(), InternalResponse{201, "Ok, Added to DB"}
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
