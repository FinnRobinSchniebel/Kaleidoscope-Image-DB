package authutil

import (
	"crypto/hmac"
	"crypto/sha256"

	"go.mongodb.org/mongo-driver/v2/bson"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	Id             bson.ObjectID `json:"id,omitempty" bson:"_id,omitempty" form:"id,omitempty"`
	Username       string        `json:"username" bson:"username" form:"username"`
	HashedPassword string        `json:"password" bson:"password" form:"password"`
	CreatedDate    bson.DateTime `json:"creation_date" bson:"creation_date" form:"creation_date"`
	IsAdmin        bool          `json:"is_admin" bson:"is_admin" form:"is_admin"`
}

// PasswordPepper is a server-side secret mixed into passwords before hashing, kept outside
// the database so a leaked user collection alone isn't enough to brute-force passwords.
var PasswordPepper []byte

func pepperedPassword(password string) []byte {
	mac := hmac.New(sha256.New, PasswordPepper)
	mac.Write([]byte(password))
	return mac.Sum(nil) // fixed 32 bytes, always well under bcrypt's 72-byte input cap
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword(pepperedPassword(password), 10)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
func ComparePassword(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), pepperedPassword(password)) == nil
}
