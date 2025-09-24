package main

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/authUtil"
	"context"
	"fmt"
	"log"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type CollisionResponsePair struct {
	IdOfHashCollision bson.ObjectID
	ImageNumber       int
}
type SessionSettings struct {
	Id            bson.ObjectID      `json:"id,omitempty" bson:"_id,omitempty" form:"id,omitempty"`
	SessionID     string             `json:"sessionid" bson:"sessionid" form:"sessionid"`
	RefreshToken  authUtil.JWTClaims `json:"refreshtoken" bson:"refreshtoken" form:"refreshtoken"`
	IndefiniteRef bool               `json:"indefiniteref" bson:"indefiniteref" form:"indefiniteref"`
}

func findOverlappingHashes(hash string) ([]CollisionResponsePair, error) {
	cursor, err := collection.Find(context.Background(), bson.D{{"hash", hash}})
	if err != nil {
		return nil, err
	}

	defer cursor.Close(context.Background())

	var itemList []ImageSetMongo

	cursor.All(context.Background(), &itemList)
	if len(itemList) == 0 {
		return nil, nil
	}

	var idList []CollisionResponsePair
	for _, item := range itemList {
		for index, imageH := range item.ImageHash {
			if imageH == hash {
				idList = append(idList, CollisionResponsePair{item.ID, index})
			}
		}

		//var iSet ImageSetMongo
		//bson.Unmarshal([]byte(item.String()), &iSet)
		//item["_id"].(bson.ObjectID)

		itemList = append(itemList)

		//idList = append(idList, CollisionResponsePair{item.ID, })
	}

	return idList, nil
}

func GetFromID(id ...string) ([]ImageSetMongo, error) {

	var IdBson []bson.ObjectID

	for _, item := range id {
		ObjId, err := bson.ObjectIDFromHex(item)
		if err != nil {
			return nil, err
		}
		IdBson = append(IdBson, ObjId)
	}

	var iSets []ImageSetMongo

	var entry ImageSetMongo

	for _, ObjId := range IdBson {
		err := collection.FindOne(context.Background(), bson.D{{"_id", ObjId}}).Decode(&entry)
		if err != nil {
			log.Println("Failed to find file!")
			return nil, err
		}
		iSets = append(iSets, entry)
	}
	return iSets, nil
}

func StoreRefresh(token *authUtil.JWTClaims, canRefresh bool) error {

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
