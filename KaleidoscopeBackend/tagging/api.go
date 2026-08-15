package tagging

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func userIDFromLocals(c *fiber.Ctx) (bson.ObjectID, error) {
	userID, _ := c.Locals("UserID").(string)
	if userID == "" {
		return bson.ObjectID{}, fmt.Errorf("no user id in context")
	}
	return bson.ObjectIDFromHex(userID)
}

// GET /api/sourcetags/search?source=&lang=&q=
func SearchSourceTagsHandler(c *fiber.Ctx) error {
	userID, err := userIDFromLocals(c)
	if err != nil {
		return c.Status(http.StatusUnauthorized).SendString(err.Error())
	}
	q := c.Query("q")
	if q == "" {
		return c.Status(http.StatusBadRequest).SendString("q is required")
	}
	results, err := SearchSourceTags(userID, c.Query("source"), c.Query("lang"), q, 20)
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString(err.Error())
	}
	return c.JSON(results)
}

// GET /api/sourcetags?source=&cursor=&limit=
func ListSourceTagsHandler(c *fiber.Ctx) error {
	userID, err := userIDFromLocals(c)
	if err != nil {
		return c.Status(http.StatusUnauthorized).SendString(err.Error())
	}
	results, err := ListSourceTags(userID, c.Query("source"), c.Query("cursor"), c.QueryInt("limit", 50))
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString(err.Error())
	}
	return c.JSON(results)
}

// GET /api/autotags?q=
func ListAutoTagsHandler(c *fiber.Ctx) error {
	userID, err := userIDFromLocals(c)
	if err != nil {
		return c.Status(http.StatusUnauthorized).SendString(err.Error())
	}
	results, err := ListAutoTags(userID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString(err.Error())
	}
	if q := c.Query("q"); q != "" {
		results = filterAutoTagsByPrefix(results, q)
	}
	return c.JSON(results)
}

func filterAutoTagsByPrefix(tags []AutoTagWithMatches, prefix string) []AutoTagWithMatches {
	prefix = strings.ToLower(prefix)
	filtered := make([]AutoTagWithMatches, 0, len(tags))
	for _, t := range tags {
		if strings.HasPrefix(strings.ToLower(t.Name), prefix) {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

type createAutoTagRequest struct {
	Name    string   `json:"name"`
	Matches []string `json:"matches"`
}

// POST /api/autotags
func CreateAutoTagHandler(c *fiber.Ctx) error {
	userID, err := userIDFromLocals(c)
	if err != nil {
		return c.Status(http.StatusUnauthorized).SendString(err.Error())
	}
	var body createAutoTagRequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).SendString(err.Error())
	}
	if body.Name == "" {
		return c.Status(http.StatusBadRequest).SendString("name is required")
	}
	id, err := CreateAutoTag(userID, body.Name, body.Matches)
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString(err.Error())
	}
	return c.JSON(fiber.Map{"id": id.Hex()})
}

// updateAutoTagRequest.Matches is nil when omitted from the body (not to be
// changed) vs a non-nil empty slice when explicitly cleared.
type updateAutoTagRequest struct {
	Name    *string  `json:"name"`
	Matches []string `json:"matches"`
}

// PATCH /api/autotags/:id
func UpdateAutoTagHandler(c *fiber.Ctx) error {
	userID, err := userIDFromLocals(c)
	if err != nil {
		return c.Status(http.StatusUnauthorized).SendString(err.Error())
	}
	autoTagID, err := bson.ObjectIDFromHex(c.Params("id"))
	if err != nil {
		return c.Status(http.StatusBadRequest).SendString("invalid id")
	}
	var body updateAutoTagRequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).SendString(err.Error())
	}
	err = UpdateAutoTag(userID, autoTagID, body.Name, body.Matches)
	if errors.Is(err, ErrAutoTagNotFound) {
		return c.Status(http.StatusNotFound).SendString(err.Error())
	}
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString(err.Error())
	}
	return c.SendStatus(http.StatusOK)
}

// DELETE /api/autotags/:id
func DeleteAutoTagHandler(c *fiber.Ctx) error {
	userID, err := userIDFromLocals(c)
	if err != nil {
		return c.Status(http.StatusUnauthorized).SendString(err.Error())
	}
	autoTagID, err := bson.ObjectIDFromHex(c.Params("id"))
	if err != nil {
		return c.Status(http.StatusBadRequest).SendString("invalid id")
	}
	err = DeleteAutoTag(userID, autoTagID)
	if errors.Is(err, ErrAutoTagNotFound) {
		return c.Status(http.StatusNotFound).SendString(err.Error())
	}
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString(err.Error())
	}
	return c.SendStatus(http.StatusOK)
}
