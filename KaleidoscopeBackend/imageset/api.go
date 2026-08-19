package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/authutil"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/png"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// ImageSetErrorResponse maps a GetFromID failure to the HTTP status and
// client-safe message describing it. Unrecognized errors map to 500; the
// underlying error is logged by GetFromID, not returned to the client.
func ImageSetErrorResponse(err error) (int, string) {
	switch {
	case errors.Is(err, bson.ErrInvalidHex):
		return fiber.StatusBadRequest, "invalid image set id"
	case errors.Is(err, mongo.ErrNoDocuments):
		return fiber.StatusNotFound, "image set not found"
	case errors.Is(err, ErrAccessDenied):
		return fiber.StatusForbidden, ErrAccessDenied.Error()
	default:
		return fiber.StatusInternalServerError, "could not retrieve image set"
	}
}

func GetThumbnail(c *fiber.Ctx) error {

	IsetID := c.Query("id")

	if IsetID == "" {
		return c.Status(fiber.StatusBadRequest).SendString("no image set ID provided")
	}

	userID := c.Locals("UserID").(string)
	if userID == "" {
		return c.Status(fiber.StatusInternalServerError).SendString("no user ID provided")
	}

	iset, err := GetFromID(userID, IsetID)
	if err != nil {
		status, msg := ImageSetErrorResponse(err)
		return c.Status(status).SendString(msg)
	}

	//

	//if no thumbnail exist create one
	if iset[0].ThumbNail == "" {
		if len(iset[0].Image) == 0 {
			return c.Status(fiber.StatusNotFound).SendString("no images in image set at this time. Please wait for uploads to complete. If no upload is in progress, there might be a bug.")
		}
		if iset[0].Image[0].Name == "" {
			return c.Status(fiber.StatusInternalServerError).SendString("the image set image link is missing. This is not supposed to happen.")
		}
		img, _, _, err := GenerateLowResFromHigh(iset[0].Path, iset[0].Image[0].Name, 256, 256)
		if err != nil {
			return fmt.Errorf("failed to generate thumbnail: %w", err)
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
		return fmt.Errorf("failed to retrieve thumbnail: %w", err)
	}
	if img == nil {
		return c.Status(fiber.StatusInternalServerError).SendString("something went wrong with thumbnail retrieve")
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
		return c.Status(fiber.StatusBadRequest).SendString("requires an 'ids' param to be sent with the request (eg: ?ids=12345,49325,...)")
	}

	userID := c.Locals("UserID").(string)
	if userID == "" {
		return c.Status(fiber.StatusInternalServerError).SendString("no user ID provided")
	}

	//check if user can access the images and remove any images that would not be valid
	iSets, err := GetFromID(userID, paramid...)
	if err != nil {
		status, msg := ImageSetErrorResponse(err)
		return c.Status(status).SendString(msg)
	}

	//clean response to avoid backend info reaching the front end and create api Json response
	iSets = CleanImagSetForFrontEnd(iSets...)

	res := fiber.Map{
		"image_sets": iSets,
	}

	return c.Status(fiber.StatusOK).JSON(res)

}

func PostImageSet(c *fiber.Ctx) error {

	var imageSet *ImageSetMongo = new(ImageSetMongo)

	userID := c.Locals("UserID").(string)
	if userID == "" {
		return c.Status(fiber.StatusInternalServerError).SendString("no user ID provided")
	}

	if err := c.BodyParser(imageSet); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("could not parse request body: " + err.Error())
	}

	//A id was sent which is invalid
	if imageSet.ID != bson.NilObjectID {
		//TODO : item sent to wrong api
		return c.Status(fiber.StatusBadRequest).SendString("called API to add while trying to update")
	}

	// parse images from api request
	form, err := c.MultipartForm()
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("could not parse multipart form: " + err.Error())
	}

	media := form.File["media"]

	MedSour := make([]MediaSource, len(media))
	for i, m := range media {
		MedSour[i] = MultipartSource{m}
	}

	hashHits, _, err := AddImageSet(imageSet, MedSour, userID)
	if err != nil {
		status := fiber.StatusInternalServerError
		if errors.Is(err, ErrNoMedia) {
			status = fiber.StatusBadRequest
		}
		return c.Status(status).SendString(err.Error())
	}

	//non-empty hashHits means the set was added but duplicate images were detected
	status := fiber.StatusCreated
	if len(hashHits) != 0 {
		status = fiber.StatusAccepted
	}
	return c.Status(status).JSON(fiber.Map{"hash_hits": hashHits})
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
		return c.Status(fiber.StatusBadRequest).SendString("requires an 'ids' param to be sent with the request (eg: ?ids=12345,49325,...)")
	}

	userID := c.Locals("UserID").(string)
	if userID == "" {
		return c.Status(fiber.StatusInternalServerError).SendString("no user ID provided")
	}

	var UnauthorizedImageIDs []bson.ObjectID

	//If user is not admin check for authority to do deletions to avoid users trying to delete other peoples images
	if !authutil.IsAdmin(userID) {
		//check if user can access the images and remove any images that would not be valid
		iSets, err := GetFromID(userID, paramid...)
		if err != nil {
			status, msg := ImageSetErrorResponse(err)
			return c.Status(status).SendString(msg)
		}
		if len(iSets) != len(paramid) {
			return c.Status(fiber.StatusInternalServerError).SendString("something has gone wrong with getting image sets from the IDs")
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
		return c.Status(fiber.StatusPartialContent).JSON(res)
	}

	if DeletedList == nil {
		return c.Status(fiber.StatusNotFound).JSON(res)
	}

	return c.Status(fiber.StatusOK).JSON(res)
}

