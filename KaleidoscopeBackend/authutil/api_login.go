package authutil

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func RegisterUser(c *fiber.Ctx) error {

	username := c.FormValue("username")
	password := c.FormValue("password")
	if len(username) < 3 {
		return errors.New("user Name Not long enough")
	}
	if len(password) < 6 {
		return errors.New("password is not long enough")
	}
	_, err := GetUserByName(username)

	//if quary succeeds return an error
	if err == nil {
		return c.Status(400).SendString("username already in use")
	}

	hashedPassword, err := HashPassword(password)
	if err != nil {
		return c.Status(400).SendString("bad password")
	}

	newuser := User{
		Username:       username,
		HashedPassword: hashedPassword,
	}

	log.Println("New User: " + newuser.Username)

	_, err = AddUser(newuser)

	if err != nil {
		return err
	}

	return c.SendStatus(200)
}

func LoginUser(c *fiber.Ctx) error {

	username := c.FormValue("username")
	password := c.FormValue("password")
	StayLoggedIn := c.QueryBool("stay_logged_in")

	/**** Check valid login ****/
	var userInfo User
	userInfo, err := GetUserByName(username)
	if err != nil {
		//return err
		log.Println("Login Request: Incorrect user! ")
		return c.Status(400).SendString("user does not exist")
	}

	if !ComparePassword(password, userInfo.HashedPassword) {
		log.Println("Login Request: Incorrect password! ")
		return c.Status(400).SendString("username and password did not match")
	}

	/**** create tokens ****/
	sessionToken, _, err := GenerateToken(userInfo, 15*time.Minute)
	if err != nil {
		//return err
		return err
	}

	//refresh token
	RefreshToken, token, err := GenerateToken(userInfo, 48*time.Hour)
	if err != nil {
		//return err
		return err
	}

	/**** store refresh token ****/
	err = StoreRefresh(token, StayLoggedIn)
	if err != nil {
		return err
	}

	/**** send tokens ****/
	c.Cookie(&fiber.Cookie{
		Name:    "refresh_token",
		Value:   RefreshToken,
		Expires: time.Now().Add(48 * time.Hour),
		Path:    "/api/session",
		//Secure:   true,
		HTTPOnly: true,
	})

	res := fiber.Map{
		"session_token": sessionToken,
	}
	log.Println("Login Request: Logged in User")
	return c.Status(200).JSON(res)
}

// Uses the Refresh_token to invalidate the session and sends a new session token that expires immediately
// Does not send a new refresh token as any check with the old token will fail anyway.
func LogoutUser(c *fiber.Ctx) error {
	userRefTok := c.Cookies("refresh_token", "")
	claim, _ := VerifyToken(userRefTok)
	tokenId := claim.ID

	err := InvalidateRefreshTokenById(tokenId)
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString("Could not Invalidate token")
	}

	bId, err := bson.ObjectIDFromHex(claim.UserID)
	if err != nil {
		return err
	}

	sessionToken, _, err := GenerateToken(User{Id: bId, Username: claim.Subject}, 0)
	if err != nil {
		//return err
		return err
	}
	res := fiber.Map{
		"session_token": sessionToken,
	}
	//"User Loggedout succesfully"
	log.Printf("User: %s logged out successfully from session %s", claim.UserID, claim.ID)

	return c.Status(200).JSON(res)
}
