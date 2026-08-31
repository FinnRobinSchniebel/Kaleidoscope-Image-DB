package tagging

import (
	"fmt"
	"sort"
	"strings"

	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ParseTagRuleOverrideEntry parses an optional "-" prefix plus an AutoTag
// id hex string. ok is false if the remainder isn't a valid id.
func ParseTagRuleOverrideEntry(entry string) (id bson.ObjectID, exclude bool, ok bool) {
	exclude = strings.HasPrefix(entry, "-")
	idStr := strings.TrimPrefix(entry, "-")
	id, err := bson.ObjectIDFromHex(idStr)
	if err != nil {
		return bson.ObjectID{}, false, false
	}
	return id, exclude, true
}

// ApplyTagRuleOverrides returns the effective tag-id set (hex strings,
// sorted) after applying overrides on autoTags: "id" includes it even if
// not matched, "-id" excludes it even if matched; later entries win over
// earlier ones for the same id. Invalid entries are skipped.
func ApplyTagRuleOverrides(autoTags []bson.ObjectID, overrides []string) []string {
	include := make(map[string]bool, len(autoTags)+len(overrides))
	for _, id := range autoTags {
		include[id.Hex()] = true
	}
	for _, entry := range overrides {
		id, exclude, ok := ParseTagRuleOverrideEntry(entry)
		if !ok {
			continue
		}
		include[id.Hex()] = !exclude
	}

	result := make([]string, 0, len(include))
	for id, keep := range include {
		if keep {
			result = append(result, id)
		}
	}
	sort.Strings(result)
	return result
}

func buildTags(set *imageset.ImageSetMongo) {
	set.Tags = ApplyTagRuleOverrides(set.AutoTags, set.TagRuleOverrides)
}

// rebuildTagsAndAdjustCounts recomputes set.Tags (see buildTags) and
// applies the resulting per-tag count delta. Does not persist set.
func rebuildTagsAndAdjustCounts(userID bson.ObjectID, set *imageset.ImageSetMongo) error {
	old := set.Tags
	buildTags(set)
	return adjustAutoTagCounts(userID, tagCountDeltas(old, set.Tags))
}

// SetTagOverrides replaces TagRuleOverrides on every set in ids (owner or
// admin only, see GetFromID) and rebuilds Tags. Stops at the first error.
func SetTagOverrides(userId string, ids []string, overrides []string) ([]string, error) {
	uid, err := bson.ObjectIDFromHex(userId)
	if err != nil {
		return nil, fmt.Errorf("parsing user id: %w", err)
	}
	sets, err := imageset.GetFromID(userId, ids...)
	if err != nil {
		return nil, err
	}
	updated := make([]string, 0, len(sets))
	for i := range sets {
		sets[i].TagRuleOverrides = overrides
		if err := rebuildTagsAndAdjustCounts(uid, &sets[i]); err != nil {
			return updated, err
		}
		if err := imageset.UpdateImageSet(&sets[i]); err != nil {
			return updated, err
		}
		updated = append(updated, sets[i].ID.Hex())
	}
	if len(updated) == 0 {
		return updated, nil
	}
	return updated, refreshUntaggedCount(uid)
}