func GetImageInfo(c *fiber.Ctx) error {

	var requestParams struct {
		IDs []string `json:"ids" bson:"ids" form:"ids" query:"ids"`
	}
	err := c.QueryParser(&requestParams)
	fmt.Println(requestParams.IDs)
	if len(requestParams.IDs) == 0 || err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("no id given")
	}

	var objectIDs []bson.ObjectID
	for _, idStr := range requestParams.IDs {
		oid, err := bson.ObjectIDFromHex(idStr)
		if err != nil {
			status, msg := ImageSetErrorResponse(fmt.Errorf("%w: %q", bson.ErrInvalidHex, idStr))
			return c.Status(status).SendString(msg)
		}
		objectIDs = append(objectIDs, oid)
	}

	userID := c.Locals("UserID").(string)
	if userID == "" {
		return c.Status(fiber.StatusInternalServerError).SendString("no user ID provided")
	}

	result, err := GetImageInfoFromDB(objectIDs, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("an error occurd in the query: " + err.Error())
	}
	res := fiber.Map{
		"imagesets": result,
	}
	return c.JSON(res)
}

func FilterForImageSets(c *fiber.Ctx) error {
	var requestParams SearchParams
	err := c.BodyParser(&requestParams)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("could not parse request body: " + err.Error())
	}

	userID := c.Locals("UserID").(string)
	if userID == "" {
		return c.Status(fiber.StatusInternalServerError).SendString("no user ID provided")
	}

	requestParams.User = userID

	// fmt.Printf("tags: %s, authors %s\n", fmt.Sprintf("%s", requestParams.Tags), fmt.Sprintf("%s", requestParams.Author))

	result, err := SearchDBForImages(requestParams)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("an error occurd in the query: " + err.Error())
	}
	res := result

	return c.JSON(res)
}

/*
Will take in ONE imagset ID ('image_set_id') and one Index (index) of the image to provide.

	WARNING: this code assumes that the token has already been validated before running the function
	Returns an array of images in the 'images' field
*/
func GetImageFromID(c *fiber.Ctx) error {

	userID := c.Locals("UserID").(string)
	if userID == "" {
		return c.Status(fiber.StatusInternalServerError).SendString("no user ID provided")
	}

	var requestParams struct {
		ImageSetId string `json:"image_set_id" form:"image_set_id" query:"image_set_id"`
		IndexList  int    `json:"index" form:"index" query:"index"`
		LowRes     bool   `json:"lowres" form:"lowres" query:"lowres"`
	}

	err := c.QueryParser(&requestParams)

	log.Println(requestParams)

	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("could not parse request " + err.Error())
	}
	if requestParams.ImageSetId == "" {
		return c.Status(fiber.StatusBadRequest).SendString("no image set ID provided")
	}

	//user is validated in request
	iset, err := GetFromID(userID, requestParams.ImageSetId)
	if err != nil {
		status, msg := ImageSetErrorResponse(err)
		return c.Status(status).SendString(msg)
	}

	if requestParams.IndexList >= len(iset[0].Image) || requestParams.IndexList < 0 {
		if len(iset[0].Image) == 0 {
			return c.Status(fiber.StatusNotFound).SendString("the imageSet does not contain images. If this was recently uploaded wait for it to be processed")
		}
		return c.Status(fiber.StatusBadRequest).SendString("index out of bounds")
	}

	var imageLink string

	var retImage image.Image
	var retGif *gif.GIF

	if requestParams.LowRes {

		imageLink = iset[0].Image[requestParams.IndexList].LowResName
		log.Println("res link: " + imageLink)
		if imageLink == "" || imageLink == " " {
			retImage, _, _, err = GenerateLowResFromHigh(iset[0].Path, iset[0].Image[requestParams.IndexList].Name, 720, 0)

			if err != nil {
				return c.Status(fiber.StatusInternalServerError).SendString("failed to create low res image: " + err.Error())
			}
			//todo save image
			go AddLowresToSetAndStorage(iset[0].Path, iset[0].Title, retImage, iset[0], requestParams.IndexList)

		} else {
			retImage, retGif, err = RetrieveLocalImage(iset[0].Path, imageLink, true)
			if err != nil {
				return fmt.Errorf("could not retrieve low res: %w", err)
			}
		}

	} else {
		retImage, retGif, err = RetrieveLocalImage(iset[0].Path, iset[0].Image[requestParams.IndexList].Name, false)
		if err != nil {
			return fmt.Errorf("could not retrieve image: %w", err)
		}
	}

	if retImage != nil {
		c.Type("png")
		return png.Encode(c.Response().BodyWriter(), retImage)
	} else if retGif != nil {
		c.Type("gif")
		return gif.EncodeAll(c.Response().BodyWriter(), retGif)
	}

	return nil
}
