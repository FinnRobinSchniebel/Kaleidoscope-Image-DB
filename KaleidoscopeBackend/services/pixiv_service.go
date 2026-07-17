package services

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"

	pixiv "github.com/ryohidaka/go-pixiv"
	pixivmodel "github.com/ryohidaka/go-pixiv/models/appmodel"
)

const pixivServiceName = "pixiv"

// PixivSession holds active API clients for a user.
// App is nil if no refresh token was stored; Web is nil if no PHPSESSID was stored.
type PixivSession struct {
	App *pixiv.AppPixivAPI
	Web *pixiv.WebPixivAPI
}

// pixivSessions caches open sessions keyed by userId.
var pixivSessions sync.Map

// activeSyncs tracks users who have a bookmark sync currently in progress.
// Cleared when the page chain ends (either normally or on error).
var activeSyncs sync.Map

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
	creds, err := GetServiceCredentials(userId, pixivServiceName)
	if err != nil {
		return nil, fmt.Errorf("pixiv credentials not found: %w", err)
	}
	if creds.Key1 == "" {
		return nil, fmt.Errorf("pixiv requires a refresh token (Key1) or PHPSESSID (Key2)")
	}

	session := &PixivSession{}

	if creds.Key1 != "" {
		session.App, err = pixiv.NewApp(creds.Key1)
		if err != nil {
			return nil, fmt.Errorf("pixiv app API: %w", err)
		}
	}

	// if creds.Key2 != "" {
	// 	session.Web, err = pixiv.NewWebApp(creds.Key2)
	// 	if err != nil {
	// 		return nil, fmt.Errorf("pixiv web API: %w", err)
	// 	}
	// }

	pixivSessions.Store(userId, session)

	return session, nil
}

// ---- Scheduler registration ----

// RegisterPixivService registers the "pixiv" service with the DefaultScheduler
// and hooks credential updates so schedules are applied whenever credentials change.
// Call this once at startup, before DefaultScheduler.Start().
func RegisterPixivService() {
	DefaultScheduler.RegisterService(pixivServiceName, ServiceConfig{
		Delay:          1 * time.Second,
		QueriesPerTurn: 1,
	})
	DefaultScheduler.RegisterCredentialHook(pixivServiceName, func(userId string, creds ExternalApiKeys) {
		InvalidatePixivSession(userId)
		if err := applyPixivSchedule(userId, creds.SyncIntervalHours); err != nil {
			log.Printf("pixiv: failed to apply schedule for %s: %v", userId, err)
		}
	})
	DefaultScheduler.RegisterCredentialTestHook(pixivServiceName, func(userId string, creds ExternalApiKeys) error {
		if creds.Key1 == "" {
			return fmt.Errorf("pixiv requires a refresh token in Key1")
		}
		app, err := pixiv.NewApp(creds.Key1)
		if err != nil {
			return fmt.Errorf("pixiv auth failed: %w", err)
		}
		if _, err := app.IllustDetail(68769799); err != nil {
			return fmt.Errorf("pixiv connection test failed: %w", err)
		}
		return nil
	})
}

// RestorePixivSchedules reads all users with stored pixiv credentials from the
// database and reinstates their periodic sync schedules. Call once at startup
// after RegisterPixivService.
func RestorePixivSchedules() {
	docs, err := GetAllUsersWithService(pixivServiceName)
	if err != nil {
		log.Printf("pixiv: could not restore schedules: %v", err)
		return
	}
	for _, doc := range docs {
		creds := doc.Services[pixivServiceName]
		if err := applyPixivSchedule(doc.UserId, creds.SyncIntervalHours); err != nil {
			log.Printf("pixiv: restore schedule for %s: %v", doc.UserId, err)
		}
	}
	log.Printf("pixiv: restored schedules for %d user(s)", len(docs))
}

// applyPixivSchedule starts (or replaces) the periodic sync for userId.
// 0 cancels any existing schedule. Values under 12 are clamped to 12 hours.
func applyPixivSchedule(userId string, intervalHours int64) error {
	if intervalHours == 0 {
		DefaultScheduler.CancelPeriodic(pixivServiceName, userId)
		return nil
	}
	intervalHours = max(intervalHours, 12)
	return SchedulePeriodicPixivSync(userId, time.Duration(intervalHours)*time.Hour)
}

