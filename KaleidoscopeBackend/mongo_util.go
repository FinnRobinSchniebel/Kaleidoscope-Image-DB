package main

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func findOverlappingHashes(hash string) ([]bson.ObjectID, error) {
	cursor, err := collection.Find(context.Background(), bson.D{{"hash", hash}})
	if err != nil {
		return nil, err
	}

	defer cursor.Close(context.Background())

	var itemList []bson.M
	cursor.All(context.Background(), &itemList)
	if len(itemList) == 0 {
		return nil, nil
	}

	var idList []bson.ObjectID
	for _, item := range itemList {
		idList = append(idList, item["_id"].(bson.ObjectID))
	}

	return idList, nil
}
