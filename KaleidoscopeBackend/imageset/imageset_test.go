package imageset

import (
	"errors"
	"slices"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestDeriveFromSourcesNewSet(t *testing.T) {
	set := ImageSetMongo{
		Sources: []SourceInfo{
			{Title: "", SourceAuthor: "bob", Description: "one"},
			{Title: "First", SourceAuthor: "alice"},
			{Title: "Second", SourceAuthor: "bob", Description: "two"},
			{Title: "Third", SourceAuthor: ""},
			{Title: "Fourth", SourceAuthor: unknownAuthor},
		},
	}
	DeriveFromSources(&set)

	if set.Title != "First" {
		t.Errorf("Title = %q, want first non-empty source title", set.Title)
	}
	if !slices.Equal(set.Authors, []string{"bob", "alice"}) {
		t.Errorf("Authors = %v, want [bob alice]", set.Authors)
	}
	if set.Description != "one\n\ntwo" {
		t.Errorf("Description = %q, want source descriptions joined by a blank line", set.Description)
	}
	if set.Sources[1].Description != "" {
		t.Errorf("a source without a description was given one: %q", set.Sources[1].Description)
	}
}

func TestDeriveFromSourcesKeepsExistingValues(t *testing.T) {
	set := ImageSetMongo{
		Title:       "Mine",
		Authors:     []string{"me"},
		Description: "my notes",
		Sources:     []SourceInfo{{Title: "Theirs", SourceAuthor: "alice", Description: "caption"}},
	}
	DeriveFromSources(&set)

	if set.Title != "Mine" || set.Description != "my notes" {
		t.Errorf("Title = %q, Description = %q; existing values were overwritten", set.Title, set.Description)
	}
	if !slices.Equal(set.Authors, []string{"alice"}) {
		t.Errorf("Authors = %v, want [alice]: Authors come only from sources", set.Authors)
	}
}

func TestDeriveFromSourcesEmpty(t *testing.T) {
	set := ImageSetMongo{Sources: []SourceInfo{{}}}
	DeriveFromSources(&set)

	if set.Title != "" || set.Description != "" {
		t.Errorf("got Title=%q Description=%q, want both empty", set.Title, set.Description)
	}
	if !slices.Equal(set.Authors, []string{unknownAuthor}) {
		t.Errorf("Authors = %v, want the unknown placeholder", set.Authors)
	}
}

func TestNormalizeSourceNameDefaultsEmpty(t *testing.T) {
	for _, name := range []string{"", "   "} {
		if got := NormalizeSourceName(name); got != UploadSourceName {
			t.Errorf("NormalizeSourceName(%q) = %q, want %q", name, got, UploadSourceName)
		}
	}
}

func TestImageChangeNoneLeftOut(t *testing.T) {
	encoded, err := bson.Marshal(SourceInfo{Name: "pixiv"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bson.Raw(encoded).LookupErr("image_change_state"); err == nil {
		t.Error("the none state was stored instead of left out")
	}
}

func TestSameSource(t *testing.T) {
	a := SourceInfo{Name: "pixiv", SourceID: "42", Title: "A", SourceAuthor: "x"}
	if !SameSource(a, SourceInfo{Name: "pixiv", SourceID: "42", Title: "B"}) {
		t.Error("same name and id with a different title should be the same source")
	}
	if SameSource(a, SourceInfo{Name: "pixiv", SourceID: "43"}) || SameSource(a, SourceInfo{Name: "upload", SourceID: "42"}) {
		t.Error("a different name or id should be a different source")
	}
}

func TestCheckAttribution(t *testing.T) {
	first, second, stranger := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	images := []ImageInfo{{ID: first}, {ID: second}}
	tests := []struct {
		name    string
		images  []ImageInfo
		sources []SourceInfo
		valid   bool
	}{
		{"each source attributes its own images", images, []SourceInfo{
			{Name: "pixiv", SourceID: "1", AttributedTo: []bson.ObjectID{first}},
			{Name: "upload", AttributedTo: []bson.ObjectID{second}},
		}, true},
		{"a source with nothing attributed", images, []SourceInfo{
			{Name: "pixiv", SourceID: "1", AttributedTo: []bson.ObjectID{second, first}},
			{Name: "upload"},
		}, true},
		{"an image no source attributes", images, []SourceInfo{
			{Name: "pixiv", SourceID: "1", AttributedTo: []bson.ObjectID{first}},
		}, false},
		{"an attributed ID not in the set", images, []SourceInfo{
			{Name: "pixiv", SourceID: "1", AttributedTo: []bson.ObjectID{first, second, stranger}},
		}, false},
		{"a zero image ID", []ImageInfo{{ID: first}, {}}, []SourceInfo{
			{Name: "pixiv", SourceID: "1", AttributedTo: []bson.ObjectID{first}},
		}, false},
		{"an image ID twice in the set", []ImageInfo{{ID: first}, {ID: first}}, []SourceInfo{
			{Name: "pixiv", SourceID: "1", AttributedTo: []bson.ObjectID{first}},
		}, false},
		{"one source listing an ID twice", images, []SourceInfo{
			{Name: "pixiv", SourceID: "1", AttributedTo: []bson.ObjectID{first, second, first}},
		}, false},
	}
	for _, tt := range tests {
		set := ImageSetMongo{Image: tt.images, Sources: tt.sources}
		err := set.checkAttribution()
		if tt.valid && err != nil {
			t.Errorf("%s: rejected: %v", tt.name, err)
		}
		if !tt.valid && !errors.Is(err, errBrokenAttribution) {
			t.Errorf("%s: err = %v, want errBrokenAttribution", tt.name, err)
		}
	}
}

func TestJoinDescriptions(t *testing.T) {
	tests := []struct{ current, next, want string }{
		{"", "", ""},
		{"a", "", "a"},
		{"", "b", "b"},
		{"a", "b", "a\n\nb"},
	}
	for _, tt := range tests {
		if got := JoinDescriptions(tt.current, tt.next); got != tt.want {
			t.Errorf("JoinDescriptions(%q, %q) = %q, want %q", tt.current, tt.next, got, tt.want)
		}
	}
}
