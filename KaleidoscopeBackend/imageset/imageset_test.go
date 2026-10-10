package imageset

import (
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

func TestSourceInfoReadsOldDocuments(t *testing.T) {
	old, err := bson.Marshal(bson.M{"name": "pixiv", "source_id": "42", "pending_image_change": false})
	if err != nil {
		t.Fatal(err)
	}
	var src SourceInfo
	if err := bson.Unmarshal(old, &src); err != nil {
		t.Fatal(err)
	}
	if src.ImageChangeState != ImageChangeNone || !src.LastAppliedUpdate.IsZero() {
		t.Errorf("old document decoded to state %q, LastAppliedUpdate %v; want none and zero", src.ImageChangeState, src.LastAppliedUpdate)
	}

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
