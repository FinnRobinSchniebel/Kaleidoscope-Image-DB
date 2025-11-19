package tags

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var TagsDB *mongo.Collection

type Tag struct {
	Id          bson.ObjectID `json:"id,omitempty" bson:"_id,omitempty" form:"id,omitempty"`
	Tag         string        `json:"tag" bson:"tag" form:"tag"`
	User        bson.ObjectID `json:"user" bson:"user" form:"user"`
	AutoAssigns []string      `json:"auto_assigns" bson:"auto_assigns" form:"auto_assigns"`
}

func AddTags(tags Tag) error {

	_, err := TagsDB.InsertOne(context.Background(), tags)

	if err != nil {
		fmt.Println(err.Error())
		return err
	}

	//TODO: Go through DB to auto tag existing items with the new auto tags

	return nil
}
func ReadTags(word string) ([]bson.M, error) {

	searchtags := bson.D{{Key: "tags", Value: bson.D{{"$regex", word}, {"$options", "i"}}}}
	searchtAutoTags := bson.D{{Key: "auto_assigns", Value: bson.D{{"$regex", word}, {"$options", "i"}}}}

	grouped := bson.D{{Key: "$match", Value: bson.D{
		{
			Key: "$or", Value: bson.A{searchtags, searchtAutoTags},
		},
	}}}

	cursor, err := TagsDB.Find(context.Background(), grouped, options.Find().SetProjection(bson.D{
		{Key: "tags", Value: 1},
		{Key: "auto_assigns", Value: 1},
	}))

	if err != nil {
		fmt.Println(err.Error())
		return nil, err
	}
	defer cursor.Close(context.Background())

	var results []bson.M
	if err := cursor.All(context.Background(), &results); err != nil {
		return nil, err
	}

	return results, nil
}

/* This function determines what tags it should be auto tagged as. It does not however preform the autoTag.
 */
func FindAutoTag(sourceTags []string) ([]string, error) {

	pipe := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.D{
			{"$or", bson.A{
				bson.D{{"tag", bson.D{{"$in", sourceTags}}}},
				bson.D{{"auto_assigns", bson.D{{"$in", sourceTags}}}},
			}},
		}}},

		bson.D{{Key: "$group", Value: bson.D{
			{"_id", nil},
			{"tags", bson.D{{"$addToSet", "$tag"}}},
		}}},

		bson.D{{Key: "$project", Value: bson.D{
			{"_id", 0},
			{"tags", 1},
		}}},
		// bson.D{{"$out", "pdb"}},
	}

	var unmarhselded []struct {
		Tags []string ` bson:"tags"`
	}

	cursor, err := TagsDB.Aggregate(context.Background(), pipe)

	if err != nil {
		fmt.Println("error" + err.Error())
		return nil, err
	}
	defer cursor.Close(context.Background())

	err = cursor.All(context.Background(), &unmarhselded)

	if len(unmarhselded) == 0 {
		return nil, nil
	}

	if err != nil {
		fmt.Println(err.Error())
		return nil, err
	}

	return unmarhselded[0].Tags, nil
}
