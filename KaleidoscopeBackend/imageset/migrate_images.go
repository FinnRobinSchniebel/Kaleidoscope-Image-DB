package imageset

// One-off migration to stable image IDs and the new hashes. It runs once on
// the test deployment (main.go's -migrate-images flag) and is then deleted.

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// legacySource is a source as stored before the migration, with image
// positions in attributed_to. The outer attributed_to wins over the embedded
// one (mongo-driver's dominant-field rule); an already-migrated set fails to
// decode, since an ObjectID isn't an int.
type legacySource struct {
	SourceInfo `bson:",inline"`
	Positions  []int `bson:"attributed_to"`
}

// legacySet is the part of a stored set the migration reads. Old images
// decode as ImageInfo directly; their pHash field is skipped.
type legacySet struct {
	ID        bson.ObjectID  `bson:"_id"`
	Path      string         `bson:"path"`
	DateAdded time.Time      `bson:"date_added"`
	Authors   []string       `bson:"authors"`
	Images    []ImageInfo    `bson:"images"`
	Sources   []legacySource `bson:"sources"`
}

// migrateSet gives old's images IDs, hashes and sizes (hashFile is given each
// file name) and turns its sources' positions into those IDs. It fails on
// data the migration doesn't fix: a position out of range, an unnormalized
// source name, a never-synced source with a midnight (unmarked zip) date, or
// an image no source is attributed to.
func migrateSet(old legacySet, hashFile func(name string) (ImageHashes, int, int, error)) (ImageSetMongo, error) {
	//empty rather than nil, so neither list is ever written as null
	set := ImageSetMongo{
		ID: old.ID, Path: old.Path, DateAdded: old.DateAdded, Authors: old.Authors,
		Image:   make([]ImageInfo, 0, len(old.Images)),
		Sources: make([]SourceInfo, 0, len(old.Sources)),
	}

	imageIDs := make([]bson.ObjectID, len(old.Images))
	for i := range imageIDs {
		imageIDs[i] = bson.NewObjectID()
	}

	attributed := make([]bool, len(old.Images))
	for _, legacy := range old.Sources {
		source := legacy.SourceInfo
		if source.Name != NormalizeSourceName(source.Name) {
			return ImageSetMongo{}, fmt.Errorf("source name %q isn't normalized", source.Name)
		}
		//a zip date stored before the day-only marker sat at exactly midnight UTC
		if source.LastChecked.IsZero() && !source.Date.IsZero() && source.Date.Equal(source.Date.UTC().Truncate(24*time.Hour)) {
			return ImageSetMongo{}, fmt.Errorf("source %s/%s has an unmarked midnight date %s", source.Name, source.SourceID, source.Date.Format(time.RFC3339))
		}

		source.AttributedTo = make([]bson.ObjectID, 0, len(legacy.Positions))
		for _, position := range legacy.Positions {
			if position < 0 || position >= len(imageIDs) {
				return ImageSetMongo{}, fmt.Errorf("source %s/%s is attributed to image %d of %d", source.Name, source.SourceID, position, len(imageIDs))
			}
			source.AttributedTo = append(source.AttributedTo, imageIDs[position])
			attributed[position] = true
		}

		//no source ever changed a set's images after import, and LastChecked is bumped by every applied update
		source.LastImageUpdate = old.DateAdded
		source.LastAppliedUpdate = old.DateAdded
		if source.LastChecked.After(old.DateAdded) {
			source.LastAppliedUpdate = source.LastChecked
		}
		source.ImageChangeState = ImageChangeNone
		set.Sources = append(set.Sources, source)
	}
	for position, isAttributed := range attributed {
		if !isAttributed {
			return ImageSetMongo{}, fmt.Errorf("image %d (%s) has no source", position, old.Images[position].Name)
		}
	}

	for position, storedImage := range old.Images {
		hashes, width, height, err := hashFile(storedImage.Name)
		if err != nil {
			return ImageSetMongo{}, fmt.Errorf("image %d (%s): %w", position, storedImage.Name, err)
		}
		storedImage.ID = imageIDs[position]
		storedImage.ImageHashes = hashes
		storedImage.Width, storedImage.Height = width, height
		set.Image = append(set.Image, storedImage)
	}

	deriveAuthors(&set)
	return set, nil
}

// hashStoredFile hashes and measures the stored full-res file name in folder
// path, through prepareImages so it hashes exactly like a new upload.
func hashStoredFile(path, name string) (ImageHashes, int, int, error) {
	prepared, err := prepareImages([]IncomingImage{{Media: DiskSource{Path: path + name}}})
	if err != nil {
		return ImageHashes{}, 0, 0, err
	}
	info := prepared[0].Info
	return info.ImageHashes, info.Width, info.Height, nil
}

// RunImageMigration converts every stored set to image IDs and the new
// hashes. Every set is converted in memory first; if any fails, each failure
// is logged and nothing is written, so a failed run works as a dry run.
// Rewriting images and sources drops the old pHash and pending_image_change.
func RunImageMigration() error {
	ctx := context.Background()
	cursor, err := Collection.Find(ctx, bson.M{})
	if err != nil {
		return fmt.Errorf("reading image sets: %w", err)
	}
	defer cursor.Close(ctx)

	var updates []mongo.WriteModel
	failed, imageCount := 0, 0
	for cursor.Next(ctx) {
		var old legacySet
		if err := cursor.Decode(&old); err != nil {
			log.Printf("migration: set %s: %v", cursor.Current.Lookup("_id"), err)
			failed++
			continue
		}
		set, err := migrateSet(old, func(name string) (ImageHashes, int, int, error) { return hashStoredFile(old.Path, name) })
		if err != nil {
			log.Printf("migration: set %s: %v", old.ID.Hex(), err)
			failed++
			continue
		}
		imageCount += len(set.Image)
		updates = append(updates, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": set.ID}).
			SetUpdate(bson.M{"$set": bson.M{"images": set.Image, "sources": set.Sources, "authors": set.Authors}}))
	}
	if err := cursor.Err(); err != nil {
		return fmt.Errorf("reading image sets: %w", err)
	}

	if failed > 0 {
		return fmt.Errorf("%d of %d sets can't be migrated; nothing was written", failed, failed+len(updates))
	}
	if len(updates) == 0 {
		log.Print("migration: no image sets to migrate")
		return nil
	}
	if _, err := Collection.BulkWrite(ctx, updates, options.BulkWrite().SetOrdered(true)); err != nil {
		return fmt.Errorf("writing migrated sets (restore the dump before running again): %w", err)
	}
	log.Printf("migration: migrated %d sets with %d images", len(updates), imageCount)
	return nil
}
