package imageset

import (
	"bytes"
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type StoredImageHash struct {
	SetID   bson.ObjectID
	ImageID bson.ObjectID
	Hashes  ImageHashes
	Plain   bool
}

// HashMatch is a stored image matching an incoming one: Exact when the pixels
// are equal, otherwise the same picture within SimilarDistance.
type HashMatch struct {
	SetID   bson.ObjectID `json:"set_id"`
	ImageID bson.ObjectID `json:"image_id"`
	Exact   bool          `json:"exact"`
	Plain   bool          `json:"plain"` //either image is a plain page; only exact matches are kept then
}

// CollisionMap holds each incoming image's matches, keyed by its position in
// the import. Images without a match are left out.
type CollisionMap map[int][]HashMatch

// loadUserImageHashes returns the hashes of every image in userID's sets,
// inactive ones included, in one query. PDQ distance can't be indexed, so
// matching happens in Go over this list.
func loadUserImageHashes(userID string) ([]StoredImageHash, error) {
	opts := options.Find().SetProjection(bson.M{"images.id": 1, "images.pixel_hash": 1, "images.pdq_hash": 1})
	cursor, err := Collection.Find(context.Background(), bson.M{"kscope_userid": userID}, opts)
	if err != nil {
		return nil, fmt.Errorf("loading image hashes: %w", err)
	}
	var sets []ImageSetMongo
	if err := cursor.All(context.Background(), &sets); err != nil {
		return nil, fmt.Errorf("loading image hashes: %w", err)
	}

	var stored []StoredImageHash
	for _, set := range sets {
		for _, img := range set.Image {

			plain, err := isPlain(img.PDQ)
			if err != nil {
				return nil, err
			}

			stored = append(stored, StoredImageHash{SetID: set.ID, ImageID: img.ID, Hashes: img.ImageHashes, Plain: plain})
		}
	}
	return stored, nil
}

// findHashMatches returns the stored images matching hashes: exact when both
// pixel hashes are present and equal, otherwise similar when the PDQ hashes
// are within SimilarDistance. A missing hash never matches, and a pair where
// either side is a plain page only matches exactly (plain pages all look alike).
func findHashMatches(hashes ImageHashes, plain bool, stored []StoredImageHash) []HashMatch {

	var matches []HashMatch
	for _, storedImage := range stored {

		pairPlain := plain || storedImage.Plain
		exact := len(hashes.Pixel) == pixelHashBytes && bytes.Equal(hashes.Pixel, storedImage.Hashes.Pixel)

		if exact || (!pairPlain && PDQDistance(hashes.PDQ, storedImage.Hashes.PDQ) <= SimilarDistance) {
			matches = append(matches, HashMatch{SetID: storedImage.SetID, ImageID: storedImage.ImageID, Exact: exact, Plain: pairPlain})
		}
	}
	return matches
}

// findImageMatches loads userID's stored hashes once and matches every
// prepared image against them, before anything is written.
func findImageMatches(userID string, images []PreparedImage) (CollisionMap, error) {

	stored, err := loadUserImageHashes(userID)
	if err != nil {
		return nil, err
	}

	collisions := make(CollisionMap)
	for index, preparedImage := range images {
		if matches := findHashMatches(preparedImage.Info.ImageHashes, preparedImage.Plain, stored); len(matches) != 0 {
			collisions[index] = matches
		}
	}
	return collisions, nil
}
