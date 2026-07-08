package services

// this file is meant to contain all direct api endpoint code that either go to the front end or external plugins (TBA).
// any code that should be accessible from anywhere else in the back end should not be in this file and have its own dedicated location.

import (
	"github.com/gofiber/fiber/v2"
)

type ServiceInfo struct {
	ServiceName       string `json:"service"            form:"service"`
	Key1              string `json:"Key1"               form:"apiKey1"            bson:"Key1,omitempty"`
	Key2              string `json:"Key2"               form:"apiKey2"            bson:"Key2,omitempty"`
	User              string `json:"User"               form:"username"           bson:"User,omitempty"`
	Password          string `json:"Password"           form:"password"           bson:"Password,omitempty"`
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

	if err := AddServiceCredentials(userID, info.ServiceName, creds); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("failed to store credentials")
	}

	if err := DefaultScheduler.TestCredentials(info.ServiceName, userID, creds); err != nil {
		c.Status(fiber.StatusServiceUnavailable).SendString("Credentials Saved, Failed to Connect to service.")
	}

	DefaultScheduler.fireCredentialHook(info.ServiceName, userID, creds)

	return c.SendStatus(fiber.StatusOK)
}
