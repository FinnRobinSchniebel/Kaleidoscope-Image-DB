package authutil

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

var UserCollection *mongo.Collection

func GetUserByName(username string) (User, error) {

	var userInfo User
	err := UserCollection.FindOne(context.Background(), bson.M{"username": username}).Decode(&userInfo)
	if err != nil {
		//return err
		return User{}, fmt.Errorf("user does not exist")
	}
	return userInfo, nil
}
func GetUserById(id bson.ObjectID) (User, error) {
	var userInfo User
	err := UserCollection.FindOne(context.Background(), bson.M{"_id": id}).Decode(&userInfo)
	if err != nil {
		//return err
		return User{}, fmt.Errorf("user does not exist")
	}
	return userInfo, nil
}
func AddUser(user User) (*mongo.InsertOneResult, error) {
	result, err := UserCollection.InsertOne(context.Background(), user)

	if err != nil {
		return nil, err
	}
	return result, nil
}
func StoreRefresh(token *JWTClaims, canRefresh bool) error {

	entry := SessionSettings{
		SessionID:     token.ID,
		RefreshToken:  *token,
		IndefiniteRef: canRefresh,
	}

	_, err := SessionDb.InsertOne(context.Background(), entry)

	if err != nil {
		return fmt.Errorf("could Not create session on db: %d", err)
	}
	return nil
}

// returns erro, token, is session indefinite
func GetRefreshToken(tokenID string) (JWTClaims, bool, error) {
	var session SessionSettings

	err := SessionDb.FindOne(context.Background(), bson.D{{"session_id", tokenID}}).Decode(&session)
	if err != nil {
		return JWTClaims{}, session.IndefiniteRef, fmt.Errorf("could not find session: %s", tokenID)
	}
	//var token *authUtil.JWTClaims

	return session.RefreshToken, session.IndefiniteRef, nil
}

// find token by 'session_id'
func InvalidateRefreshTokenById(tokenID string) error {

	var session SessionSettings

	err := SessionDb.FindOne(context.Background(), bson.D{{"session_id", tokenID}}).Decode(&session)
	if err != nil {
		return err
	}

	updateRequest := bson.M{
		"$set": bson.M{
			"refresh_token.RefreshToken": session.RefreshToken,
		},
	}

	_, err = SessionDb.UpdateOne(context.Background(), bson.D{{"session_id", tokenID}}, updateRequest)

	if err != nil {
		return err
	}
	fmt.Println("Session invalidated: " + session.SessionID)

	return nil
}
