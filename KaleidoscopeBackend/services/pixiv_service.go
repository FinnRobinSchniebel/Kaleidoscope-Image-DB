package services

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"encoding/json"
	"errors"
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
	"go.mongodb.org/mongo-driver/v2/bson"
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
		if errors.Is(err, ErrServiceNotConnected) {
			return fmt.Errorf("%w: %w", notification.ErrServiceSignIn, err)
		}
		if err != nil {
			return fmt.Errorf("reading pixiv credentials: %w", err)
		}
		if creds.Key1 == "" {
			return fmt.Errorf("%w: pixiv requires an APP refresh token (Key1)", notification.ErrServiceSignIn)
		}

		app, err := newPixivApp(creds.Key1)
		if err != nil {
			return fmt.Errorf("%w: pixiv APP API: %w", notification.ErrServiceSignIn, err)
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

func (p *PixivProvider) Sync(userId string, done func(error)) error {
	return SyncPixivBookmarks(userId, done)
}

// ---- Bookmark sync ----

// SyncPixivBookmarks starts a bookmark sync by enqueuing the first page task
// into the scheduler. Subsequent pages are chained automatically, one task per
// scheduler turn, queued behind the previous page's new-item save tasks.
// Only calls done itself when the sync fails before starting.
// Return does not mean the sync has finished, chained tasks must call done on fail or finish.
// Prerequisites: Key1 = refresh token, UserName = numeric Pixiv UID.
func SyncPixivBookmarks(userId string, done func(error)) (err error) {
	defer func() {
		if err != nil {
			done(err)
		}
	}()

	sess, err := GetPixivSession(userId)
	if err != nil {
		return err
	}
	if sess.App == nil {
		return fmt.Errorf("%w: pixiv bookmark sync requires App API (store a refresh token in Key1)", notification.ErrServiceSignIn)
	}

	creds, err := GetServiceCredentials(userId, pixivServiceName)
	if errors.Is(err, ErrServiceNotConnected) {
		return fmt.Errorf("%w: %w", notification.ErrServiceSignIn, err)
	}
	if err != nil {
		return err
	}
	if creds.UserName == "" {
		return fmt.Errorf("%w: pixiv user ID not set – store your numeric Pixiv UID in the UserName field", notification.ErrServiceSettings)
	}
	pixivUID, err := strconv.ParseUint(creds.UserName, 10, 64)
	if err != nil {
		return fmt.Errorf("%w: invalid pixiv UID %q: %w", notification.ErrServiceSettings, creds.UserName, err)
	}

	if err := enqueueBookmarkPage(userId, pixivUID, pixiv.Public, 0, done); err != nil {
		return fmt.Errorf("%w: %w", notification.ErrSyncStopped, err)
	}
	return nil
}

// enqueueBookmarkPage adds a single bookmark-page task to the scheduler.
// maxBookmarkID == 0  is  first page
func enqueueBookmarkPage(userId string, pixivUID uint64, restrict pixiv.Restrict, maxBookmarkID int, done func(error)) error {
	return DefaultScheduler.Enqueue(pixivServiceName, userId, func() ([]notification.ItemResult, error) {
		return processBookmarkPage(userId, pixivUID, restrict, maxBookmarkID, done), nil
	})
}

// processBookmarkPage fetches one page of bookmarks, processes its items, then
// enqueues the next page task. Public pages are followed by private pages.
// Returns the page's item results; new items report from their own save task.
// A failure is handed to done rather than returned, so it's logged only once.
func processBookmarkPage(userId string, pixivUID uint64, restrict pixiv.Restrict, maxBookmarkID int, done func(error)) []notification.ItemResult {
	sess, err := GetPixivSession(userId)
	if err != nil {
		finishPixivSync(userId, done, fmt.Errorf("pixiv session: %w", err))
		return nil
	}

	opts := pixiv.UserBookmarksIllustOptions{Restrict: &restrict}
	if maxBookmarkID != 0 {
		opts.MaxBookmarkID = &maxBookmarkID
	}

	illusts, next, err := sess.App.UserBookmarksIllust(pixivUID, opts)
	if err != nil {
		finishPixivSync(userId, done, fmt.Errorf("%w: UserBookmarksIllust (restrict=%s after=%d): %w", notification.ErrServiceRequest, restrict, maxBookmarkID, err))
		return nil
	}

	var results []notification.ItemResult
	if len(illusts) > 0 {
		results = processBookmarkItems(userId, illusts, restrict == pixiv.Private)
	}

	// Chain to the next page, or move from public to private, or finish.
	var nextErr error
	if next != 0 {
		nextErr = enqueueBookmarkPage(userId, pixivUID, restrict, next, done)
	} else if restrict == pixiv.Public {
		log.Printf("pixiv sync [%s]: public bookmarks done, starting private", userId)
		nextErr = enqueueBookmarkPage(userId, pixivUID, pixiv.Private, 0, done)
	} else {
		log.Printf("pixiv sync [%s]: bookmark pages done, finishing queued saves", userId)
		finishPixivSync(userId, done, nil)
	}

	if nextErr != nil {
		finishPixivSync(userId, done, fmt.Errorf("%w: %w", notification.ErrSyncStopped, nextErr))
	}
	return results
}

// processBookmarkItems uses the page's DB snapshot only to decide what to do;
// every write re-reads its set first. Returns one result per item, except new
// items that were queued: their save task reports them.
func processBookmarkItems(userId string, illusts []pixivmodel.Illust, isPrivate bool) []notification.ItemResult {
	sourceIDs := make([]string, len(illusts))
	for i, il := range illusts {
		sourceIDs[i] = strconv.FormatUint(il.ID, 10)
	}

	existing, err := imageset.GetImageSetsBySourceIDs(userId, pixivServiceName, sourceIDs)
	if err != nil {
		// Can't tell new from existing, so skip rather than re-import duplicates.
		err = fmt.Errorf("%w: looking up stored sets: %w", notification.ErrLibraryAccess, err)
		results := make([]notification.ItemResult, len(sourceIDs))
		for i, id := range sourceIDs {
			results[i] = notification.FailedItem(userId, id, bson.NilObjectID, err)
		}
		return results
	}

	results := make([]notification.ItemResult, 0, len(illusts))
	for _, il := range illusts {
		idStr := strconv.FormatUint(il.ID, 10)
		set, exists := existing[idStr]

		// Must stay first: placeholder entries look changed to every later check.
		if !il.Visible {
			switch {
			case !illustRemoved(il):
				results = append(results, notification.ItemResult{Kind: notification.ItemSkipped, Ref: idStr, Reason: "hidden by the account's filter"})
			case exists:
				results = append(results, markPixivSourceMissing(userId, il.ID))
			default:
				results = append(results, notification.ItemResult{Kind: notification.ItemSkipped, Ref: idStr, Reason: "removed at source"})
			}
			continue
		}

		if !exists {
			if err := enqueueNewIllust(userId, il, isPrivate); err != nil {
				err = fmt.Errorf("%w: queueing the download: %w", notification.ErrSyncStopped, err)
				results = append(results, notification.FailedItem(userId, idStr, bson.NilObjectID, err))
			}
			continue
		}

		src, idx := sourceByID(set, idStr)
		if idx < 0 || !pixivSourceStale(il, src, isPrivate) {
			results = append(results, notification.ItemResult{Kind: notification.ItemUnchanged, Ref: idStr})
			continue
		}
		results = append(results, applyPixivSourceUpdate(userId, &il, isPrivate))
	}
	return results
}

// finishPixivSync queues done(err) behind the user's pending save tasks, or
// calls it directly, with the queueing error added, if queueing fails.
func finishPixivSync(userId string, done func(error), err error) {
	if queueErr := DefaultScheduler.Enqueue(pixivServiceName, userId, func() ([]notification.ItemResult, error) {
		log.Printf("pixiv sync [%s]: bookmark sync ended", userId)
		done(err)
		return nil, nil
	}); queueErr != nil {
		done(errors.Join(err, fmt.Errorf("%w: %w", notification.ErrSyncStopped, queueErr)))
	}
}

// pixivSourceStale reports whether src needs re-applying from listing item il,
// which must be visible. Tags are compared separately because a tag edit or
// bookmark visibility flip doesn't move create_date.
func pixivSourceStale(il pixivmodel.Illust, src imageset.SourceInfo, isPrivate bool) bool {
	return src.LastChecked.IsZero() ||
		src.SourceMissing ||
		!imageset.SourceDateMatches(src.Date, il.CreateDate) ||
		tagsChanged(src.Tags, pixivIllustTags(&il, isPrivate))
}

// illustRemoved reports whether il is Pixiv's placeholder for a deleted or
// author-privated work. Only limit_unknown matches, so any other placeholder
// (e.g. R-18 filtered) is never marked missing.
func illustRemoved(il pixivmodel.Illust) bool {
	return !il.Visible && il.ImageURLs != nil &&
		strings.Contains(il.ImageURLs.SquareMedium, "limit_unknown")
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

// ----- Per-illust work ----

// enqueueNewIllust is queued rather than run inline because it downloads images.
// Its task reports the illust as added or failed.
func enqueueNewIllust(userId string, illust pixivmodel.Illust, isPrivate bool) error {
	ref := strconv.FormatUint(illust.ID, 10)
	return DefaultScheduler.Enqueue(pixivServiceName, userId, func() ([]notification.ItemResult, error) {
		setID, err := savePixivIllust(userId, &illust, isPrivate)
		if err != nil {
			return []notification.ItemResult{notification.FailedItem(userId, ref, bson.NilObjectID, err)}, nil
		}
		return []notification.ItemResult{{Kind: notification.ItemAdded, Ref: ref, SetID: setID}}, nil
	})
}

// markPixivSourceMissing re-reads the set right before writing, to keep the
// window for overwriting a concurrent edit small. Only a source that wasn't
// missing yet is reported as missing.
func markPixivSourceMissing(userId string, illustID uint64) notification.ItemResult {
	sourceID := strconv.FormatUint(illustID, 10)
	set, ok, err := imageset.GetImageSetBySourceID(userId, pixivServiceName, sourceID)
	if err != nil {
		return notification.FailedItem(userId, sourceID, bson.NilObjectID, fmt.Errorf("%w: looking up removed illust: %w", notification.ErrLibraryAccess, err))
	}
	if !ok {
		return notification.ItemResult{Kind: notification.ItemSkipped, Ref: sourceID, Reason: "removed at source"}
	}
	_, idx := sourceByID(set, sourceID)
	if idx < 0 {
		return notification.ItemResult{Kind: notification.ItemUnchanged, Ref: sourceID}
	}

	newlyMissing, err := imageset.MarkSourceMissing(set, idx, time.Now())
	switch {
	case err != nil:
		return notification.FailedItem(userId, sourceID, set.ID, fmt.Errorf("%w: marking missing: %w", notification.ErrLibraryAccess, err))
	case newlyMissing:
		return notification.ItemResult{Kind: notification.ItemMissing, Ref: sourceID, SetID: set.ID}
	default:
		return notification.ItemResult{Kind: notification.ItemUnchanged, Ref: sourceID}
	}
}

// tagsChanged reports whether want has a tag missing from have, compared by
// normalized Default text only. Stored tags are add-only, so a tag the source
// dropped isn't a change.
func tagsChanged(have, want []imageset.SourceTag) bool {
	haveSet := make(map[string]struct{}, len(have))
	for _, t := range have {
		haveSet[imageset.NormalizeTagText(t.Default)] = struct{}{}
	}
	for _, t := range want {
		if _, ok := haveSet[imageset.NormalizeTagText(t.Default)]; !ok {
			return true
		}
	}
	return false
}

// savePixivIllust must run serially (as a scheduler task): AddImageSet's
// duplicate-hash check relies on it. Returns the new set's ID.
func savePixivIllust(userId string, illust *pixivmodel.Illust, isPrivate bool) (bson.ObjectID, error) {
	illustID := illust.ID

	// Download all pages to a temporary directory, then pass them to AddImageSet.
	tmpDir, err := os.MkdirTemp("", fmt.Sprintf("pixiv_%d_*", illustID))
	if err != nil {
		return bson.NilObjectID, fmt.Errorf("%w: create temp dir: %w", notification.ErrSaveFailed, err)
	}
	defer os.RemoveAll(tmpDir)

	urls := illustImageURLs(illust)
	if len(urls) == 0 {
		return bson.NilObjectID, fmt.Errorf("%w: illust %d: no downloadable image URLs", notification.ErrDownloadFailed, illustID)
	}

	media := make([]imageset.MediaSource, 0, len(urls))
	for _, url := range urls {
		path, err := downloadPixivImage(url, tmpDir)
		if err != nil {
			return bson.NilObjectID, fmt.Errorf("%w: download %s: %w", notification.ErrDownloadFailed, url, err)
		}
		media = append(media, imageset.DiskSource{Path: path})
	}

	iset := buildPixivImageSet(illust, userId, isPrivate)
	_, _, err = imageset.AddImageSet(iset, media, userId)
	if err != nil {
		return bson.NilObjectID, fmt.Errorf("%w: AddImageSet for illust %d: %w", notification.ErrSaveFailed, illustID, err)
	}

	log.Printf("pixiv: saved illust %d (%q)", illustID, illust.Title)
	return iset.ID, nil
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
func buildPixivImageSet(illust *pixivmodel.Illust, userId string, isPrivate bool) *imageset.ImageSetMongo {

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
		Description:  pixivIllustCaption(illust),
		SourceAuthor: illust.User.Name,
		AuthorID:     strconv.FormatUint(illust.User.ID, 10),
		Tags:         pixivIllustTags(illust, isPrivate),
		Date:         illust.CreateDate,
		AttributedTo: attributed,
		LastChecked:  time.Now(),
	}

	set := &imageset.ImageSetMongo{
		Sources:      []imageset.SourceInfo{src},
		Itype:        string(illust.Type),
		KscopeUserId: userId,
	}
	imageset.DeriveFromSources(set)
	return set
}

// pixivIllustTags returns the illust's tags: Default is always the untranslated
// Pixiv tag, EN is Pixiv's own translation when it provides one. isPrivate
// also appends pixivPrivatedTag.
func pixivIllustTags(illust *pixivmodel.Illust, isPrivate bool) []imageset.SourceTag {
	tags := make([]imageset.SourceTag, 0, len(illust.Tags)+1)
	for _, t := range illust.Tags {
		tag := imageset.SourceTag{Default: t.Name}
		if t.TranslatedName != nil && *t.TranslatedName != "" {
			tag.EN = *t.TranslatedName
		}
		tags = append(tags, tag)
	}
	if isPrivate {
		tags = append(tags, imageset.SourceTag{Default: pixivPrivatedTag})
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
func pixivSourceInfo(illust *pixivmodel.Illust, old imageset.SourceInfo, isPrivate bool) imageset.SourceInfo {
	return imageset.SourceInfo{
		Name:         pixivServiceName,
		ID:           old.ID,
		SourceID:     strconv.FormatUint(illust.ID, 10),
		Title:        illust.Title,
		Description:  pixivIllustCaption(illust),
		SourceAuthor: illust.User.Name,
		AuthorID:     strconv.FormatUint(illust.User.ID, 10),
		Tags:         pixivIllustTags(illust, isPrivate),
		Date:         illust.CreateDate,
		AttributedTo: old.AttributedTo,
	}
}

// applyPixivSourceUpdate applies illust's metadata to its stored set,
// re-reading the set right before writing to keep the window for overwriting
// a concurrent edit small. A re-apply of identical values reports unchanged, and
// a set the user changed since the page was read is skipped.
func applyPixivSourceUpdate(userId string, illust *pixivmodel.Illust, isPrivate bool) notification.ItemResult {
	sourceID := strconv.FormatUint(illust.ID, 10)
	set, ok, err := imageset.GetImageSetBySourceID(userId, pixivServiceName, sourceID)
	if err != nil {
		return notification.FailedItem(userId, sourceID, bson.NilObjectID, fmt.Errorf("%w: looking up the stored set: %w", notification.ErrLibraryAccess, err))
	}
	if !ok {
		return notification.ItemResult{Kind: notification.ItemSkipped, Ref: sourceID, Reason: "the set changed during the sync"}
	}
	_, idx := sourceByID(set, sourceID)
	if idx < 0 {
		return notification.ItemResult{Kind: notification.ItemSkipped, Ref: sourceID, SetID: set.ID, Reason: "the set changed during the sync"}
	}

	// Re-enabling this downloads images, so the CreateDate-changed case must
	// then move to a queued task instead of running inline.
	// changed, err := imagesChanged(illust, set, idx)
	// if err != nil {
	// 	return fmt.Errorf("checking images for illust %d: %w", illust.ID, err)
	// }

	checkedAt := time.Now()
	// if changed {
	// 	log.Printf("pixiv: illust %d (%q) images changed - deferring, manual review required", illust.ID, illust.Title)
	// 	return imageset.MarkSourcePendingImageChange(set, idx, illust.CreateDate, checkedAt)
	// }

	newSrc := pixivSourceInfo(illust, set.Sources[idx], isPrivate)
	changed, err := imageset.ApplySourceMetadataUpdate(set, idx, newSrc, checkedAt, userId)
	if err != nil {
		return notification.FailedItem(userId, sourceID, set.ID, fmt.Errorf("%w: applying metadata update: %w", notification.ErrLibraryAccess, err))
	}
	if !changed {
		return notification.ItemResult{Kind: notification.ItemUnchanged, Ref: sourceID}
	}
	log.Printf("pixiv: updated illust %d (%q)", illust.ID, illust.Title)
	return notification.ItemResult{Kind: notification.ItemUpdated, Ref: sourceID, SetID: set.ID}
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
func downloadPixivImage(url, dir string) (_ string, err error) {
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
	defer func() {
		err = errors.Join(err, f.Close())
	}()

	if _, err = io.Copy(f, resp.Body); err != nil {
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
