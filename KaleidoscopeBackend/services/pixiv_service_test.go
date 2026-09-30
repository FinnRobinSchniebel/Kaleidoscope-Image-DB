package services

import (
	"slices"
	"testing"

	pixivmodel "github.com/ryohidaka/go-pixiv/models/appmodel"
)

func TestBuildPixivImageSetDerivesFromSource(t *testing.T) {
	caption := "a caption"
	illust := &pixivmodel.Illust{
		ID:        7,
		Title:     "Sunset",
		Type:      "illust",
		Caption:   &caption,
		User:      &pixivmodel.User{ID: 9, Name: "alice"},
		PageCount: 2,
	}
	set := buildPixivImageSet(illust, "user1", false)

	if set.Title != "Sunset" {
		t.Errorf("Title = %q, want Sunset", set.Title)
	}
	if !slices.Equal(set.Authors, []string{"alice"}) {
		t.Errorf("Authors = %v, want [alice]", set.Authors)
	}
	if set.Description != caption || set.Sources[0].Description != caption {
		t.Errorf("Description = %q / source %q, want %q on both", set.Description, set.Sources[0].Description, caption)
	}
	if set.Itype != "illust" || set.KscopeUserId != "user1" {
		t.Errorf("Itype = %q, KscopeUserId = %q", set.Itype, set.KscopeUserId)
	}
	src := set.Sources[0]
	if src.Name != pixivServiceName || src.SourceID != "7" || src.AuthorID != "9" || !slices.Equal(src.AttributedTo, []int{0, 1}) {
		t.Errorf("source = %+v", src)
	}
}