// SchedulePeriodicPixivSync sets a recurring bookmark sync for userId at the
// given interval. Replaces any existing schedule. Pass interval == 0 to cancel.
func SchedulePeriodicPixivSync(userId string, interval time.Duration) error {
	if interval == 0 {
		DefaultScheduler.CancelPeriodic(pixivServiceName, userId)
		return nil
	}
	return DefaultScheduler.SchedulePeriodic(pixivServiceName, userId, interval, func() {
		if err := SyncPixivBookmarks(userId); err != nil {
			log.Printf("pixiv periodic sync [%s]: %v", userId, err)
		}
	})
}

// ---- Bookmark sync ----

// SyncPixivBookmarks starts a bookmark sync by enqueuing the first page task
// into the scheduler. Subsequent pages are chained automatically, one task per
// scheduler turn, interleaved with any pending illust-fetch tasks.
//
// Prerequisites: Key1 = refresh token, UserName = numeric Pixiv UID.
func SyncPixivBookmarks(userId string) error {
	sess, err := GetPixivSession(userId)
	if err != nil {
		return err
	}
	if sess.App == nil {
		return fmt.Errorf("pixiv bookmark sync requires App API (store a refresh token in Key1)")
	}

	creds, err := GetServiceCredentials(userId, pixivServiceName)
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

	if _, alreadyRunning := activeSyncs.LoadOrStore(userId, struct{}{}); alreadyRunning {
		return fmt.Errorf("pixiv bookmark sync already in progress for this user")
	}

	if err := enqueueBookmarkPage(userId, pixivUID, pixiv.Public, 0); err != nil {
		activeSyncs.Delete(userId)
		return err
	}
	return nil
}

// enqueueBookmarkPage adds a single bookmark-page task to the scheduler.
// maxBookmarkID == 0  is  first page
func enqueueBookmarkPage(userId string, pixivUID uint64, restrict pixiv.Restrict, maxBookmarkID int) error {
	return DefaultScheduler.Enqueue(pixivServiceName, userId, func() error {
		return processBookmarkPage(userId, pixivUID, restrict, maxBookmarkID)
	})
}

// processBookmarkPage fetches one page of bookmarks, queries the DB for only
// those IDs, schedules fetch tasks for missing or changed items, then enqueues
// the next page task. Public pages are followed by private pages.
func processBookmarkPage(userId string, pixivUID uint64, restrict pixiv.Restrict, maxBookmarkID int) error {
	sess, err := GetPixivSession(userId)
	if err != nil {
		activeSyncs.Delete(userId)
		return fmt.Errorf("pixiv session: %w", err)
	}

	opts := pixiv.UserBookmarksIllustOptions{Restrict: &restrict}
	if maxBookmarkID != 0 {
		opts.MaxBookmarkID = &maxBookmarkID
	}

	illusts, next, err := sess.App.UserBookmarksIllust(pixivUID, opts)
	if err != nil {
		activeSyncs.Delete(userId)
		return fmt.Errorf("UserBookmarksIllust (restrict=%s after=%d): %w", restrict, maxBookmarkID, err)
	}

	if len(illusts) > 0 {
		sourceIDs := make([]string, len(illusts))
		for i, il := range illusts {
			sourceIDs[i] = strconv.FormatUint(il.ID, 10)
		}

		existing, dbErr := imageset.GetPixivSourcesByIDs(userId, sourceIDs)
		if dbErr != nil {
			log.Printf("pixiv sync [%s]: DB lookup failed: %v – treating page as new", userId, dbErr)
			existing = nil
		}

		for _, il := range illusts {
			idStr := strconv.FormatUint(il.ID, 10)
			src, exists := existing[idStr]
			if !exists {
				enqueueIllustFetch(userId, il.ID, false)
			} else if illustDiffers(il, src) {
				enqueueIllustFetch(userId, il.ID, true)
			}
		}
	}

	// Chain to the next page, or transition Public→Private, or finish.
	// Clear the active-sync flag whenever we do NOT successfully chain,
	// so a new sync can be started after a failure or on completion.
	var nextErr error
	if next != 0 {
		nextErr = enqueueBookmarkPage(userId, pixivUID, restrict, next)
	} else if restrict == pixiv.Public {
		log.Printf("pixiv sync [%s]: public bookmarks done, starting private", userId)
		nextErr = enqueueBookmarkPage(userId, pixivUID, pixiv.Private, 0)
	} else {
		log.Printf("pixiv sync [%s]: bookmark sync complete", userId)
	}

	if nextErr != nil {
		activeSyncs.Delete(userId)
		return nextErr
	}
	if next == 0 && restrict == pixiv.Private {
		activeSyncs.Delete(userId)
	}
	return nil
}

