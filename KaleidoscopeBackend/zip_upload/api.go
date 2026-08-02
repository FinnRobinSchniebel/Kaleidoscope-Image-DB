package zipupload

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
)

func UploadZip(c *fiber.Ctx) error {

	//Get the zip
	fileHeader, err := c.FormFile("zipFile")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("No File Sent")
	}

	//create form for array grouping
	form, err := c.MultipartForm()
	if err != nil {
		return fiber.ErrBadRequest
	}
	defer form.RemoveAll()

	//Combine all rules for files and zips for easier use
	var ruleLayers []string

	if v := form.Value["structureZip"]; len(v) > 0 {
		ruleLayers = append(ruleLayers, v...)
	}

	if v := form.Value["folders"]; len(v) > 0 {
		ruleLayers = append(ruleLayers, v...)
	}

	for i := range ruleLayers {
		if ruleLayers[i] == "NAN" {
			ruleLayers[i] = ""
		}
	}

	//keep file rules separate and give a default if no instructions are given.
	fileLayer := "[order]"
	if v := form.Value["files"]; len(v) > 0 && v[0] != "" {
		fileLayer = v[0]
	}

	//grouping index
	GroupingIndex, err := strconv.Atoi(c.FormValue("GroupingLevel", "0"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid grouping index")
	}
	userID := c.Locals("UserID").(string)
	if userID == "" {
		return c.Status(500).SendString("No user ID provided")
	}

	code, hashHits, skip, errors, err := ProcessZip(fileHeader, ruleLayers, fileLayer, GroupingIndex, userID)

	if err != nil {
		return c.Status(code).SendString(err.Error())
	}

	return c.Status(code).JSON(
		fiber.Map{
			"hash_hits": hashHits,
			"skipped":   skip,
			"errors":    errors,
		},
	)
}
