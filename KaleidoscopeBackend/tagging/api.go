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

// GET /api/sourcetags/search?source=&lang=&prefix=
func SearchSourceTagsHandler(c *fiber.Ctx) error {
	userID, err := userIDFromLocals(c)
	if err != nil {
		return c.Status(http.StatusUnauthorized).SendString(err.Error())
	}
	prefix := c.Query("prefix")
	if prefix == "" {
		return c.Status(http.StatusBadRequest).SendString("prefix is required")
	}
	results, err := SearchSourceTags(userID, c.Query("source"), c.Query("lang"), prefix, 20)
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString(err.Error())
	}
	return c.JSON(results)
}

// GET /api/sourcetags?source=&cursor=&limit= - limit<=0 (including omitted)
// returns everything matching. TODO: give this a bounded default and real
// paging once a UI consumes the cursor param.
func ListSourceTagsHandler(c *fiber.Ctx) error {
	userID, err := userIDFromLocals(c)
	if err != nil {
		return c.Status(http.StatusUnauthorized).SendString(err.Error())
	}
	results, err := ListSourceTags(userID, c.Query("source"), c.Query("cursor"), c.QueryInt("limit", 0))
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString(err.Error())
	}
	return c.JSON(results)
}

// GET /api/autotags?prefix=&limit= - {id, name, count} only, no SourceTagDoc
// resolution. For autocomplete and for resolving ImageSetMongo.AutoTags IDs
// to names. limit <= 0 means unlimited.
func ListAutoTagsHandler(c *fiber.Ctx) error {
	userID, err := userIDFromLocals(c)
	if err != nil {
		return c.Status(http.StatusUnauthorized).SendString(err.Error())
	}
	results, err := ListAutoTagSummaries(userID, c.Query("prefix"), c.QueryInt("limit", 0))
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString(err.Error())
	}
	return c.JSON(results)
}

// GET /api/autotags/byIds?ids=id1,id2,... - resolves specific AutoTag IDs to
// {id, name, count}. Missing/unknown IDs are omitted, not an error.
func AutoTagSummariesByIDHandler(c *fiber.Ctx) error {
	userID, err := userIDFromLocals(c)
	if err != nil {
		return c.Status(http.StatusUnauthorized).SendString(err.Error())
	}
	raw := strings.Split(c.Query("ids"), ",")
	ids := make([]bson.ObjectID, 0, len(raw))
	for _, s := range raw {
		if s == "" {
			continue
		}
		id, err := bson.ObjectIDFromHex(s)
		if err != nil {
			return c.Status(http.StatusBadRequest).SendString("invalid id: " + s)
		}
		ids = append(ids, id)
	}
	results, err := AutoTagSummariesByID(userID, ids)
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString(err.Error())
	}
	return c.JSON(results)
}

// GET /api/autotags/details?prefix= - full AutoTagWithMatches, resolved
// SourceTagDocs included. For the edit page only; costs an extra query and
// a much larger payload than ListAutoTagsHandler.
func ListAutoTagDetailsHandler(c *fiber.Ctx) error {
	userID, err := userIDFromLocals(c)
	if err != nil {
		return c.Status(http.StatusUnauthorized).SendString(err.Error())
	}
	results, err := ListAutoTags(userID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString(err.Error())
	}
	if prefix := c.Query("prefix"); prefix != "" {
		results = filterAutoTagsByPrefix(results, prefix)
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
	Name           string   `json:"name"`
	SrcTagKeyMatch []string `json:"srcTagKeyMatch"`
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
	id, err := CreateAutoTag(userID, body.Name, body.SrcTagKeyMatch)
	if errors.Is(err, ErrAutoTagNameExists) {
		return c.Status(http.StatusConflict).SendString(err.Error())
	}
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString(err.Error())
	}
	return c.JSON(fiber.Map{"id": id.Hex()})
}

// updateAutoTagRequest.SrcTagKeyMatch is nil when omitted from the body (not
// to be changed) vs a non-nil empty slice when explicitly cleared.
type updateAutoTagRequest struct {
	Name           *string  `json:"name"`
	SrcTagKeyMatch []string `json:"srcTagKeyMatch"`
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
	err = UpdateAutoTag(userID, autoTagID, body.Name, body.SrcTagKeyMatch)
	if errors.Is(err, ErrAutoTagNotFound) {
		return c.Status(http.StatusNotFound).SendString(err.Error())
	}
	if errors.Is(err, ErrAutoTagNameExists) {
		return c.Status(http.StatusConflict).SendString(err.Error())
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
