package services

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
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
	return DefaultScheduler.Enqueue(pixivServiceName, userId, func(ctx context.Context) ([]notification.ItemResult, error) {
		return processBookmarkPage(ctx, userId, pixivUID, restrict, maxBookmarkID, done), nil
	})
}

// processBookmarkPage fetches one page of bookmarks, processes its items, then
// enqueues the next page task. Public pages are followed by private pages.
// Returns the page's item results; new items report from their own save task.
// A failure is handed to done rather than returned, so it's logged only once.
func processBookmarkPage(ctx context.Context, userId string, pixivUID uint64, restrict pixiv.Restrict, maxBookmarkID int, done func(error)) []notification.ItemResult {
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
		results = processBookmarkItems(ctx, userId, illusts, restrict == pixiv.Private)
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
// every write re-reads its sets first. Items are handled oldest bookmark first,
// so new ones are queued in bookmark order. Returns the page's items, except new
// items that were queued: their save task reports them. Works with nothing to
// write, works held in every set (Skipped per set without reaching the writer)
// and stored works hidden by the account's filter get their LastChecked bumped
// once, after the loop.
func processBookmarkItems(ctx context.Context, userId string, illusts []pixivmodel.Illust, isPrivate bool) []notification.ItemResult {
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

	checkedAt := time.Now()
	var checked []string
	results := make([]notification.ItemResult, 0, len(illusts))
	for _, il := range slices.Backward(illusts) {
		idStr := strconv.FormatUint(il.ID, 10)
		sets := existing[idStr]
		exists := len(sets) > 0

		// Must stay first: placeholder entries look changed to every later check.
		if !il.Visible {
			switch {
			case !illustRemoved(il):
				results = append(results, notification.ItemResult{Kind: notification.ItemSkipped, Ref: idStr, Reason: "hidden by the account's filter"})
				if exists {
					checked = append(checked, idStr) //still bookmarked
				}
			case exists:
				results = append(results, markPixivSourceMissing(ctx, userId, il.ID)...)
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

		if isPixivSourceHeldInEverySet(sets, idStr) {
			for _, set := range sets {
				src, _ := sourceByID(set, idStr)
				results = append(results, reportHeldPixivCopy(idStr, set.ID, src.ImageChangeState))
			}
			checked = append(checked, idStr)
			continue
		}

		if !isPixivSourceStaleInAnySet(il, sets, isPrivate) {
			results = append(results, notification.ItemResult{Kind: notification.ItemUnchanged, Ref: idStr})
			checked = append(checked, idStr)
			continue
		}
		results = append(results, applyPixivSourceUpdate(ctx, userId, &il, isPrivate)...)
	}

	if err := imageset.MarkSourcesChecked(ctx, userId, pixivServiceName, checked, checkedAt); err != nil {
		log.Printf("------ Warning: pixiv sync [%s]: stamping the page's checked works: %s ------", userId, err)
	}
	return results
}

// finishPixivSync queues done(err) behind the user's pending save tasks, or
// calls it directly, with the queueing error added, if queueing fails.
func finishPixivSync(userId string, done func(error), err error) {
	if queueErr := DefaultScheduler.Enqueue(pixivServiceName, userId, func(context.Context) ([]notification.ItemResult, error) {
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

// isPixivSourceStaleInAnySet reports whether pixivSourceStale holds for any of
// the sets' copies of il's source.
func isPixivSourceStaleInAnySet(il pixivmodel.Illust, sets []*imageset.ImageSetMongo, isPrivate bool) bool {

	sourceID := strconv.FormatUint(il.ID, 10)
	for _, set := range sets {

		if src, idx := sourceByID(set, sourceID); idx >= 0 && pixivSourceStale(il, src, isPrivate) {
			return true
		}
	}
	return false
}

// isPixivSourceHeldInEverySet reports whether every set's copy of sourceID is
// held by its image-change state; false for no sets.
func isPixivSourceHeldInEverySet(sets []*imageset.ImageSetMongo, sourceID string) bool {

	if len(sets) == 0 {
		return false
	}

	for _, set := range sets {
		if src, idx := sourceByID(set, sourceID); idx < 0 || src.ImageChangeState == imageset.ImageChangeNone {
			return false
		}
	}
	return true
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
// Its task reports what importing the illust did.
func enqueueNewIllust(userId string, illust pixivmodel.Illust, isPrivate bool) error {
	return DefaultScheduler.Enqueue(pixivServiceName, userId, func(ctx context.Context) ([]notification.ItemResult, error) {
		return savePixivIllust(ctx, userId, &illust, isPrivate), nil
	})
}

// markPixivSourceMissing marks the illust's source missing in every set holding
// it. Only a set whose copy wasn't missing yet is reported as missing.
func markPixivSourceMissing(ctx context.Context, userId string, illustID uint64) []notification.ItemResult {
	sourceID := strconv.FormatUint(illustID, 10)
	results, err := imageset.MarkSourceMissing(ctx, userId, pixivServiceName, sourceID, time.Now())
	switch {
	case errors.Is(err, imageset.ErrSourceNotStored):
		return []notification.ItemResult{{Kind: notification.ItemSkipped, Ref: sourceID, Reason: "removed at source"}}
	case errors.Is(err, imageset.ErrLibraryBusy):
		return []notification.ItemResult{{Kind: notification.ItemSkipped, Ref: sourceID, Reason: imageset.ErrLibraryBusy.Error()}}
	case errors.Is(err, context.Canceled):
		return []notification.ItemResult{notification.FailedItem(userId, sourceID, bson.NilObjectID, fmt.Errorf("%w: %w", notification.ErrSyncStopped, err))}
	case err != nil:
		return []notification.ItemResult{notification.FailedItem(userId, sourceID, bson.NilObjectID, fmt.Errorf("%w: marking missing: %w", notification.ErrLibraryAccess, err))}
	}
	return reportPixivSourceWrites(userId, sourceID, results, notification.ItemMissing, "marking missing")
}

// reportPixivSourceWrites turns a by-source writer's per-set results into report
// items: a failed set is Failed, a held one Skipped and a changed one
// changedKind, each with its set ID; action names the write in a failure. If no
// set gave an item, the work is one Unchanged item.
func reportPixivSourceWrites(userId, sourceID string, results []imageset.SourceWriteResult, changedKind notification.ItemKind, action string) []notification.ItemResult {

	var items []notification.ItemResult
	for _, result := range results {

		switch {
		case result.Err != nil:
			err := fmt.Errorf("%w: %s: %w", notification.ErrLibraryAccess, action, result.Err)
			items = append(items, notification.FailedItem(userId, sourceID, result.SetID, err))
		case result.Held != imageset.ImageChangeNone:
			items = append(items, reportHeldPixivCopy(sourceID, result.SetID, result.Held))
		case result.Changed:
			items = append(items, notification.ItemResult{Kind: changedKind, Ref: sourceID, SetID: result.SetID})
		}
	}

	if len(items) == 0 {
		return []notification.ItemResult{{Kind: notification.ItemUnchanged, Ref: sourceID}}
	}
	return items
}

// reportHeldPixivCopy is the Skipped item for setID, whose copy of sourceID is
// held by its image-change state.
func reportHeldPixivCopy(sourceID string, setID bson.ObjectID, state imageset.ImageChangeState) notification.ItemResult {

	reason := "an image update is waiting for review"
	if state == imageset.ImageChangeDeclined {
		reason = "image updates were declined"
	}
	return notification.ItemResult{Kind: notification.ItemSkipped, Ref: sourceID, SetID: setID, Reason: reason}
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

// savePixivIllust downloads illust's pages and imports them, returning the
// report items for it: a new set, a merge into stored sets, or a failure.
func savePixivIllust(ctx context.Context, userId string, illust *pixivmodel.Illust, isPrivate bool) []notification.ItemResult {

	illustID := illust.ID
	ref := strconv.FormatUint(illustID, 10)

	// Download all pages to a temporary directory, then import them.
	tmpDir, err := os.MkdirTemp("", fmt.Sprintf("pixiv_%d_*", illustID))
	if err != nil {
		return []notification.ItemResult{notification.FailedItem(userId, ref, bson.NilObjectID, fmt.Errorf("%w: create temp dir: %w", notification.ErrSaveFailed, err))}
	}
	defer os.RemoveAll(tmpDir)

	urls := illustImageURLs(illust)
	if len(urls) == 0 {
		err := fmt.Errorf("%w: illust %d: no downloadable image URLs", notification.ErrDownloadFailed, illustID)
		return []notification.ItemResult{notification.FailedItem(userId, ref, bson.NilObjectID, err)}
	}

	files := make([]imageset.MediaSource, 0, len(urls))
	for _, url := range urls {

		path, err := downloadPixivImage(ctx, url, tmpDir)
		if err != nil && ctx.Err() != nil {
			err = fmt.Errorf("%w: download %s: %w", notification.ErrSyncStopped, url, err)
			return []notification.ItemResult{notification.FailedItem(userId, ref, bson.NilObjectID, err)}
		}

		if err != nil {
			err = fmt.Errorf("%w: download %s: %w", notification.ErrDownloadFailed, url, err)
			return []notification.ItemResult{notification.FailedItem(userId, ref, bson.NilObjectID, err)}
		}
		files = append(files, imageset.DiskSource{Path: path})
	}

	results := imageset.ProcessImageSet(ctx, buildPixivImageSet(illust, userId, isPrivate), files, userId, ref)
	log.Printf("pixiv: imported illust %d (%q)", illustID, illust.Title)
	return results
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

// illustCheckImageURLs returns a smaller preview URL for every page, for the
// image-change check: PDQ resizes its input anyway, so a preview saves
// network, disk and memory. Falls back to illustImageURLs per page where no
// smaller size exists.
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
// Images, their attribution and the path are left empty; storing the set
// fills those in.
func buildPixivImageSet(illust *pixivmodel.Illust, userId string, isPrivate bool) *imageset.ImageSetMongo {

	authorName, authorID := readPixivAuthor(illust)
	src := imageset.SourceInfo{
		Name:         pixivServiceName,
		SourceID:     strconv.FormatUint(illust.ID, 10),
		Title:        illust.Title,
		Description:  pixivIllustCaption(illust),
		SourceAuthor: authorName,
		AuthorID:     authorID,
		Tags:         pixivIllustTags(illust, isPrivate),
		Date:         illust.CreateDate,
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

// pixivSourceInfo builds the incoming SourceInfo for illust's metadata update.
func pixivSourceInfo(illust *pixivmodel.Illust, isPrivate bool) imageset.SourceInfo {
	authorName, authorID := readPixivAuthor(illust)
	return imageset.SourceInfo{
		Name:         pixivServiceName,
		SourceID:     strconv.FormatUint(illust.ID, 10),
		Title:        illust.Title,
		Description:  pixivIllustCaption(illust),
		SourceAuthor: authorName,
		AuthorID:     authorID,
		Tags:         pixivIllustTags(illust, isPrivate),
		Date:         illust.CreateDate,
	}
}

// readPixivAuthor returns illust's author name and ID, both empty (an unknown
// author) when the listing has no user.
func readPixivAuthor(illust *pixivmodel.Illust) (name, id string) {

	if illust.User == nil {
		return "", ""
	}
	return illust.User.Name, strconv.FormatUint(illust.User.ID, 10)
}

// applyPixivSourceUpdate applies illust's metadata to every set holding its
// source. A re-apply of identical values reports unchanged, and a source no set
// holds any more since the page was read is skipped.
func applyPixivSourceUpdate(ctx context.Context, userId string, illust *pixivmodel.Illust, isPrivate bool) []notification.ItemResult {
	sourceID := strconv.FormatUint(illust.ID, 10)

	// Image changes aren't checked yet: a changed CreateDate only updates the metadata.
	results, err := imageset.ApplySourceMetadataUpdate(ctx, userId, pixivSourceInfo(illust, isPrivate), time.Now())
	switch {
	case errors.Is(err, imageset.ErrSourceNotStored):
		return []notification.ItemResult{{Kind: notification.ItemSkipped, Ref: sourceID, Reason: "the set changed during the sync"}}
	case errors.Is(err, imageset.ErrLibraryBusy):
		return []notification.ItemResult{{Kind: notification.ItemSkipped, Ref: sourceID, Reason: imageset.ErrLibraryBusy.Error()}}
	case errors.Is(err, context.Canceled):
		return []notification.ItemResult{notification.FailedItem(userId, sourceID, bson.NilObjectID, fmt.Errorf("%w: %w", notification.ErrSyncStopped, err))}
	case err != nil:
		return []notification.ItemResult{notification.FailedItem(userId, sourceID, bson.NilObjectID, fmt.Errorf("%w: applying metadata update: %w", notification.ErrLibraryAccess, err))}
	}

	items := reportPixivSourceWrites(userId, sourceID, results, notification.ItemUpdated, "applying metadata update")
	if slices.ContainsFunc(items, func(item notification.ItemResult) bool { return item.Kind == notification.ItemUpdated }) {
		log.Printf("pixiv: updated illust %d (%q)", illust.ID, illust.Title)
	}
	return items
}

// errDownloadStalled is the cause a download's watchdog cancels it with.
var errDownloadStalled = errors.New("the download stalled")

// stallReader passes reads through to r and pushes watchdog back by timeout
// after each read that brought data.
type stallReader struct {
	r        io.Reader
	watchdog *time.Timer
	timeout  time.Duration
}

func (s stallReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if n > 0 {
		s.watchdog.Reset(s.timeout)
	}
	return n, err
}

// downloadPixivImage fetches a single Pixiv image URL into dir and returns the
// local file path. Pixiv image servers require Referer: https://www.pixiv.net/
// which differs from the App API host used by the library's own downloader. It
// fails when no response starts within pixivResponseTimeout, no data arrives for
// pixivStallTimeout, or ctx is cancelled.
func downloadPixivImage(ctx context.Context, url, dir string) (_ string, err error) {

	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	watchdog := time.AfterFunc(pixivResponseTimeout, func() { cancel(errDownloadStalled) })
	defer watchdog.Stop()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Referer", "https://www.pixiv.net/")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {

		if errors.Is(context.Cause(ctx), errDownloadStalled) {
			return "", fmt.Errorf("no response within %s: %w", pixivResponseTimeout, errDownloadStalled)
		}
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

	watchdog.Reset(pixivStallTimeout)
	if _, err = io.Copy(f, stallReader{r: resp.Body, watchdog: watchdog, timeout: pixivStallTimeout}); err != nil {

		if errors.Is(context.Cause(ctx), errDownloadStalled) {
			return "", fmt.Errorf("no data for %s: %w", pixivStallTimeout, errDownloadStalled)
		}
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
