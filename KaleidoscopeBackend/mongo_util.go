package main

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type CollisionResponsePair struct {
	IdOfHashCollision bson.ObjectID
	ImageNumber       int
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
