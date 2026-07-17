package services

// this file is meant to contain all direct api endpoint code that either go to the front end or external plugins (TBA).
// any code that should be accessible from anywhere else in the back end should not be in this file and have its own dedicated location.

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
)

type ServiceInfo struct {
	ServiceName       string `json:"service"            form:"service"`
	Key1              string `json:"key1"               form:"apiKey1"            bson:"Key1,omitempty"`
	Key2              string `json:"key2"               form:"apiKey2"            bson:"Key2,omitempty"`
	User              string `json:"user"               form:"username"           bson:"User,omitempty"`
	Password          string `json:"password"           form:"password"           bson:"Password,omitempty"`
	SyncIntervalHours int64  `json:"sync_interval_hours" form:"sync_interval_hours"`
}

func Register(c *fiber.Ctx) error {
	userID := c.Locals("UserID").(string)

	var info ServiceInfo
	if err := c.BodyParser(&info); err != nil {
		return fiber.ErrBadRequest
	}
	if info.ServiceName == "" {
		return fiber.ErrBadRequest
	}

	creds := ExternalApiKeys{
		Key1:              info.Key1,
		Key2:              info.Key2,
		UserName:          info.User,
		Password:          info.Password,
		SyncIntervalHours: info.SyncIntervalHours,
	}

	if err := DefaultScheduler.TestCredentials(info.ServiceName, userID, creds); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString(fmt.Sprintf("Failed to Connect to service: %s", err.Error()))
	}

	if err := AddServiceCredentials(userID, info.ServiceName, creds); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("failed to store credentials")
	}

	DefaultScheduler.fireCredentialHook(info.ServiceName, userID, creds)

	return c.SendStatus(fiber.StatusOK)
}

func GetKeys(c *fiber.Ctx) error {
	userID := c.Locals("UserID").(string)
	service := c.Query("Service")
	key, err := GetServiceCredentials(userID, service)
	if err != nil {
		return err
	}
	return c.JSON(key)
}

func SyncService(c *fiber.Ctx) error {
	userID := c.Locals("UserID").(string)
	if err := SyncPixivBookmarks(userID); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(err.Error())
	}
	return c.Status(fiber.StatusAccepted).SendString("pixiv bookmark sync Added to Queue")
}

func PixivConnect(c *fiber.Ctx) error {
	userID := c.Locals("UserID").(string)

	var params struct {
		Code              string `form:"code"`
		CodeVerifier      string `form:"code_verifier"`
		PixivUserID       string `form:"pixiv_user_id"`
		SyncIntervalHours int64  `form:"sync_interval_hours"`
	}
	if err := c.BodyParser(&params); err != nil || params.Code == "" || params.CodeVerifier == "" {
		return fiber.ErrBadRequest
	}

	refreshToken, err := PixivOAuthExchange(params.Code, params.CodeVerifier)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).SendString(err.Error())
	}

	creds := ExternalApiKeys{
		Key1:              refreshToken,
		UserName:          params.PixivUserID,
		SyncIntervalHours: params.SyncIntervalHours,
	}

	if err := DefaultScheduler.TestCredentials(pixivServiceName, userID, creds); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString(fmt.Sprintf("Failed to Connect to service: %s", err.Error()))
	}
	if err := AddServiceCredentials(userID, pixivServiceName, creds); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("failed to store credentials")
	}
	DefaultScheduler.fireCredentialHook(pixivServiceName, userID, creds)
	return c.SendStatus(fiber.StatusOK)
}

// func RemoveKey(c *fiber.Ctx) error {

// }
