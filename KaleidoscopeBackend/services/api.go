package services

// this file is meant to contain all direct api endpoint code that either go to the front end or external plugins (TBA).
// any code that should be accessible from anywhere else in the back end should not be in this file and have its own dedicated location.

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
)

// validateService rejects any service name that isn't a currently registered provider.
// This must run before serviceName is used to build a DB field path (e.g. "services.<name>.credentials"),
// since that name is otherwise attacker-controlled input taken straight from the URL.
func validateService(service string) error {
	if service == "" || !DefaultScheduler.IsRegisteredService(service) {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("unknown service: %q", service))
	}
	return nil
}

func Register(c *fiber.Ctx) error {
	userID := c.Locals("UserID").(string)
	service := c.Params("name")
	if err := validateService(service); err != nil {
		return err
	}

	var creds ExternalApiKeys
	if err := c.BodyParser(&creds); err != nil {
		return fiber.ErrBadRequest
	}

	if err := DefaultScheduler.TestCredentials(service, userID, creds); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString(fmt.Sprintf("Failed to Connect to service: %s", err.Error()))
	}

	if err := AddServiceCredentials(userID, service, creds); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("failed to store credentials")
	}

	_ = DefaultScheduler.AddUser(service, userID)
	DefaultScheduler.fireCredentialHook(service, userID, creds)

	return c.SendStatus(fiber.StatusOK)
}

func GetKeys(c *fiber.Ctx) error {
	userID := c.Locals("UserID").(string)
	service := c.Params("name")
	if err := validateService(service); err != nil {
		return err
	}
	key, err := GetServiceCredentials(userID, service)
	if err != nil {
		return err
	}
	return c.JSON(key)
}

// start sync of a service manually
func SyncService(c *fiber.Ctx) error {
	userID := c.Locals("UserID").(string)
	service := c.Params("name")
	if err := validateService(service); err != nil {
		return err
	}
	if err := DefaultScheduler.SyncUser(service, userID); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(err.Error())
	}
	return c.Status(fiber.StatusAccepted).SendString(service + " sync added to queue")
}

func RemoveService(c *fiber.Ctx) error {
	userID := c.Locals("UserID").(string)
	service := c.Params("name")
	if err := validateService(service); err != nil {
		return err
	}
	if err := DefaultScheduler.RemoveService(service, userID); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(err.Error())
	}
	return c.SendStatus(fiber.StatusOK)
}

// SetServiceSyncSchedule updates a user's sync scheduling settings for a service
// (currently just SyncIntervalHours) without touching stored credentials.
func SetServiceSyncSchedule(c *fiber.Ctx) error {
	userID := c.Locals("UserID").(string)
	service := c.Params("name")
	if err := validateService(service); err != nil {
		return err
	}

	var params struct {
		SyncIntervalHours int64 `form:"sync_interval_hours"`
	}
	if err := c.BodyParser(&params); err != nil {
		return fiber.ErrBadRequest
	}
	if params.SyncIntervalHours != 0 {
		params.SyncIntervalHours = max(params.SyncIntervalHours, MinScheduleInterval)
	}

	if err := SetServiceSyncInterval(userID, service, params.SyncIntervalHours); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("failed to update service settings")
	}

	DefaultScheduler.fireSyncSettingsHook(service, userID)

	return c.SendStatus(fiber.StatusOK)
}
func GetServiceSyncInfo(c *fiber.Ctx) error {
	userID := c.Locals("UserID").(string)
	service := c.Params("name")
	if err := validateService(service); err != nil {
		return err
	}
	syncInfo, err := GetServiceSync(userID, service)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{
		"sync_interval_hours": syncInfo.SyncIntervalHours,
		"last_synced":         syncInfo.LastSynced,
		"syncing":             DefaultScheduler.IsSyncing(service, userID),
	})
}

func PixivConnect(c *fiber.Ctx) error {
	userID := c.Locals("UserID").(string)

	var params struct {
		Code         string `form:"code"`
		CodeVerifier string `form:"code_verifier"`
		PixivUserID  string `form:"pixiv_user_id"`
	}
	if err := c.BodyParser(&params); err != nil || params.Code == "" || params.CodeVerifier == "" {
		return fiber.ErrBadRequest
	}

	refreshToken, err := PixivOAuthExchange(params.Code, params.CodeVerifier)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).SendString(err.Error())
	}

	creds := ExternalApiKeys{
		Key1:     refreshToken,
		UserName: params.PixivUserID,
	}

	if err := DefaultScheduler.TestCredentials(pixivServiceName, userID, creds); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString(fmt.Sprintf("Failed to Connect to service: %s", err.Error()))
	}
	if err := AddServiceCredentials(userID, pixivServiceName, creds); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("failed to store credentials")
	}
	_ = DefaultScheduler.AddUser(pixivServiceName, userID)
	DefaultScheduler.fireCredentialHook(pixivServiceName, userID, creds)
	return c.SendStatus(fiber.StatusOK)
}

// ListServices reports every registered service integration and whether the
// caller has connected it. Sourced from the scheduler's in-memory user
// rotation (kept in sync via AddUser/RemoveUser at connect/disconnect time),
// not the database, so it always reflects what the scheduler will actually
// act on.
func ListServices(c *fiber.Ctx) error {
	userID := c.Locals("UserID").(string)

	names := DefaultScheduler.RegisteredServiceNames()
	out := make(fiber.Map, len(names))
	for _, name := range names {
		if DefaultScheduler.IsUserRegistered(name, userID) {
			out[name] = "ok"
		} else {
			out[name] = "No"
		}
	}
	return c.JSON(out)
}
