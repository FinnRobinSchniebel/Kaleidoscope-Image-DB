package imageset

import (
	"image/png"
	"net/http"

	"github.com/gofiber/fiber/v2"
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
		go SaveThumbNailLocal(iset[0].Path, iset[0].Image[0].Name, img, iset[0].ID, 0)

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
