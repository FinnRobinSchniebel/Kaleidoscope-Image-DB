package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"slices"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// mergeScenario is how an incoming image list relates to one stored set.
type mergeScenario string

const (
	scenarioDuplicate mergeScenario = "duplicate" //the same images
	scenarioAddition  mergeScenario = "addition"  //every stored image, plus new ones
	scenarioLess      mergeScenario = "less"      //a subset of the stored images
	scenarioPartial   mergeScenario = "partial"   //some images in common, both sides have extras
	scenarioSimilar   mergeScenario = "similar"   //the same pictures, but no identical images
)

// candidateMatch is a stored set some incoming images matched, and how.
type candidateMatch struct {
	set      *ImageSetMongo
	scenario mergeScenario
	exactOf  [][]bson.ObjectID        //per incoming image, the set's pixel-identical images in set order
	pairs    []notification.ImagePair //uses the incoming images' IDs, so valid for a suggestion on the set they belong to
}

// findCandidates returns the sets with at least one non-plain hit among the
// first count incoming images, in order of their first hit.
func findCandidates(matches CollisionMap, count int) []bson.ObjectID {

	var candidates []bson.ObjectID
	seen := make(map[bson.ObjectID]bool)
	for i := range count {
		for _, hit := range matches[i] {

			if hit.Plain || seen[hit.SetID] {
				continue
			}

			seen[hit.SetID] = true
			candidates = append(candidates, hit.SetID)
		}
	}
	return candidates
}

// classifyCandidate decides set's scenario. A stored image only counts as hit
// by an exact match; an incoming image with only similar matches makes the
// result partial (with any exact match) or similar.
func classifyCandidate(set *ImageSetMongo, incoming []PreparedImage, matches CollisionMap) candidateMatch {

	match := candidateMatch{set: set, exactOf: make([][]bson.ObjectID, len(incoming))}
	storedHit := make(map[bson.ObjectID]bool)
	anyExact, anySimilarOnly, allIncomingExact := false, false, true

	for i, preparedImage := range incoming {

		similar := false
		for _, hit := range matches[i] {

			if hit.SetID != set.ID {
				continue
			}

			kind := notification.MatchSimilar
			if hit.Exact {
				kind = notification.MatchExact
				match.exactOf[i] = append(match.exactOf[i], hit.ImageID)
				storedHit[hit.ImageID] = true
			} else {
				similar = true
			}
			match.pairs = append(match.pairs, notification.ImagePair{ImageID: preparedImage.Info.ID, CandidateImageID: hit.ImageID, Match: kind})
		}

		switch {
		case len(match.exactOf[i]) > 0:
			anyExact = true
		case similar:
			anySimilarOnly, allIncomingExact = true, false
		default:
			allIncomingExact = false
		}
	}

	allStoredHit := true
	for _, img := range set.Image {
		if !storedHit[img.ID] {
			allStoredHit = false
			break
		}
	}

	switch {
	case anySimilarOnly && anyExact:
		match.scenario = scenarioPartial
	case anySimilarOnly:
		match.scenario = scenarioSimilar
	case allIncomingExact && allStoredHit:
		match.scenario = scenarioDuplicate
	case allStoredHit:
		match.scenario = scenarioAddition
	case allIncomingExact:
		match.scenario = scenarioLess
	default:
		match.scenario = scenarioPartial
	}
	return match
}

// splitCandidates separates classified candidates into automatic merges
// (duplicate, addition, less) and suggestions (partial, similar), keeping order.
func splitCandidates(candidates []candidateMatch) (automatic, suggested []candidateMatch) {

	for _, candidate := range candidates {
		switch candidate.scenario {
		case scenarioDuplicate, scenarioAddition, scenarioLess:
			automatic = append(automatic, candidate)
		default:
			suggested = append(suggested, candidate)
		}
	}
	return automatic, suggested
}

// planNewImages picks the incoming images match's set doesn't have. keeperOf[i]
// is -1 for an image the set has, otherwise the index of the first new image
// with the same pixels, so repeats are planned once. added holds each planned
// image with a fresh ID and no file name yet.
func planNewImages(incoming []PreparedImage, match candidateMatch) (keeperOf []int, added map[int]ImageInfo) {

	keeperOf = make([]int, len(incoming))
	added = make(map[int]ImageInfo)
	firstWithPixels := make(map[string]int)

	for i, preparedImage := range incoming {

		if len(match.exactOf[i]) > 0 {
			keeperOf[i] = -1
			continue
		}

		//a missing or malformed hash is never equal to another, so such an image keeps itself
		pixels := string(preparedImage.Info.Pixel)
		if keeper, seen := firstWithPixels[pixels]; seen && len(preparedImage.Info.Pixel) == pixelHashBytes {
			keeperOf[i] = keeper
			continue
		}

		firstWithPixels[pixels] = i
		keeperOf[i] = i
		info := preparedImage.Info
		info.ID = bson.NewObjectID()
		info.Name, info.LowResName = "", ""
		info.IsImageActive = true
		added[i] = info
	}
	return keeperOf, added
}

// placeInsertedImages returns existing with each planned image placed after the
// stored image matched by the nearest earlier incoming image; planned images
// before the first match go at the start.
func placeInsertedImages(existing []ImageInfo, match candidateMatch, keeperOf []int, added map[int]ImageInfo) []ImageInfo {

	var atStart []ImageInfo
	after := make(map[bson.ObjectID][]ImageInfo)
	var anchor bson.ObjectID
	anchored := false

	for i, keeper := range keeperOf {

		if keeper == -1 {
			anchor, anchored = match.exactOf[i][0], true
			continue
		}

		if keeper != i {
			continue //a repeat of an earlier new image
		}

		if anchored {
			after[anchor] = append(after[anchor], added[i])
		} else {
			atStart = append(atStart, added[i])
		}
	}

	placed := make([]ImageInfo, 0, len(existing)+len(added))
	placed = append(placed, atStart...)
	for _, img := range existing {
		placed = append(placed, img)
		placed = append(placed, after[img.ID]...)
	}
	return placed
}

// targetImageIDs returns the set images the incoming source covers, in its page
// order without repeats: every pixel-identical copy of a matched image, or the
// planned new image.
func targetImageIDs(match candidateMatch, keeperOf []int, added map[int]ImageInfo) []bson.ObjectID {

	var ids []bson.ObjectID
	seen := make(map[bson.ObjectID]bool)
	for i, keeper := range keeperOf {

		covered := match.exactOf[i]
		if keeper != -1 {
			covered = []bson.ObjectID{added[keeper].ID}
		}

		for _, id := range covered {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// mergeAttribution returns incomingOrder followed by the old IDs it doesn't
// list: attribution keeps images the source no longer has.
func mergeAttribution(old, incomingOrder []bson.ObjectID) []bson.ObjectID {

	merged := slices.Clone(incomingOrder)
	for _, id := range old {
		if !slices.Contains(merged, id) {
			merged = append(merged, id)
		}
	}
	return merged
}
