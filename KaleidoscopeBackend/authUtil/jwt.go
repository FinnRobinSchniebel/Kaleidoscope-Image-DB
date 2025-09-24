package authUtil

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

type JWTClaims struct {
	Is_revoked bool   `json:"is_revoked" bson:"is_revoked"`
	UserID     string `json:"userID" bson:"userID"`
	jwt.RegisteredClaims
}

var JWTSecret []byte

func GenerateToken(user User) (string, *JWTClaims, error) {

	claims := CreateNewClaim(user)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenString, err := token.SignedString(JWTSecret)
	if err != nil {
		return "", nil, err
	}

	return tokenString, claims, nil
}

func CreateNewClaim(user User) *JWTClaims {

	return &JWTClaims{
		UserID:     user.Id.Hex(),
		Is_revoked: false,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			Subject:   user.Username,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * 24 * 7)),
			Issuer:    "Kaleidoscope",
		},
	}
}

func BasicAuthorize(c *fiber.Ctx, userCollection *mongo.Collection) error {
	sessionT := c.Cookies("session_token")
	//log.Println("token: " + sessionT)
	if sessionT == "" {
		return errors.New("not authorized")
	}

	var userInfo User
	err := userCollection.FindOne(context.Background(), bson.M{"session_token": sessionT}).Decode(&userInfo)
	if err != nil {
		return err
		//return c.Status(400).SendString("user does not exist")
	}

	return nil

}

func VarifyToken(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {

		_, Ok := token.Method.(*jwt.SigningMethodHMAC)
		if !Ok {
			return nil, fmt.Errorf("invalid token")
		}

		return []byte(JWTSecret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("error Parsing token %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token")
	}

	return claims, nil

}
