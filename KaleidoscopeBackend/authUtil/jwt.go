package authutil

import (
	"fmt"
	"log"
	"strings"
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

type SessionSettings struct {
	Id            bson.ObjectID `json:"id,omitempty" bson:"_id,omitempty" form:"id,omitempty"`
	SessionID     string        `json:"session_id" bson:"session_id" form:"session_id"`
	RefreshToken  JWTClaims     `json:"refresh_token" bson:"refresh_token" form:"refresh_token"`
	IndefiniteRef bool          `json:"indefinite_ref" bson:"indefinite_ref" form:"indefinite_ref"`
}

var SessionDb *mongo.Collection

var JWTSecret []byte

func GenerateToken(user User, duration time.Duration) (string, *JWTClaims, error) {

	claims := CreateNewClaim(user, duration)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenString, err := token.SignedString(JWTSecret)
	if err != nil {
		return "", nil, err
	}

	return tokenString, claims, nil
}

func CreateNewClaim(user User, duration time.Duration) *JWTClaims {

	return &JWTClaims{
		UserID:     user.Id.Hex(),
		Is_revoked: false,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			Subject:   user.Username,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(duration)),
			Issuer:    "Kaleidoscope",
		},
	}
}

func VerifyToken(tokenString string) (*JWTClaims, error) {
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
func GetSessionTokenFromApiHelper(c *fiber.Ctx) (string, error) {
	sessionToken := c.Get("session_token", "")

	if sessionToken == "" || sessionToken == "Bearer " || !strings.HasPrefix(sessionToken, "Bearer ") {
		log.Printf("recieved invalid session token: %s", sessionToken)
		return "", fmt.Errorf("no valid Session token received")
	}
	sessionToken = strings.TrimPrefix(sessionToken, "Bearer ")

	return sessionToken, nil
}
