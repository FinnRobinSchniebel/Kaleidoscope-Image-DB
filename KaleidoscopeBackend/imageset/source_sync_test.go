package imageset

import (
	"slices"
	"testing"
)

func TestApplySourceMetadataEmptyNeverOverwrites(t *testing.T) {
	set := ImageSetMongo{
		Title:   "Sunset",
		Authors: []string{"alice"},
		Sources: []SourceInfo{{Title: "Sunset", Description: "from the zip", SourceAuthor: "alice", AuthorID: "9"}},
	}
	fields := applySourceMetadata(&set, 0, SourceInfo{})

	src := set.Sources[0]
	if src.Title != "Sunset" || src.Description != "from the zip" || src.SourceAuthor != "alice" || src.AuthorID != "9" {
		t.Errorf("source = %+v, want every field kept", src)
	}
	if len(fields) != 0 {
		t.Errorf("fields = %v, want nothing changed", fields)
	}
}

func TestApplySourceMetadataTitleFollowsWhileUnchanged(t *testing.T) {
	set := ImageSetMongo{Title: "Old", Sources: []SourceInfo{{Title: "Old"}}}
	fields := applySourceMetadata(&set, 0, SourceInfo{Title: "New", Description: "caption"})

	if set.Title != "New" || fields["title"] != "New" {
		t.Errorf("set Title = %q, fields = %v; want it to follow", set.Title, fields)
	}
	if set.Sources[0].Description != "caption" || set.Description != "" {
		t.Errorf("source description = %q, set description = %q; want only the source updated",
			set.Sources[0].Description, set.Description)
	}

	edited := ImageSetMongo{Title: "My title", Sources: []SourceInfo{{Title: "Old"}}}
	fields = applySourceMetadata(&edited, 0, SourceInfo{Title: "New"})
	if edited.Title != "My title" || fields["title"] != nil {
		t.Errorf("an edited set title was changed to %q", edited.Title)
	}
	if edited.Sources[0].Title != "New" {
		t.Errorf("source Title = %q, want New", edited.Sources[0].Title)
	}
}

func TestApplySourceMetadataAuthor(t *testing.T) {
	tests := []struct {
		name      string
		authors   []string
		oldAuthor string
		want      []string
		changed   bool
	}{
		{"rename in place", []string{"x", "alice", "y"}, "alice", []string{"x", "alice2", "y"}, true},
		{"new name already listed", []string{"alice", "alice2"}, "alice", []string{"alice2"}, true},
		{"source had no author, placeholder replaced", []string{unknownAuthor}, "", []string{"alice2"}, true},
		{"source had no author, name added", []string{"x"}, "", []string{"x", "alice2"}, true},
		{"edited list left alone", []string{"x"}, "alice", []string{"x"}, false},
	}
	for _, tt := range tests {
		set := ImageSetMongo{Authors: slices.Clone(tt.authors), Sources: []SourceInfo{{SourceAuthor: tt.oldAuthor}}}
		fields := applySourceMetadata(&set, 0, SourceInfo{SourceAuthor: "alice2", AuthorID: "10"})

		if !slices.Equal(set.Authors, tt.want) {
			t.Errorf("%s: Authors = %v, want %v", tt.name, set.Authors, tt.want)
		}
		if _, ok := fields["authors"]; ok != tt.changed {
			t.Errorf("%s: authors in fields = %t, want %t", tt.name, ok, tt.changed)
		}
		if set.Sources[0].SourceAuthor != "alice2" || set.Sources[0].AuthorID != "10" {
			t.Errorf("%s: source author = %q/%q, want alice2/10", tt.name, set.Sources[0].SourceAuthor, set.Sources[0].AuthorID)
		}
	}
}
