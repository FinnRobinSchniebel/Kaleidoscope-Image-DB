package authutil

import (
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

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return "", err
	}
	return string(bytes), err
}
func ComparePassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil

}
