package services

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
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

	pixiv "github.com/ryohidaka/go-pixiv"
	pixivmodel "github.com/ryohidaka/go-pixiv/models/appmodel"
)

// PixivSession holds active API clients for a user.
type PixivSession struct {
	App *pixiv.AppPixivAPI
	//Web *pixiv.WebPixivAPI
}

// pixivSessions caches open sessions keyed by userId.
var pixivSessions sync.Map

// GetPixivSession returns a cached session or opens a new one from stored credentials.
// Credentials are read from MongoDB under service name "pixiv":
//
//	Key1     → OAuth refresh token  (initialises App API)
//	UserName → numeric Pixiv user ID (required for bookmark sync)
func GetPixivSession(userId string) (*PixivSession, error) {
	if v, ok := pixivSessions.Load(userId); ok {
		return v.(*PixivSession), nil
	}
	return openPixivSession(userId)
}

// InvalidatePixivSession removes a user's cached session.
// Call this after credential changes so the next GetPixivSession re-authenticates.
// Runs under the same per-user lock openPixivSession holds for its whole
// build, so a slow in-flight build can't re-Store a stale session after this
// runs — see Scheduler.WithUserLock.
func InvalidatePixivSession(userId string) {
	_ = DefaultScheduler.WithUserLock(pixivServiceName, userId, func() error {
		pixivSessions.Delete(userId)
		return nil
	})
}

