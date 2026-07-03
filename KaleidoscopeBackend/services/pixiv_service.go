package services

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"

	pixiv "github.com/ryohidaka/go-pixiv"
	pixivmodel "github.com/ryohidaka/go-pixiv/models/appmodel"
)

// PixivSession holds active API clients for a user.
// App is nil if no refresh token was stored; Web is nil if no PHPSESSID was stored.
type PixivSession struct {
	App *pixiv.AppPixivAPI
	Web *pixiv.WebPixivAPI
}

// pixivSessions caches open sessions keyed by userId.
var pixivSessions sync.Map

// GetPixivSession returns a cached session or opens a new one from stored credentials.
// Credentials are read from MongoDB under service name "pixiv":
//
//	Key1     → OAuth refresh token  (initialises App API)
//	Key2     → PHPSESSID cookie     (initialises Web API)
//	UserName → numeric Pixiv user ID (required for bookmark sync)
func GetPixivSession(userId string) (*PixivSession, error) {
	if v, ok := pixivSessions.Load(userId); ok {
		return v.(*PixivSession), nil
	}
	return openPixivSession(userId)
}

// InvalidatePixivSession removes a user's cached session.
// Call this after credential changes so the next GetPixivSession re-authenticates.
func InvalidatePixivSession(userId string) {
	pixivSessions.Delete(userId)
}

func openPixivSession(userId string) (*PixivSession, error) {
	creds, err := GetServiceCredentials(userId, "pixiv")
	if err != nil {
		return nil, fmt.Errorf("pixiv credentials not found: %w", err)
	}
	if creds.Key1 == "" && creds.Key2 == "" {
		return nil, fmt.Errorf("pixiv requires a refresh token (Key1) or PHPSESSID (Key2)")
	}

	session := &PixivSession{}

	if creds.Key1 != "" {
		session.App, err = pixiv.NewApp(creds.Key1)
		if err != nil {
			return nil, fmt.Errorf("pixiv app API: %w", err)
		}
	}

	if creds.Key2 != "" {
		session.Web, err = pixiv.NewWebApp(creds.Key2)
		if err != nil {
			return nil, fmt.Errorf("pixiv web API: %w", err)
		}
	}

	pixivSessions.Store(userId, session)
	return session, nil
}

// ─── Scheduler registration ──────────────────────────────────────────────────

// RegisterPixivService registers the "pixiv" service with the DefaultScheduler.
// Call this once at startup, before DefaultScheduler.Start().
func RegisterPixivService() {
	DefaultScheduler.RegisterService("pixiv", ServiceConfig{
		Delay:          1 * time.Second,
		QueriesPerTurn: 1,
	})
}

// ─── Bookmark sync ───────────────────────────────────────────────────────────

// bookmarkEntry is the minimal data we need per illust during the collection
// phase: enough to detect whether an item is new or has changed since it was
// last saved to the database.
type bookmarkEntry struct {
	IllustID   uint64
	CreateDate time.Time
	Title      string
	AuthorName string
	AuthorID   string
	Tags       []string
}

// SyncPixivBookmarks starts an asynchronous bookmark sync for userId.
// It pages through both public and private bookmarks, checks the database for
// entries that are missing or differ from the live data, then enqueues
// individual fetch-and-save tasks for each item that needs work.
//
// Prerequisites – the user must have registered their Pixiv credentials with:
//
//	Key1     = OAuth refresh token
//	UserName = numeric Pixiv user ID
func SyncPixivBookmarks(userId string) error {
	sess, err := GetPixivSession(userId)
	if err != nil {
		return err
	}
	if sess.App == nil {
		return fmt.Errorf("pixiv bookmark sync requires App API (store a refresh token in Key1)")
	}

	creds, err := GetServiceCredentials(userId, "pixiv")
	if err != nil {
		return err
	}
	if creds.UserName == "" {
		return fmt.Errorf("pixiv user ID not set – store your numeric Pixiv UID in the UserName field")
	}
	pixivUID, err := strconv.ParseUint(creds.UserName, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid pixiv UID %q: %w", creds.UserName, err)
	}

	go runBookmarkSync(userId, sess, pixivUID)
	return nil
}

