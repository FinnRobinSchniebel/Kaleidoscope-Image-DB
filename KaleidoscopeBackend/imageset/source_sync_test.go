package imageset

import (
	"slices"
	"testing"
	"time"
)

func TestApplySourceMetadataEmptyNeverOverwrites(t *testing.T) {
	date := time.Date(2026, 8, 27, 9, 12, 12, 0, time.UTC)
	set := ImageSetMongo{
		Title:   "Sunset",
		Authors: []string{"alice"},
		Sources: []SourceInfo{{Title: "Sunset", Description: "from the zip", SourceAuthor: "alice", AuthorID: "9", Date: date}},
	}
	fields := applySourceMetadata(&set, 0, SourceInfo{})

	src := set.Sources[0]
	if src.Title != "Sunset" || src.Description != "from the zip" || src.SourceAuthor != "alice" || src.AuthorID != "9" || !src.Date.Equal(date) {
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
		name       string
		sources    []string // SourceAuthor of each source; index 0 gets the update
		incoming   string
		wantSource string
		want       []string
		changed    bool
	}{
		{"rename the sole author", []string{"alice"}, "alice2", "alice2", []string{"alice2"}, true},
		{"rename to another source's author", []string{"alice", "alice2"}, "alice2", "alice2", []string{"alice2"}, true},
		{"same author again", []string{"alice"}, "alice", "alice", []string{"alice"}, false},
		{"unknown keeps the real author", []string{"alice"}, unknownAuthor, "alice", []string{"alice"}, false},
	}
	for _, tt := range tests {
		set := ImageSetMongo{Authors: slices.Clone(tt.sources)}
		for _, author := range tt.sources {
			set.Sources = append(set.Sources, SourceInfo{SourceAuthor: author})
		}
		fields := applySourceMetadata(&set, 0, SourceInfo{SourceAuthor: tt.incoming})

		if !slices.Equal(set.Authors, tt.want) {
			t.Errorf("%s: Authors = %v, want %v", tt.name, set.Authors, tt.want)
		}
		if _, ok := fields["authors"]; ok != tt.changed {
			t.Errorf("%s: authors in fields = %t, want %t", tt.name, ok, tt.changed)
		}
		if set.Sources[0].SourceAuthor != tt.wantSource {
			t.Errorf("%s: source author = %q, want %q", tt.name, set.Sources[0].SourceAuthor, tt.wantSource)
		}
	}
}

func TestSourceDataChanged(t *testing.T) {
	date := time.Date(2026, 8, 27, 9, 12, 12, 0, time.UTC)
	base := SourceInfo{Title: "A", Description: "d", SourceAuthor: "x", AuthorID: "1", Date: date, Tags: []SourceTag{{Default: "cat"}}}
	tests := []struct {
		name   string
		modify func(*SourceInfo)
		want   bool
	}{
		{"identical", func(*SourceInfo) {}, false},
		{"same instant in another zone", func(s *SourceInfo) { s.Date = date.In(time.FixedZone("JST", 9*60*60)) }, false},
		{"only LastChecked", func(s *SourceInfo) { s.LastChecked = date }, false},
		{"only SourceMissing", func(s *SourceInfo) { s.SourceMissing = true }, false},
		{"title", func(s *SourceInfo) { s.Title = "B" }, true},
		{"tag translation", func(s *SourceInfo) { s.Tags = []SourceTag{{Default: "cat", EN: "Cat"}} }, true},
	}
	for _, tt := range tests {
		after := base
		tt.modify(&after)
		if got := sourceDataChanged(base, after); got != tt.want {
			t.Errorf("%s: sourceDataChanged = %t, want %t", tt.name, got, tt.want)
		}
	}
}
