package imageset

import (
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// FetchedPages is what a sync fetched of a source's pages at the service: one
// preview per page in the service's page order, since a missing page would read
// as a removal.
type FetchedPages struct {
	Previews        []PreparedImage
	Originals       map[int]PreparedImage //by page index, only the pages the check asked for
	FirstPageWidth  int                   //from the service's listing
	FirstPageHeight int
}

// sourcePageDistance is the largest PDQ distance at which a preview still
// matches its source's own page: master1200 previews measured 0 against their
// originals, and JPEG quality 50 up to 4.
const sourcePageDistance = 4

// NeedsImageCheck reports whether src's images must be checked against the
// service's because its Date moved. A held copy is never checked, and a missing
// service date can't show a move.
func NeedsImageCheck(src SourceInfo, serviceDate time.Time) bool {
	return src.ImageChangeState == ImageChangeNone && !serviceDate.IsZero() && !SourceDateMatches(src.Date, serviceDate)
}

// FindPagesNeedingOriginals returns, sorted, the pages whose originals the image
// check needs for any of sets whose copy of newSrc NeedsImageCheck. It works on
// the page lookup's projected sets (GetImageSetsBySourceIDs), before the lock.
func FindPagesNeedingOriginals(sets []*ImageSetMongo, newSrc SourceInfo, pages FetchedPages) ([]int, error) {

	var needed []int
	for _, set := range sets {

		index := slices.IndexFunc(set.Sources, func(src SourceInfo) bool { return SameSource(src, newSrc) })
		if index < 0 || !NeedsImageCheck(set.Sources[index], newSrc.Date) {
			continue
		}

		check, err := checkSourceImages(set, index, pages)
		if err != nil {
			return nil, err
		}
		needed = append(needed, check.needsOriginals...)
	}

	slices.Sort(needed)
	return slices.Compact(needed), nil
}

// sourceImageCheck is checkSourceImages' outcome for one copy.
type sourceImageCheck struct {
	kind           SourceWriteKind
	attribution    []bson.ObjectID //the copy's new AttributedTo, for the outcomes that write it
	needsOriginals []int           //pages to fetch before deciding; when set, nothing else is
}

// checkSourceImages compares the service's pages with the copy at
// set.Sources[index] and the rest of the set, and returns the copy's outcome, or
// only the pages whose originals it needs first. With nothing visible changed,
// an exact-dated copy is pending, since nothing explains its Date move, and a
// zip-dated one gets its baseline; a zip-dated copy whose first page's size
// differs is pending whatever the outcome. Pure: the caller decides when a copy
// is checked (NeedsImageCheck).
func checkSourceImages(set *ImageSetMongo, index int, pages FetchedPages) (sourceImageCheck, error) {

	src := set.Sources[index]
	imagesByID := make(map[bson.ObjectID]ImageInfo, len(set.Image))
	for _, image := range set.Image {
		imagesByID[image.ID] = image
	}

	stored, err := appendStoredImageHashes(nil, set.ID, set.Image)
	if err != nil {
		return sourceImageCheck{}, err
	}

	matched := matchPagesToSourceImages(src.AttributedTo, imagesByID, pages.Previews)
	similar, needsOriginals := matchPagesToSetImages(src.AttributedTo, stored, matched, pages)
	if len(needsOriginals) > 0 {
		return sourceImageCheck{needsOriginals: needsOriginals}, nil
	}

	kind := decideSourceImageChange(src.AttributedTo, matched, similar)
	zipDated := src.Date.IsZero() || isDayOnly(src.Date)
	if (zipDated && !doesFirstPageSizeMatch(imagesByID, matched, pages)) || (!zipDated && kind == SourceMetadataUpdate) {
		kind = SourcePendingFlip
	}

	if kind == SourcePendingFlip {
		return sourceImageCheck{kind: kind}, nil
	}
	return sourceImageCheck{kind: kind, attribution: mergeAttribution(src.AttributedTo, matched)}, nil
}

// matchPagesToSourceImages matches each preview to one of the source's own
// images (own, in its page order) within sourcePageDistance: first the one at
// the page's position, then, in page order, the closest one no page took yet
// (the earliest on a tie). An unmatched page is left zero.
func matchPagesToSourceImages(own []bson.ObjectID, imagesByID map[bson.ObjectID]ImageInfo, previews []PreparedImage) []bson.ObjectID {

	matched := make([]bson.ObjectID, len(previews))
	claimed := make(map[bson.ObjectID]bool)
	for i := range min(len(previews), len(own)) {

		if image, ok := imagesByID[own[i]]; ok && PDQDistance(previews[i].Info.PDQ, image.PDQ) <= sourcePageDistance {
			matched[i] = own[i]
			claimed[own[i]] = true
		}
	}

	for i, preview := range previews {

		if !matched[i].IsZero() {
			continue
		}

		best, bestDistance := bson.NilObjectID, sourcePageDistance+1
		for _, id := range own {

			image, ok := imagesByID[id]
			if !ok || claimed[id] {
				continue
			}

			if distance := PDQDistance(preview.Info.PDQ, image.PDQ); distance < bestDistance {
				best, bestDistance = id, distance
			}
		}

		if !best.IsZero() {
			matched[i] = best
			claimed[best] = true
		}
	}
	return matched
}

// matchPagesToSetImages fills in matched, in place, for the pages the own steps
// left unmatched. A page hits when findHashMatches finds an image of stored
// outside own; it's matched when its original is pixel-identical to an image no
// page took yet, and similar otherwise. Hit pages whose original wasn't fetched
// are returned in needsOriginals. A page with no hit stops it with nothing to
// fetch: nothing can be added yet, so the outcome is pending anyway.
func matchPagesToSetImages(own []bson.ObjectID, stored []StoredImageHash, matched []bson.ObjectID, pages FetchedPages) (similar bool, needsOriginals []int) {

	isOwn := make(map[bson.ObjectID]bool, len(own))
	for _, id := range own {
		isOwn[id] = true
	}

	var others []StoredImageHash
	for _, image := range stored {
		if !isOwn[image.ImageID] {
			others = append(others, image)
		}
	}

	claimed := make(map[bson.ObjectID]bool)
	for _, id := range matched {
		if !id.IsZero() {
			claimed[id] = true
		}
	}

	for i, preview := range pages.Previews {

		if !matched[i].IsZero() {
			continue
		}

		if len(findHashMatches(preview.Info.ImageHashes, preview.Plain, others)) == 0 {
			return false, nil
		}

		original, fetched := pages.Originals[i]
		if !fetched {
			needsOriginals = append(needsOriginals, i)
			continue
		}

		hits := findHashMatches(original.Info.ImageHashes, original.Plain, stored)
		exact := slices.IndexFunc(hits, func(hit HashMatch) bool { return hit.Exact && !claimed[hit.ImageID] })
		if exact < 0 {
			similar = true
			continue
		}

		matched[i] = hits[exact].ImageID
		claimed[matched[i]] = true
	}
	return similar, needsOriginals
}

// decideSourceImageChange returns the first outcome, in the order checked, that
// the service's pages matched against the copy's own images call for;
// SourceMetadataUpdate means nothing visible changed. Own images unmatched after
// every matched one are leftovers of earlier removals, not a removal.
func decideSourceImageChange(own, matched []bson.ObjectID, similar bool) SourceWriteKind {

	if similar || slices.Contains(matched, bson.NilObjectID) {
		return SourcePendingFlip
	}

	isOwn := make(map[bson.ObjectID]bool, len(own))
	for _, id := range own {
		isOwn[id] = true
	}

	isMatched := make(map[bson.ObjectID]bool, len(matched))
	for _, id := range matched {

		if !isOwn[id] {
			return SourceAttribution
		}
		isMatched[id] = true
	}

	unmatchedSeen := false
	for _, id := range own {

		if !isMatched[id] {
			unmatchedSeen = true
		} else if unmatchedSeen {
			return SourceRemoval
		}
	}

	for i, id := range matched {
		if i >= len(own) || own[i] != id {
			return SourceReorder
		}
	}
	return SourceMetadataUpdate
}

// doesFirstPageSizeMatch reports whether the image the first page matched has
// the first page's size from the service's listing.
func doesFirstPageSizeMatch(imagesByID map[bson.ObjectID]ImageInfo, matched []bson.ObjectID, pages FetchedPages) bool {

	if len(matched) == 0 {
		return false
	}

	image, ok := imagesByID[matched[0]]
	return ok && image.Width == pages.FirstPageWidth && image.Height == pages.FirstPageHeight
}