func runBookmarkSync(userId string, sess *PixivSession, pixivUID uint64) {
	log.Printf("pixiv sync [%s]: collecting bookmarks for UID %d", userId, pixivUID)

	entries, err := collectBookmarkEntries(sess, pixivUID)
	if err != nil {
		log.Printf("pixiv sync [%s]: bookmark collection error: %v", userId, err)
		// continue with whatever was collected before the error
	}
	log.Printf("pixiv sync [%s]: collected %d bookmarked items", userId, len(entries))
	if len(entries) == 0 {
		return
	}

	existing, err := imageset.GetPixivSources(userId)
	if err != nil {
		log.Printf("pixiv sync [%s]: DB query failed: %v", userId, err)
		return
	}

	var newCount, updateCount int
	for _, e := range entries {
		idStr := strconv.FormatUint(e.IllustID, 10)
		src, exists := existing[idStr]
		if !exists {
			newCount++
			enqueueIllustFetch(userId, e.IllustID, false)
			continue
		}
		if bookmarkDiffers(e, src) {
			updateCount++
			enqueueIllustFetch(userId, e.IllustID, true)
		}
	}
	log.Printf("pixiv sync [%s]: queued %d new, %d changed", userId, newCount, updateCount)
}

// bookmarkDiffers returns true when the live Pixiv data differs from what is
// stored in SourceInfo, meaning the DB entry may need attention.
func bookmarkDiffers(e bookmarkEntry, src imageset.SourceInfo) bool {
	if e.Title != src.Title || e.AuthorName != src.SourceAuthor || e.AuthorID != src.AuthorID {
		return true
	}
	if !e.CreateDate.Equal(src.Date) {
		return true
	}
	if len(e.Tags) != len(src.Tags) {
		return true
	}
	existing := make(map[string]struct{}, len(src.Tags))
	for _, t := range src.Tags {
		existing[t] = struct{}{}
	}
	for _, t := range e.Tags {
		if _, ok := existing[t]; !ok {
			return true
		}
	}
	return false
}

// collectBookmarkEntries pages through public then private bookmarks and
// returns a deduplicated list. A 1-second pause is inserted between pages to
// respect Pixiv's rate limits during the collection phase; individual illust
// detail fetches are rate-limited by the scheduler instead.
func collectBookmarkEntries(sess *PixivSession, pixivUID uint64) ([]bookmarkEntry, error) {
	seen := make(map[uint64]struct{})
	var all []bookmarkEntry
	var lastErr error

	for _, r := range []pixiv.Restrict{pixiv.Public, pixiv.Private} {
		entries, err := collectOneRestrict(sess, pixivUID, r, seen)
		all = append(all, entries...)
		if err != nil {
			lastErr = err
		}
	}
	return all, lastErr
}

func collectOneRestrict(sess *PixivSession, pixivUID uint64, restrict pixiv.Restrict, seen map[uint64]struct{}) ([]bookmarkEntry, error) {
	var entries []bookmarkEntry
	var maxID *int

	for {
		opts := pixiv.UserBookmarksIllustOptions{
			Restrict:      &restrict,
			MaxBookmarkID: maxID,
		}
		illusts, next, err := sess.App.UserBookmarksIllust(pixivUID, opts)
		if err != nil {
			return entries, fmt.Errorf("UserBookmarksIllust (restrict=%s): %w", restrict, err)
		}

		for _, il := range illusts {
			if _, dup := seen[il.ID]; dup {
				continue
			}
			seen[il.ID] = struct{}{}

			e := bookmarkEntry{
				IllustID:   il.ID,
				CreateDate: il.CreateDate,
				Title:      il.Title,
				AuthorID:   strconv.FormatUint(il.User.ID, 10),
				AuthorName: il.User.Name,
			}
			for _, t := range il.Tags {
				e.Tags = append(e.Tags, t.Name)
			}
			entries = append(entries, e)
		}

		if next == 0 {
			break
		}
		maxID = &next
		time.Sleep(1 * time.Second)
	}
	return entries, nil
}

// ─── Per-illust scheduler tasks ──────────────────────────────────────────────

func enqueueIllustFetch(userId string, illustID uint64, isUpdate bool) {
	if err := DefaultScheduler.Enqueue("pixiv", userId, func() error {
		return fetchAndSavePixivIllust(userId, illustID, isUpdate)
	}); err != nil {
		log.Printf("pixiv: failed to enqueue illust %d: %v", illustID, err)
	}
}