// openPixivSession builds a session from stored credentials and caches it.
// The whole read-credentials/authenticate/store sequence runs under
// WithUserLock so it can't race a concurrent InvalidatePixivSession: either
// this runs entirely before the invalidation (and gets cleared by it, correctly)
// or entirely after (and reflects whatever credentials are current by then).
func openPixivSession(userId string) (*PixivSession, error) {
	var session *PixivSession
	err := DefaultScheduler.WithUserLock(pixivServiceName, userId, func() error {
		creds, err := GetServiceCredentials(userId, pixivServiceName)
		if err != nil {
			return fmt.Errorf("pixiv credentials not found: %w", err)
		}
		if creds.Key1 == "" {
			return fmt.Errorf("pixiv requires an APP refresh token (Key1)")
		}

		app, err := newPixivApp(creds.Key1)
		if err != nil {
			return fmt.Errorf("pixiv APP API: %w", err)
		}

		session = &PixivSession{App: app}
		pixivSessions.Store(userId, session)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return session, nil
}

// newPixivApp builds an App API client with the project's Accept-Language
// header set, so Pixiv returns translated tag names (Tag.TranslatedName).
func newPixivApp(refreshToken string) (*pixiv.AppPixivAPI, error) {
	app, err := pixiv.NewApp(refreshToken)
	if err != nil {
		return nil, err
	}
	app.SetAcceptLanguage(pixivAcceptLanguage)
	return app, nil
}

// ---- ServiceProvider implementation ----

// PixivProvider implements ServiceProvider for the Pixiv integration.
type PixivProvider struct{}

func (p *PixivProvider) Name() string { return pixivServiceName }

func (p *PixivProvider) Config() ServiceConfig {
	return ServiceConfig{Delay: PixivDelaySec * time.Second, QueriesPerTurn: PixivQpT}
}

func (p *PixivProvider) TestCredentials(userId string, creds ExternalApiKeys) error {
	if creds.Key1 == "" {
		return fmt.Errorf("pixiv requires a refresh token")
	}
	app, err := newPixivApp(creds.Key1)
	if err != nil {
		return err
	}
	UID, err := strconv.ParseUint(creds.UserName, 10, 64)
	if err != nil {
		return fmt.Errorf("pixiv user ID could not be parsed into a number")
	}
	if _, _, err := app.UserBookmarksIllust(UID, pixiv.UserBookmarksIllustOptions{}); err != nil {
		return err
	}
	return nil
}

func (p *PixivProvider) OnCredentialsUpdated(userId string, creds ExternalApiKeys) {
	InvalidatePixivSession(userId)
}

func (p *PixivProvider) OnCredentialsRemoved(userId string) {
	InvalidatePixivSession(userId)
}

func (p *PixivProvider) Sync(userId string, done func()) error {
	return SyncPixivBookmarks(userId, done)
}

// ---- Bookmark sync ----

// SyncPixivBookmarks starts a bookmark sync by enqueuing the first page task
// into the scheduler. Subsequent pages are chained automatically, one task per
// scheduler turn, interleaved with any pending illust-fetch tasks.
// Only calls Done when the sync fails before starting.
// Return does not mean the sync has finished, chained tasks must call done on fail or finish.
// Prerequisites: Key1 = refresh token, UserName = numeric Pixiv UID.
func SyncPixivBookmarks(userId string, done func()) error {
	sess, err := GetPixivSession(userId)
	if err != nil {
		done()
		return err
	}
	if sess.App == nil {
		done()
		return fmt.Errorf("pixiv bookmark sync requires App API (store a refresh token in Key1)")
	}

	creds, err := GetServiceCredentials(userId, pixivServiceName)
	if err != nil {
		done()
		return err
	}
	if creds.UserName == "" {
		done()
		return fmt.Errorf("pixiv user ID not set – store your numeric Pixiv UID in the UserName field")
	}
	pixivUID, err := strconv.ParseUint(creds.UserName, 10, 64)
	if err != nil {
		done()
		return fmt.Errorf("invalid pixiv UID %q: %w", creds.UserName, err)
	}

	if err := enqueueBookmarkPage(userId, pixivUID, pixiv.Public, 0, done); err != nil {
		done()
		return err
	}
	return nil
}

// enqueueBookmarkPage adds a single bookmark-page task to the scheduler.
// maxBookmarkID == 0  is  first page
func enqueueBookmarkPage(userId string, pixivUID uint64, restrict pixiv.Restrict, maxBookmarkID int, done func()) error {
	return DefaultScheduler.Enqueue(pixivServiceName, userId, func() error {
		return processBookmarkPage(userId, pixivUID, restrict, maxBookmarkID, done)
	})
}

// processBookmarkPage fetches one page of bookmarks, queries the DB for only
// those IDs, schedules fetch tasks for missing or changed items, then enqueues
// the next page task. Public pages are followed by private pages.
func processBookmarkPage(userId string, pixivUID uint64, restrict pixiv.Restrict, maxBookmarkID int, done func()) error {
	sess, err := GetPixivSession(userId)
	if err != nil {
		done()
		return fmt.Errorf("pixiv session: %w", err)
	}

	opts := pixiv.UserBookmarksIllustOptions{Restrict: &restrict}
	if maxBookmarkID != 0 {
		opts.MaxBookmarkID = &maxBookmarkID
	}

	illusts, next, err := sess.App.UserBookmarksIllust(pixivUID, opts)
	if err != nil {
		done()
		return fmt.Errorf("UserBookmarksIllust (restrict=%s after=%d): %w", restrict, maxBookmarkID, err)
	}

	if len(illusts) > 0 {
		sourceIDs := make([]string, len(illusts))
		for i, il := range illusts {
			sourceIDs[i] = strconv.FormatUint(il.ID, 10)
		}

		existing, dbErr := imageset.GetImageSetsBySourceIDs(userId, pixivServiceName, sourceIDs)
		if dbErr != nil {
			log.Printf("pixiv sync [%s]: DB lookup failed: %v – treating page as new", userId, dbErr)
			existing = nil
		}

		for _, il := range illusts {
			idStr := strconv.FormatUint(il.ID, 10)
			set, exists := existing[idStr]
			if !exists {
				enqueueIllustFetch(userId, il.ID, false)
				continue
			}
			src, idx := sourceByID(set, idStr)

			if idx >= 0 && (sourceChanged(il, src) || src.LastChecked.IsZero()) {
				enqueueIllustFetch(userId, il.ID, true)
			}
		}
	}

	// Chain to the next page, or transition Public→Private, or finish.
	// Call done whenever we do NOT successfully chain, so a new sync can be
	// started after a failure or on completion.
	var nextErr error
	if next != 0 {
		nextErr = enqueueBookmarkPage(userId, pixivUID, restrict, next, done)
	} else if restrict == pixiv.Public {
		log.Printf("pixiv sync [%s]: public bookmarks done, starting private", userId)
		nextErr = enqueueBookmarkPage(userId, pixivUID, pixiv.Private, 0, done)
	} else {
		log.Printf("pixiv sync [%s]: bookmark sync complete", userId)
	}

	if nextErr != nil {
		done()
		return nextErr
	}
	if next == 0 && restrict == pixiv.Private {
		done()
	}
	return nil
}

// sourceChanged reports whether the source has moved on since the last sync.
// Pixiv's App API create_date empirically tracks a work's last edit, not its
// original post date, so this doubles as the sync's overall change gate:
// metadata and image checks only run once this is true.
func sourceChanged(il pixivmodel.Illust, src imageset.SourceInfo) bool {
	return !il.CreateDate.Equal(src.Date)
}

// sourceByID returns the pixiv source within set matching sourceID and its index
// in set.Sources. idx is -1 if no match is found.
func sourceByID(set *imageset.ImageSetMongo, sourceID string) (src imageset.SourceInfo, idx int) {
	for i, s := range set.Sources {
		if s.Name == pixivServiceName && s.SourceID == sourceID {
			return s, i
		}
	}
	return imageset.SourceInfo{}, -1
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
// For changed items it checks whether the images themselves changed and either
// applies the metadata update directly or defers to applyPixivSourceUpdate's
// pending state.
func fetchAndSavePixivIllust(userId string, illustID uint64, isUpdate bool) error {
	sess, err := GetPixivSession(userId)
	if err != nil {
		return fmt.Errorf("pixiv session: %w", err)
	}

	illust, err := sess.App.IllustDetail(illustID)
	if err != nil {
		// note: a 404 here means Pixiv could no longer find the work (confirmed for one
		// known-deleted illust via manual testing), but that isn't yet distinguished from
		// other failure modes through this error alone - see imageset.SourceMissing. This
		// still falls through as an ordinary retryable failure until that's settled.
		return fmt.Errorf("IllustDetail(%d): %w", illustID, err)
	}

	if isUpdate {
		return applyPixivSourceUpdate(userId, illust)
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
	_, _, err = imageset.AddImageSet(iset, media, userId)
	if err != nil {
		return fmt.Errorf("AddImageSet for illust %d: %w", illustID, err)
	}

	log.Printf("pixiv: saved illust %d (%q)", illustID, illust.Title)
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

// illustCheckImageURLs returns a smaller preview URL for every page, for use by
// the hash check: imghash's PHash resizes its input internally, so hashing a
// preview instead of the full original saves network, disk and memory without
// changing the result for a genuinely unchanged image. Falls back to
// illustImageURLs per page wherever a smaller size isn't available.
func illustCheckImageURLs(illust *pixivmodel.Illust) []string {
	if illust.PageCount > 1 {
		urls := make([]string, 0, len(illust.MetaPages))
		for _, p := range illust.MetaPages {
			switch {
			case p.Images.Large != "":
				urls = append(urls, p.Images.Large)
			case p.Images.Original != "":
				urls = append(urls, p.Images.Original)
			}
		}
		return urls
	}
	if illust.ImageURLs != nil && illust.ImageURLs.Large != "" {
		return []string{illust.ImageURLs.Large}
	}
	return illustImageURLs(illust)
}

// buildPixivImageSet constructs the ImageSetMongo metadata from a Pixiv Illust.
// Image slices and path are left empty; AddImageSet fills those in.
func buildPixivImageSet(illust *pixivmodel.Illust, userId string) *imageset.ImageSetMongo {

	pageCount := illust.PageCount
	if pageCount < 1 {
		pageCount = 1
	}
	attributed := make([]int, pageCount)
	for i := range attributed {
		attributed[i] = i
	}

	caption := pixivIllustCaption(illust)

	src := imageset.SourceInfo{
		Name:            pixivServiceName,
		SourceID:        strconv.FormatUint(illust.ID, 10),
		Title:           illust.Title,
		Description:     caption,
		SourceAuthor:    illust.User.Name,
		AuthorID:        strconv.FormatUint(illust.User.ID, 10),
		Tags:            pixivIllustTags(illust),
		Date:            illust.CreateDate,
		AttributedTo:    attributed,
		LastChecked:     time.Now(),
		LastImageUpdate: illust.CreateDate,
	}

	return &imageset.ImageSetMongo{
		Title:        illust.Title,
		Sources:      []imageset.SourceInfo{src},
		Authors:      []string{illust.User.Name},
		Description:  caption,
		Itype:        string(illust.Type),
		KscopeUserId: userId,
	}
}

// pixivIllustTags returns the illust's tags: Default is always the untranslated
// Pixiv tag, EN is Pixiv's own translation when it provides one.
func pixivIllustTags(illust *pixivmodel.Illust) []imageset.SourceTag {
	tags := make([]imageset.SourceTag, 0, len(illust.Tags))
	for _, t := range illust.Tags {
		tag := imageset.SourceTag{Default: t.Name}
		if t.TranslatedName != nil && *t.TranslatedName != "" {
			tag.EN = *t.TranslatedName
		}
		tags = append(tags, tag)
	}
	return tags
}

// pixivIllustCaption dereferences illust.Caption, defaulting to empty.
func pixivIllustCaption(illust *pixivmodel.Illust) string {
	if illust.Caption != nil {
		return *illust.Caption
	}
	return ""
}

// pixivSourceInfo builds a fresh SourceInfo from illust for a source that already
// exists in the DB, preserving its DB sub-id and image attribution from old.
func pixivSourceInfo(illust *pixivmodel.Illust, old imageset.SourceInfo) imageset.SourceInfo {
	return imageset.SourceInfo{
		Name:         pixivServiceName,
		ID:           old.ID,
		SourceID:     strconv.FormatUint(illust.ID, 10),
		Title:        illust.Title,
		Description:  pixivIllustCaption(illust),
		SourceAuthor: illust.User.Name,
		AuthorID:     strconv.FormatUint(illust.User.ID, 10),
		Tags:         pixivIllustTags(illust),
		Date:         illust.CreateDate,
		AttributedTo: old.AttributedTo,
	}
}

// applyPixivSourceUpdate re-fetches the existing set for illust's source, checks
// whether its images changed, and either applies the metadata update directly or
// records a pending state ahead of a not-yet-built approval flow. Title and
// description are withheld along with images when a change is pending, since they
// may only make sense together with the new images.
func applyPixivSourceUpdate(userId string, illust *pixivmodel.Illust) error {
	sourceID := strconv.FormatUint(illust.ID, 10)
	set, ok, err := imageset.GetImageSetBySourceID(userId, pixivServiceName, sourceID)
	if err != nil {
		return fmt.Errorf("look up existing set for illust %d: %w", illust.ID, err)
	}
	if !ok {
		return fmt.Errorf("illust %d: flagged as changed but no existing set found", illust.ID)
	}
	_, idx := sourceByID(set, sourceID)
	if idx < 0 {
		return fmt.Errorf("illust %d: source vanished from its own set between sync passes", illust.ID)
	}

	// changed, err := imagesChanged(illust, set, idx)
	// if err != nil {
	// 	return fmt.Errorf("checking images for illust %d: %w", illust.ID, err)
	// }

	checkedAt := time.Now()
	// if changed {
	// 	log.Printf("pixiv: illust %d (%q) images changed - deferring, manual review required", illust.ID, illust.Title)
	// 	return imageset.MarkSourcePendingImageChange(set, idx, illust.CreateDate, checkedAt)
	// }

	newSrc := pixivSourceInfo(illust, set.Sources[idx])
	if err := imageset.ApplySourceMetadataUpdate(set, idx, newSrc, checkedAt, userId); err != nil {
		return fmt.Errorf("applying metadata update for illust %d: %w", illust.ID, err)
	}
	log.Printf("pixiv: updated illust %d (%q)", illust.ID, illust.Title)
	return nil
}

// imagesChanged reports whether illust's images likely differ from what's stored
// for set.Sources[idx]: either the page count changed, or a downloaded page's hash
// no longer matches. Hashing is the expensive path and only runs once the caller's
// date gate already found something different, keeping unchanged syncs cheap.
func imagesChanged(illust *pixivmodel.Illust, set *imageset.ImageSetMongo, idx int) (bool, error) {
	src := set.Sources[idx]
	if illust.PageCount != len(src.AttributedTo) {
		return true, nil
	}
	return imageHashesDiffer(illust, set, src.AttributedTo)
}

// imageHashesDiffer downloads a small preview of each of illust's current pages
// and compares its perceptual hash against the stored image at the matching
// attributedTo index. Uses illustCheckImageURLs rather than full resolution, since
// the hash algorithm resizes its input internally regardless.
func imageHashesDiffer(illust *pixivmodel.Illust, set *imageset.ImageSetMongo, attributedTo []int) (bool, error) {
	urls := illustCheckImageURLs(illust)
	if len(urls) != len(attributedTo) {
		return true, nil
	}

	tmpDir, err := os.MkdirTemp("", fmt.Sprintf("pixiv_hashcheck_%d_*", illust.ID))
	if err != nil {
		return false, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	for i, url := range urls {
		imgIndex := attributedTo[i]
		if imgIndex < 0 || imgIndex >= len(set.Image) {
			return true, nil
		}

		path, err := downloadPixivImage(url, tmpDir)
		if err != nil {
			return false, fmt.Errorf("download %s: %w", url, err)
		}

		f, err := os.Open(path)
		if err != nil {
			return false, fmt.Errorf("open %s: %w", path, err)
		}
		img, _, decodeErr := image.Decode(f)
		f.Close()
		if decodeErr != nil {
			return false, fmt.Errorf("decode %s: %w", path, decodeErr)
		}

		if imageset.HashImage(img) != set.Image[imgIndex].ImageHash {
			return true, nil
		}
	}
	return false, nil
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