// TODO: add check for edit date
// illustDiffers reports whether the live Pixiv data differs from what is stored.
func illustDiffers(il pixivmodel.Illust, src imageset.SourceInfo) bool {
	if il.Title != src.Title || il.User.Name != src.SourceAuthor {
		return true
	}
	if strconv.FormatUint(il.User.ID, 10) != src.AuthorID {
		return true
	}
	if !il.CreateDate.Equal(src.Date) {
		return true
	}
	if len(il.Tags) != len(src.Tags) {
		return true
	}
	storedTags := make(map[string]struct{}, len(src.Tags))
	for _, t := range src.Tags {
		storedTags[t] = struct{}{}
	}
	for _, t := range il.Tags {
		if _, ok := storedTags[t.Name]; !ok {
			return true
		}
	}
	return false
}

// ----- Per-illust scheduler tasks ----

func enqueueIllustFetch(userId string, illustID uint64, isUpdate bool) {
	if err := DefaultScheduler.Enqueue(pixivServiceName, userId, func() error {
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
		log.Printf("pixiv: illust %d (%q) has changed - manual review required to update existing DB entry", illustID, illust.Title)
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

	log.Printf("----------ERROR: pixiv: Find Images %d (%q) Missing illustrations ------------------", illust.ID, illust.Title)
	return nil
}

// buildPixivImageSet constructs the ImageSetMongo metadata from a Pixiv Illust.
// Image slices and path are left empty; AddImageSet fills those in.
func buildPixivImageSet(illust *pixivmodel.Illust, userId string) *imageset.ImageSetMongo {
	tags := make([]string, 0, len(illust.Tags)+len(illust.Tools))
	for _, t := range illust.Tags {
		tags = append(tags, t.Name)
	}
	for _, t := range illust.Tools {
		tags = append(tags, t)
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
		Name:         pixivServiceName,
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

// ---- OAuth PKCE token exchange ----

const (
	pixivClientID     = "MOBrBDS8blbauoSck0ZfDbtuzpyT"
	pixivClientSecret = "lsACyCD94FhDUtGTXi3QzcFE2uU1hqtDaKeqrdwj"
	pixivRedirectURI  = "https://app-api.pixiv.net/web/v1/users/auth/pixiv/callback"
	pixivUserAgent    = "PixivAndroidApp/5.0.234 (Android 11; Pixel 5)"
	pixivAuthTokenURL = "https://oauth.secure.pixiv.net/auth/token"
)

// PixivOAuthExchange exchanges a PKCE authorization code for a Pixiv refresh token.
// code is the value from the callback URL; codeVerifier is the secret generated
// by the frontend before the login URL was opened.
func PixivOAuthExchange(code, codeVerifier string) (string, error) {
	body := url.Values{
		"client_id":      {pixivClientID},
		"client_secret":  {pixivClientSecret},
		"code":           {code},
		"code_verifier":  {codeVerifier},
		"grant_type":     {"authorization_code"},
		"include_policy": {"true"},
		"redirect_uri":   {pixivRedirectURI},
	}
	req, err := http.NewRequest(http.MethodPost, pixivAuthTokenURL, strings.NewReader(body.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", pixivUserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		RefreshToken string `json:"refresh_token"`
		Message      string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("pixiv response decode: %w", err)
	}
	if result.RefreshToken == "" {
		return "", fmt.Errorf("pixiv auth failed, no token: %s", result.Message)
	}
	return result.RefreshToken, nil
}