// fetchAndSavePixivIllust is executed by the scheduler.
// For new items it downloads all pages and saves them via AddImageSet.
// For changed items it logs the detected change and skips auto-update,
// since altering existing entries requires user review.
func fetchAndSavePixivIllust(userId string, illustID uint64, isUpdate bool) error {
	sess, err := GetPixivSession(userId)
	if err != nil {
		return fmt.Errorf("pixiv session: %w", err)
	}

	illust, err := sess.App.IllustDetail(illustID)
	if err != nil {
		return fmt.Errorf("IllustDetail(%d): %w", illustID, err)
	}

	if isUpdate {
		log.Printf("pixiv: illust %d (%q) has changed – manual review required to update existing DB entry", illustID, illust.Title)
		return nil
	}

	// Download all pages to a temporary directory, then pass them to AddImageSet.
	tmpDir, err := os.MkdirTemp("", fmt.Sprintf("pixiv_%d_*", illustID))
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	urls := illustImageURLs(illust)
	if len(urls) == 0 {
		return fmt.Errorf("illust %d: no downloadable image URLs", illustID)
	}

	media := make([]imageset.MediaSource, 0, len(urls))
	for _, url := range urls {
		path, err := downloadPixivImage(url, tmpDir)
		if err != nil {
			return fmt.Errorf("download %s: %w", url, err)
		}
		media = append(media, imageset.DiskSource{Path: path})
	}

	iset := buildPixivImageSet(illust, userId)
	_, _, resp := imageset.AddImageSet(iset, media, userId)
	if resp.ErrorCode >= 400 {
		return fmt.Errorf("AddImageSet for illust %d: %s", illustID, resp.ErrorString)
	}

	log.Printf("pixiv: saved illust %d (%q) status=%d", illustID, illust.Title, resp.ErrorCode)
	return nil
}

// illustImageURLs returns the original-resolution download URLs for every page
// of an illustration. Single-page works use MetaSinglePage; multi-page works
// use MetaPages.
func illustImageURLs(illust *pixivmodel.Illust) []string {
	if illust.PageCount > 1 {
		urls := make([]string, 0, len(illust.MetaPages))
		for _, p := range illust.MetaPages {
			if p.Images.Original != "" {
				urls = append(urls, p.Images.Original)
			}
		}
		return urls
	}
	if illust.MetaSinglePage != nil && illust.MetaSinglePage.OriginalImageURL != "" {
		return []string{illust.MetaSinglePage.OriginalImageURL}
	}
	// fallback to largest available size
	if illust.ImageURLs != nil && illust.ImageURLs.Large != "" {
		return []string{illust.ImageURLs.Large}
	}
	return nil
}

// buildPixivImageSet constructs the ImageSetMongo metadata from a Pixiv Illust.
// Image slices and path are left empty; AddImageSet fills those in.
func buildPixivImageSet(illust *pixivmodel.Illust, userId string) *imageset.ImageSetMongo {
	tags := make([]string, 0, len(illust.Tags))
	for _, t := range illust.Tags {
		tags = append(tags, t.Name)
	}

	pageCount := illust.PageCount
	if pageCount < 1 {
		pageCount = 1
	}
	attributed := make([]int, pageCount)
	for i := range attributed {
		attributed[i] = i
	}

	src := imageset.SourceInfo{
		Name:         "pixiv",
		SourceID:     strconv.FormatUint(illust.ID, 10),
		Title:        illust.Title,
		SourceAuthor: illust.User.Name,
		AuthorID:     strconv.FormatUint(illust.User.ID, 10),
		Tags:         tags,
		Date:         illust.CreateDate,
		AttributedTo: attributed,
	}

	var caption string
	if illust.Caption != nil {
		caption = *illust.Caption
	}

	return &imageset.ImageSetMongo{
		Title:        illust.Title,
		Tags:         tags,
		Sources:      []imageset.SourceInfo{src},
		Authors:      []string{illust.User.Name},
		Description:  caption,
		Itype:        string(illust.Type),
		KscopeUserId: userId,
	}
}

// downloadPixivImage fetches a single Pixiv image URL into dir and returns the
// local file path. Pixiv image servers require Referer: https://www.pixiv.net/
// which differs from the App API host used by the library's own downloader.
func downloadPixivImage(url, dir string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Referer", "https://www.pixiv.net/")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	dest := filepath.Join(dir, filepath.Base(url))
	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", err
	}
	return dest, nil
}
