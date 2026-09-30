package imageset

import (
	"slices"
	"testing"
)

func TestDeriveFromSources(t *testing.T) {
	set := ImageSetMongo{
		Title:       "stale",
		Authors:     []string{"stale"},
		Description: "stale",
		Sources: []SourceInfo{
			{Title: "", SourceAuthor: "bob", Description: "one"},
			{Title: "First", SourceAuthor: "alice"},
			{Title: "Second", SourceAuthor: "bob", Description: "two"},
			{Title: "Third", SourceAuthor: ""},
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

func TestDeriveFromSourcesEmpty(t *testing.T) {
	set := ImageSetMongo{Sources: []SourceInfo{{}}}
	DeriveFromSources(&set)

	if set.Title != "" || set.Authors != nil || set.Description != "" {
		t.Errorf("got Title=%q Authors=%v Description=%q, want all empty", set.Title, set.Authors, set.Description)
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
