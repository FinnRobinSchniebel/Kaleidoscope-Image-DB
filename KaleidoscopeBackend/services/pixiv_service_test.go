package services

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	pixiv "github.com/ryohidaka/go-pixiv"
	pixivmodel "github.com/ryohidaka/go-pixiv/models/appmodel"
)

func TestSavePixivIllustNoURLs(t *testing.T) {
	results := savePixivIllust(context.Background(), "user1", &pixivmodel.Illust{ID: 1}, false)
	if len(results) != 1 || results[0].Kind != notification.ItemFailed || results[0].Reason != notification.ErrDownloadFailed.Error() {
		t.Errorf("results = %+v, want one failed item with the download reason", results)
	}
}

func TestBookmarkPageFailureStoresReason(t *testing.T) {
	// DefaultScheduler has no services registered here, so both the session
	// and queueing the end of the sync fail before any DB or network call.
	report := notification.NewImportReport(pixivServiceName)
	done := func(err error) { report.Finish(notification.OutcomeFailed, err) }

	processBookmarkPage(context.Background(), "user1", 1, pixiv.Public, 0, done)

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
	//storing the set attributes the images; the builder leaves that to it
	if src.Name != pixivServiceName || src.SourceID != "7" || src.AuthorID != "9" || len(src.AttributedTo) != 0 {
		t.Errorf("source = %+v", src)
	}
}

func TestPixivWritesSkipWhenLibraryBusy(t *testing.T) {

	//the lock is held and the task gives up, so neither write reaches the DB
	defer imageset.LockUserSets("pixiv-busy-user")()
	ctx := imageset.WithPause(context.Background(), func(context.Context) error { return errors.New("gave up") })

	missing := markPixivSourceMissing(ctx, "pixiv-busy-user", 5)
	updated := applyPixivSourceUpdate(ctx, "pixiv-busy-user", &pixivmodel.Illust{ID: 5}, false)
	for name, item := range map[string]notification.ItemResult{"missing mark": missing, "metadata update": updated} {
		if item.Kind != notification.ItemSkipped || item.Reason != imageset.ErrLibraryBusy.Error() {
			t.Errorf("%s: %+v, want skipped with the busy reason", name, item)
		}
	}
}

func TestReadPixivAuthor(t *testing.T) {

	if name, id := readPixivAuthor(&pixivmodel.Illust{}); name != "" || id != "" {
		t.Errorf("no user: %q, %q; want both empty", name, id)
	}

	if name, id := readPixivAuthor(&pixivmodel.Illust{User: &pixivmodel.User{ID: 9, Name: "alice"}}); name != "alice" || id != "9" {
		t.Errorf("a user: %q, %q; want alice, 9", name, id)
	}

	set := buildPixivImageSet(&pixivmodel.Illust{ID: 1, Title: "No user"}, "user1", false)
	if src := set.Sources[0]; src.SourceAuthor != "" || src.AuthorID != "" {
		t.Errorf("source without a user: %+v, want no author", src)
	}
}

func TestDownloadPixivImageTimeouts(t *testing.T) {

	oldResponse, oldStall := pixivResponseTimeout, pixivStallTimeout
	pixivResponseTimeout, pixivStallTimeout = 100*time.Millisecond, 100*time.Millisecond
	defer func() { pixivResponseTimeout, pixivStallTimeout = oldResponse, oldStall }()

	//every handler returns once the client goes away, so the server can close
	waitForClient := func(r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/silent.png", func(w http.ResponseWriter, r *http.Request) {
		waitForClient(r)
	})
	mux.HandleFunc("/stalls.png", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("first bytes"))
		w.(http.Flusher).Flush()
		waitForClient(r)
	})
	mux.HandleFunc("/slow.png", func(w http.ResponseWriter, r *http.Request) {
		for range 6 {
			w.Write([]byte("chunk"))
			w.(http.Flusher).Flush()
			time.Sleep(50 * time.Millisecond)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	download := func(ctx context.Context, name string) (string, error) {
		type result struct {
			path string
			err  error
		}
		done := make(chan result, 1)
		go func() {
			path, err := downloadPixivImage(ctx, server.URL+"/"+name, t.TempDir())
			done <- result{path, err}
		}()

		select {
		case r := <-done:
			return r.path, r.err
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: the download never returned", name)
			return "", nil
		}
	}

	if _, err := download(context.Background(), "silent.png"); !errors.Is(err, errDownloadStalled) || !strings.Contains(err.Error(), "no response") {
		t.Errorf("no response: %v, want the response timeout", err)
	}

	if _, err := download(context.Background(), "stalls.png"); !errors.Is(err, errDownloadStalled) || !strings.Contains(err.Error(), "no data") {
		t.Errorf("stalled body: %v, want the stall timeout", err)
	}

	//300 ms in total, longer than the stall timeout, but never 100 ms without data
	path, err := download(context.Background(), "slow.png")
	if err != nil {
		t.Fatalf("slow but steady: %v, want it to finish", err)
	}
	if got, _ := os.ReadFile(path); string(got) != strings.Repeat("chunk", 6) {
		t.Errorf("slow but steady saved %q", got)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := download(cancelled, "slow.png"); !errors.Is(err, context.Canceled) || errors.Is(err, errDownloadStalled) {
		t.Errorf("cancelled: %v, want the cancellation", err)
	}
}
