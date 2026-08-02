package tagging

import (
	"fmt"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/bson"
)

/*
Returns an array of images in the 'images' field
*/
func AddTag(c *fiber.Ctx) error {

	var inputs Tag

	c.BodyParser(&inputs)

	userID := c.Locals("UserID").(string)
	if userID == "" {
		return c.Status(500).SendString("No user ID provided")
	}

	var err error

	inputs.User, err = bson.ObjectIDFromHex(userID)
	if err != nil {
		return err
	}

	err = AddTags(inputs)

	if err != nil {
		return err
	}
	return c.SendStatus(200)
}

// TODO
func TagRetrieve(c *fiber.Ctx) error {

	var requestParams struct {
		IDs []string `json:"ids" bson:"ids" form:"ids" query:"ids"`
	}
	err := c.QueryParser(&requestParams)
	fmt.Println(requestParams.IDs)
	if len(requestParams.IDs) == 0 || err != nil {
		return c.Status(http.StatusBadRequest).SendString("no id given")
	}

	var objectIDs []bson.ObjectID
	for _, idStr := range requestParams.IDs {
		oid, err := bson.ObjectIDFromHex(idStr)
		if err != nil {
			return err
		}
		objectIDs = append(objectIDs, oid)
	}

	// fmt.Printf("tags: %s, authors %s\n", fmt.Sprintf("%s", requestParams.Tags), fmt.Sprintf("%s", requestParams.Author))

	// result, err := imageset.GetImageInfoFromDB(objectIDs)
	// if err != nil {
	// 	return c.Status(http.StatusInternalServerError).SendString("an error occurd in the query: " + err.Error())
	// }
	// res := fiber.Map{
	// 	"imagesets": result,
	// }
	// return c.JSON(res)
	return nil
}

func Testautotag(c *fiber.Ctx) error {

	var items struct {
		Tags []string `json:"tags" bson:"tags" form:"tags"`
	}

	err := c.BodyParser(&items)
	if err != nil {
		return err
	}
	if len(items.Tags) == 0 {
		return c.Status(http.StatusBadRequest).SendString("no tags given")
	}

	res, err := FindAutoTag(items.Tags)
	if err != nil {
		return err
	}

	return c.JSON(res)

}
