package services

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"errors"
	"slices"
	"testing"
	"time"

	pixiv "github.com/ryohidaka/go-pixiv"
	pixivmodel "github.com/ryohidaka/go-pixiv/models/appmodel"
)

func TestSavePixivIllustNoURLs(t *testing.T) {
	_, err := savePixivIllust("user1", &pixivmodel.Illust{ID: 1}, false)
	if !errors.Is(err, notification.ErrDownloadFailed) {
		t.Errorf("err = %v, want it marked ErrDownloadFailed", err)
	}
}

func TestBookmarkPageFailureStoresReason(t *testing.T) {
	// DefaultScheduler has no services registered here, so both the session
	// and queueing the end of the sync fail before any DB or network call.
	report := notification.NewImportReport(pixivServiceName)
	done := func(err error) { report.Finish(notification.OutcomeFailed, err) }

	processBookmarkPage("user1", 1, pixiv.Public, 0, done)

	if report.Error != notification.ErrSyncStopped.Error() {
		t.Errorf("report Error = %q, want only %q", report.Error, notification.ErrSyncStopped.Error())
	}
}

func TestTagsChanged(t *testing.T) {
	have := []imageset.SourceTag{{Default: "Cat"}, {Default: "dog"}}
	tests := []struct {
		name    string
		want    []imageset.SourceTag
		changed bool
	}{
		{"a tag pixiv dropped", []imageset.SourceTag{{Default: "cat"}}, false},
		{"a new tag", []imageset.SourceTag{{Default: "cat"}, {Default: "bird"}}, true},
		{"case and spacing only", []imageset.SourceTag{{Default: " CAT "}, {Default: "Dog"}}, false},
	}
	for _, tt := range tests {
		if got := tagsChanged(have, tt.want); got != tt.changed {
			t.Errorf("%s: tagsChanged = %t, want %t", tt.name, got, tt.changed)
		}
	}
}

func TestPixivSourceStaleDayOnlyDate(t *testing.T) {
	// 18:12 JST is 09:12 UTC on the 27th.
	il := pixivmodel.Illust{CreateDate: time.Date(2026, 8, 27, 18, 12, 12, 0, time.FixedZone("JST", 9*60*60))}
	tests := []struct {
		name  string
		day   int
		stale bool
	}{
		{"same UTC day", 27, false},
		{"another day", 26, true},
	}
	for _, tt := range tests {
		src := imageset.SourceInfo{
			Date:        imageset.DayOnlyDate(time.Date(2026, 8, tt.day, 0, 0, 0, 0, time.UTC)),
			LastChecked: time.Now(),
		}
		if got := pixivSourceStale(il, src, false); got != tt.stale {
			t.Errorf("%s: pixivSourceStale = %t, want %t", tt.name, got, tt.stale)
		}
	}
}

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
