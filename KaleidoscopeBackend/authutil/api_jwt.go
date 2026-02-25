package authutil

import (
	"log"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func AuthSessionToken(c *fiber.Ctx) error {

	sessionToken, err := GetSessionTokenFromApiHelper(c)
	if err != nil {
		return c.Status(http.StatusUnauthorized).SendString(err.Error())
	}

	claims, err := VerifyToken(sessionToken)
	if err != nil {
		return c.Status(http.StatusUnauthorized).SendString(err.Error())

	}
	c.Locals("UserID", claims.UserID)

	return c.Next()
}

func NewSessionToken(c *fiber.Ctx) error {

	userRefTok := c.Cookies("refresh_token", "")
	if userRefTok == "" {
		return c.Status(http.StatusUnauthorized).SendString("no refresh token given")
	}

	userRefClaim, err := VerifyToken(userRefTok)
	if err != nil {
		return c.Status(http.StatusUnauthorized).SendString("Invalid token")
	}

	serverClaim, _, err := GetRefreshToken(userRefClaim.ID)
	if err != nil {
		return c.Status(http.StatusUnauthorized).SendString("no session on server")
	}

	if serverClaim.Is_revoked {
		return c.Status(http.StatusUnauthorized).SendString("access revoked")
	}

	if serverClaim.UserID != userRefClaim.UserID {
		return c.Status(http.StatusUnauthorized).SendString("invalid session")
	}

	//creating User without checking db to improve speed (if something passes all of the checks we already assume the request is trustworthy but still user our claim to avoid injection)
	bId, err := bson.ObjectIDFromHex(serverClaim.UserID)
	if err != nil {
		return err
	}

	sessionToken, _, err := GenerateToken(User{Id: bId, Username: serverClaim.Subject}, 15*time.Minute)
	if err != nil {
		return err
	}

	res := fiber.Map{
		"session_token": sessionToken,
	}

	log.Println("New session token created for user: " + userRefClaim.UserID)

	return c.Status(200).JSON(res)
}

// Accepts a single id of a refresh token (Must be admin to do so).
// If none is given it will try to invalidate the used token
func InvalidateRefreshToken(c *fiber.Ctx) error {

	userRefTok := c.Cookies("refresh_token", "")
	claim, _ := VerifyToken(userRefTok)
	tokenId := claim.ID

	param := c.Params("id", "")

	if userRefTok == "" {
		return c.Status(http.StatusBadRequest).SendString("no refresh token provided in request")
	}

	if param != "" && tokenId != param {
		bid, err := bson.ObjectIDFromHex(claim.UserID)
		if err != nil {
			return c.Status(http.StatusBadRequest).SendString("Failed to turn user ID into a valid ID")
		}
		user, err := GetUserById(bid)

		if err != nil {
			return c.Status(http.StatusBadRequest).SendString("Invalid user ID in token")
		}
		if !user.IsAdmin {
			return c.Status(http.StatusUnauthorized).SendString("Must be Admin to Invalidate another users Token")
		}
		tokenId = param
	}

	err := InvalidateRefreshTokenById(tokenId)
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString("Could not Invalidate token")
	}

	log.Println("Session token invalidated for user: " + claim.UserID)

	return c.Status(200).SendString("session invalidated successfully")
}
