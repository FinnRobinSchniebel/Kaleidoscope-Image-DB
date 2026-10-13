package imageset

import (
	"errors"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestMigrateSetConvertsPositionsToIDs(t *testing.T) {
	dateAdded := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	lastChecked := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)
	raw, err := bson.Marshal(bson.M{
		"_id": bson.NewObjectID(), "path": "/data/user/alice/", "date_added": dateAdded, "authors": bson.A{"old name"},
		"images": bson.A{
			bson.M{"images": "a.png", "low_images": "", "active": true, "hash": "p:1"},
			bson.M{"images": "bb.png", "low_images": "low_bb.png", "active": true, "hash": "p:2"},
			bson.M{"images": "ccc.png", "low_images": "", "active": true, "hash": "p:3"},
		},
		"sources": bson.A{
			bson.M{"name": "pixiv", "source_id": "42", "source_author": "alice", "attributed_to": bson.A{2, 0}, "last_checked": lastChecked, "pending_image_change": false},
			bson.M{"name": "upload", "source_id": "", "attributed_to": bson.A{1}, "pending_image_change": true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var old legacySet
	if err := bson.Unmarshal(raw, &old); err != nil {
		t.Fatalf("decoding the old document: %v", err)
	}

	hashFile := func(name string) (ImageHashes, int, int, error) {
		return ImageHashes{Pixel: []byte("pixel " + name), PDQ: []byte("pdq " + name)}, 10 * len(name), 20, nil
	}
	set, err := migrateSet(old, hashFile)
	if err != nil {
		t.Fatal(err)
	}

	imageIDs := make([]bson.ObjectID, len(set.Image))
	for i, name := range []string{"a.png", "bb.png", "ccc.png"} {
		migrated := set.Image[i]
		imageIDs[i] = migrated.ID
		if migrated.ID.IsZero() || migrated.Name != name || !migrated.IsImageActive {
			t.Errorf("image %d = %+v, want an ID and its name and active flag kept", i, migrated)
		}
		if string(migrated.Pixel) != "pixel "+name || string(migrated.PDQ) != "pdq "+name || migrated.Width != 10*len(name) || migrated.Height != 20 {
			t.Errorf("image %d: hashes %q/%q, size %dx%d; want the hasher's for %s", i, migrated.Pixel, migrated.PDQ, migrated.Width, migrated.Height, name)
		}
	}
	if set.Image[1].LowResName != "low_bb.png" {
		t.Errorf("low-res name = %q, want it kept", set.Image[1].LowResName)
	}
	if imageIDs[0] == imageIDs[1] || imageIDs[1] == imageIDs[2] || imageIDs[0] == imageIDs[2] {
		t.Errorf("image IDs %v aren't distinct", imageIDs)
	}

	pixiv, upload := set.Sources[0], set.Sources[1]
	if !slices.Equal(pixiv.AttributedTo, []bson.ObjectID{imageIDs[2], imageIDs[0]}) || !slices.Equal(upload.AttributedTo, []bson.ObjectID{imageIDs[1]}) {
		t.Errorf("AttributedTo = %v and %v, want positions [2 0] and [1] as IDs", pixiv.AttributedTo, upload.AttributedTo)
	}
	//LastAppliedUpdate is the later of date_added and LastChecked
	if !pixiv.LastImageUpdate.Equal(dateAdded) || !pixiv.LastAppliedUpdate.Equal(lastChecked) {
		t.Errorf("pixiv timestamps = %v / %v, want date_added / LastChecked", pixiv.LastImageUpdate, pixiv.LastAppliedUpdate)
	}
	if !upload.LastImageUpdate.Equal(dateAdded) || !upload.LastAppliedUpdate.Equal(dateAdded) {
		t.Errorf("upload timestamps = %v / %v, want date_added for both", upload.LastImageUpdate, upload.LastAppliedUpdate)
	}
	if !slices.Equal(set.Authors, []string{"alice"}) {
		t.Errorf("Authors = %v, want them re-derived from the sources", set.Authors)
	}

	//the shape RunImageMigration writes
	written, err := bson.Marshal(bson.M{"images": set.Image, "sources": set.Sources})
	if err != nil {
		t.Fatal(err)
	}
	doc := bson.Raw(written)
	for _, field := range []string{"id", "pixel_hash", "pdq_hash"} {
		if _, err := doc.LookupErr("images", "0", field); err != nil {
			t.Errorf("stored image has no top-level %q", field)
		}
	}
	if _, err := doc.LookupErr("images", "0", "hash"); err == nil {
		t.Error("stored image still has the old hash")
	}
	if _, err := doc.LookupErr("sources", "0", "pending_image_change"); err == nil {
		t.Error("stored source still has pending_image_change")
	}
}

func TestMigrateSetAborts(t *testing.T) {
	hashOK := func(string) (ImageHashes, int, int, error) { return ImageHashes{}, 1, 1, nil }
	hashFails := func(string) (ImageHashes, int, int, error) { return ImageHashes{}, 0, 0, errors.New("file missing") }
	//one image attributed to one synced source; each case changes one thing
	valid := func() legacySet {
		return legacySet{
			ID:        bson.NewObjectID(),
			DateAdded: time.Now(),
			Images:    []ImageInfo{{Name: "a.png"}},
			Sources:   []legacySource{{SourceInfo: SourceInfo{Name: "pixiv", SourceID: "1", LastChecked: time.Now()}, Positions: []int{0}}},
		}
	}
	midnight := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	neverSynced := func(date time.Time) func(*legacySet) {
		return func(set *legacySet) { set.Sources[0].LastChecked, set.Sources[0].Date = time.Time{}, date }
	}

	tests := []struct {
		name    string
		change  func(*legacySet)
		hash    func(string) (ImageHashes, int, int, error)
		wantErr bool
	}{
		{"valid", func(*legacySet) {}, hashOK, false},
		{"position out of range", func(set *legacySet) { set.Sources[0].Positions = []int{1} }, hashOK, true},
		{"unattributed image", func(set *legacySet) { set.Images = append(set.Images, ImageInfo{Name: "b.png"}) }, hashOK, true},
		{"name not normalized", func(set *legacySet) { set.Sources[0].Name = "Pixiv" }, hashOK, true},
		{"never-synced midnight date", neverSynced(midnight), hashOK, true},
		{"never-synced day-only date", neverSynced(DayOnlyDate(midnight)), hashOK, false},
		{"never-synced with no date", neverSynced(time.Time{}), hashOK, false},
		{"file can't be hashed", func(*legacySet) {}, hashFails, true},
	}
	for _, tt := range tests {
		set := valid()
		tt.change(&set)
		if _, err := migrateSet(set, tt.hash); (err != nil) != tt.wantErr {
			t.Errorf("%s: err = %v, want error %t", tt.name, err, tt.wantErr)
		}
	}

	//a set migrated already holds IDs, which can't decode as positions
	raw, err := bson.Marshal(bson.M{"_id": bson.NewObjectID(), "sources": bson.A{bson.M{"name": "pixiv", "attributed_to": bson.A{bson.NewObjectID()}}}})
	if err != nil {
		t.Fatal(err)
	}
	var migrated legacySet
	if err := bson.Unmarshal(raw, &migrated); err == nil {
		t.Error("an already-migrated set decoded as an old one")
	}
}
