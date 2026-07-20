package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/authutil"
	"errors"
	"image/png"
	"log"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func GetThumbnail(c *fiber.Ctx) error {

	IsetID := c.Query("id")

	if IsetID == "" {
		return c.Status(fiber.StatusBadRequest).SendString("no image set ID provided")
	}

	userID := c.Locals("UserID").(string)
	if userID == "" {
		return c.Status(500).SendString("No user ID provided")
	}

	iset, err := GetFromID(userID, IsetID)
	if err != nil || len(iset) == 0 {
		return c.Status(http.StatusNotFound).SendString("imageSet could not be found: " + err.Error())
	}

	//

	//if no thumbnail exist create one
	if iset[0].ThumbNail == "" {
		if len(iset[0].Image) == 0 {
			return c.Status(fiber.StatusNotFound).SendString("No Images in Image set at this time. Please wait for uploads to complete. If no Upload is in progress, there might be a bug.")
		}
		if iset[0].Image[0].Name == "" {
			return c.Status(fiber.StatusInternalServerError).SendString("The image set image link is missing. This is not supposed to happen.")
		}
		img, _, _, err := GenerateLowResFromHigh(iset[0].Path, iset[0].Image[0].Name, 256, 256)
		if err != nil {
			return err
		}

		//save async
		go SaveThumbnailLocal(iset[0].Path, iset[0].Title, img, iset[0].ID, 0)

		//TODO: Change to webP
		c.Type("png")
		return png.Encode(c.Response().BodyWriter(), img)

	}

	//thumbnail is always considered low res
	img, _, err := RetrieveLocalImage(iset[0].Path, iset[0].ThumbNail, true)
	if err != nil {
		return err
	}
	if img == nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Something went wrong with thumbnail retrieve.")
	}
	c.Type("png")
	return png.Encode(c.Response().BodyWriter(), img)

}

// This api Call is to get info about the Image.
// It does not provide the image itself.
func GetImageSetById(c *fiber.Ctx) error {
	//get the ids from the api
	paramIdRaw := c.Context().QueryArgs().PeekMulti("ids")

	var paramid []string
	for _, groupedIds := range paramIdRaw {
		paramid = append(paramid, strings.Split(string(groupedIds), ",")...)
	}
	if paramid == nil {
		return c.Status(400).SendString("Requires an 'ids' param to be sent with the request (eg: ?ids=12345,49325,...)")
	}

	userID := c.Locals("UserID").(string)
	if userID == "" {
		return c.Status(500).SendString("No user ID provided")
	}

	//check if user can access the images and remove any images that would not be valid
	iSets, err := GetFromID(userID, paramid...)
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString("could not get imageset from the request: " + err.Error())
	}

	//clean response to avoid backend info reaching the front end and create api Json response
	iSets = CleanImagSetForFrontEnd(iSets...)

	res := fiber.Map{
		"image_sets": iSets,
	}

	if err != nil {
		log.Println("Could Not fetch Items from DB")
		return err
	}

	return c.Status(200).JSON(res)

}
func PostImageSet(c *fiber.Ctx) error {

	var imageSet *ImageSetMongo = new(ImageSetMongo)

	userID := c.Locals("UserID").(string)
	if userID == "" {
		return c.Status(500).SendString("No user ID provided")
	}

	if err := c.BodyParser(imageSet); err != nil {
		return err
	}

	//A id was sent which is invalid
	if imageSet.ID != bson.NilObjectID {
		//TODO : item sent to wrong api
		return c.Status(400).SendString("Called API to add while trying to update.")
	}

	// parse images from api request
	form, err := c.MultipartForm()
	if err != nil {
		return err
	}

	media := form.File["media"]

	MedSour := make([]MediaSource, len(media))
	for i, m := range media {
		MedSour[i] = MultipartSource{m}
	}

	hashHits, _, response := AddImageSet(imageSet, MedSour, userID)

	return c.Status(response.ErrorCode).JSON(fiber.Map{"error": response.ErrorString, "hash_hits": hashHits})
}

// takes in one or multiple "ids" in a coma separated list (no spaces)
// returns a list of Ids that were deleted.
func DeleteImageSets(c *fiber.Ctx) error {

	//get all params of type 'ids' and split the param by delimiter "," to get a list of all ids to be deleted
	paramIdRaw := c.Context().QueryArgs().PeekMulti("ids")

	var paramid []string
	for _, groupedIds := range paramIdRaw {
		paramid = append(paramid, strings.Split(string(groupedIds), ",")...)
	}

	log.Println("List of Items to delete:\n" + strings.Join(paramid, ", "))

	if paramid == nil {
		return c.Status(400).SendString("Requires an 'ids' param to be sent with the request (eg: ?ids=12345,49325,...)")
	}

	userID := c.Locals("UserID").(string)
	if userID == "" {
		return c.Status(500).SendString("No user ID provided")
	}

	var UnauthorizedImageIDs []bson.ObjectID

	//If user is not admin check for authority to do deletions to avoid users trying to delete other peoples images
	if !authutil.IsAdmin(userID) {
		//check if user can access the images and remove any images that would not be valid
		iSets, err := GetFromID(userID, paramid...)
		if err != nil {
			return c.Status(http.StatusInternalServerError).SendString("Could not get ID from the Request")
		}
		if len(iSets) != len(paramid) {
			return c.Status(500).SendString("something has gone wrong with getting image sets from the IDs")
		}

		for index := range iSets {
			if iSets[index].KscopeUserId != userID {
				UnauthorizedImageIDs = append(UnauthorizedImageIDs, iSets[index].ID)
				//Must remove unauthorized items to avoid deletion during next step
				paramid = append(paramid[:index], paramid[(index+1):]...)
			}
		}
	}

	var DeletedList []string

	var errList error
	for _, id := range paramid {

		ObjId, err := bson.ObjectIDFromHex(id)
		if err != nil {
			errList = errors.Join(errList, err)

			continue
		}

		err = DeleteImageSetInDB(ObjId)
		if err != nil {
			errList = errors.Join(errList, err)
			continue
		}
		DeletedList = append(DeletedList, id)
	}

	var errorText string
	if errList != nil {
		errorText = errList.Error()
	}

	res := fiber.Map{
		"deleted":      DeletedList,
		"unauthorized": UnauthorizedImageIDs,
		"errors":       errorText,
	}

	if DeletedList != nil && (errList != nil || UnauthorizedImageIDs != nil) {
		return c.Status(http.StatusPartialContent).JSON(res)
	}

	if DeletedList == nil {
		return c.Status(404).JSON(res)
	}

	return c.Status(200).JSON(res)
}
