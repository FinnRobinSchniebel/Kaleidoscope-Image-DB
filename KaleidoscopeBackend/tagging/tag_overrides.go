package tagging

import (
	"errors"
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

// ErrInvalidOverride is returned for a malformed override entry or an id
// listed more than once. Callers map it to HTTP 400.
var ErrInvalidOverride = errors.New("invalid override entry")

// ErrUnknownOverrideTag is returned when an override names an id that isn't
// one of the set owner's AutoTags, or names the Untagged system tag. Callers
// map it to HTTP 400.
var ErrUnknownOverrideTag = errors.New("override is not a usable auto tag")

// validateOverrideEntries rejects any entry that doesn't parse, and any id
// that appears more than once, with either sign.
func validateOverrideEntries(entries []string) error {
	seen := make(map[bson.ObjectID]bool, len(entries))
	for _, entry := range entries {
		id, _, ok := ParseTagRuleOverrideEntry(entry)
		if !ok {
			return fmt.Errorf("%w: %s", ErrInvalidOverride, entry)
		}
		if seen[id] {
			return fmt.Errorf("%w: %s is listed more than once", ErrInvalidOverride, id.Hex())
		}
		seen[id] = true
	}
	return nil
}

// normalizeOverrides rewrites valid entries as "id" / "-id" with lowercase
// hex, so stored entries compare equal regardless of how they were sent.
func normalizeOverrides(entries []string) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		id, exclude, ok := ParseTagRuleOverrideEntry(entry)
		if !ok {
			continue
		}
		if exclude {
			out = append(out, "-"+id.Hex())
		} else {
			out = append(out, id.Hex())
		}
	}
	return out
}

// splitOverrides returns the included ids, the excluded ids, and every entry
// with its sign flipped ("a" -> "-a", "-b" -> "b"). Expects normalized entries.
func splitOverrides(overrides []string) (includes, excludes, reversed []string) {
	for _, entry := range overrides {
		id, exclude, ok := ParseTagRuleOverrideEntry(entry)
		if !ok {
			continue
		}
		if exclude {
			excludes = append(excludes, id.Hex())
			reversed = append(reversed, id.Hex())
		} else {
			includes = append(includes, id.Hex())
			reversed = append(reversed, "-"+id.Hex())
		}
	}
	return includes, excludes, reversed
}

func ownerOf(state imageset.SetTagState) (bson.ObjectID, error) {
	owner, err := bson.ObjectIDFromHex(state.KscopeUserId)
	if err != nil {
		return bson.ObjectID{}, fmt.Errorf("image set %s has no valid owner: %w", state.ID.Hex(), err)
	}
	return owner, nil
}

// validateOverrideTags checks that every override id is one of each set
// owner's AutoTags, and that none is the Untagged system tag, whose id must
// never appear in a set's Tags.
func validateOverrideTags(states []imageset.SetTagState, overrides []string) error {
	ids := make([]bson.ObjectID, 0, len(overrides))
	for _, entry := range overrides {
		if id, _, ok := ParseTagRuleOverrideEntry(entry); ok {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}

	checked := make(map[bson.ObjectID]bool)
	for _, state := range states {
		owner, err := ownerOf(state)
		if err != nil {
			return err
		}
		if checked[owner] {
			continue
		}
		checked[owner] = true

		summaries, err := AutoTagSummariesByID(owner, ids)
		if err != nil {
			return err
		}
		known := make(map[bson.ObjectID]AutoTagSummary, len(summaries))
		for _, s := range summaries {
			known[s.ID] = s
		}
		for _, id := range ids {
			s, ok := known[id]
			if !ok {
				return fmt.Errorf("%w: %s", ErrUnknownOverrideTag, id.Hex())
			}
			if s.System && s.Name == untaggedTagName {
				return fmt.Errorf("%w: %s is the %s tag", ErrUnknownOverrideTag, id.Hex(), untaggedTagName)
			}
		}
	}
	return nil
}

// patchCountDeltas returns, per set owner, the AutoTag count change a PATCH
// of includes/excludes causes: +1 per include not already in a set's Tags,
// -1 per exclude that was.
func patchCountDeltas(states []imageset.SetTagState, includes, excludes []string) (map[bson.ObjectID]map[bson.ObjectID]int, error) {
	deltas := make(map[bson.ObjectID]map[bson.ObjectID]int)
	add := func(owner bson.ObjectID, idHex string, d int) {
		id, err := bson.ObjectIDFromHex(idHex)
		if err != nil {
			return
		}
		if deltas[owner] == nil {
			deltas[owner] = make(map[bson.ObjectID]int)
		}
		deltas[owner][id] += d
	}
	for _, state := range states {
		owner, err := ownerOf(state)
		if err != nil {
			return nil, err
		}
		has := toSet(state.Tags)
		for _, id := range includes {
			if !has[id] {
				add(owner, id, 1)
			}
		}
		for _, id := range excludes {
			if has[id] {
				add(owner, id, -1)
			}
		}
	}
	return deltas, nil
}

// applyOwnerCounts applies each owner's count deltas, then refreshes that
// owner's Untagged count.
func applyOwnerCounts(deltas map[bson.ObjectID]map[bson.ObjectID]int) error {
	for owner, d := range deltas {
		if err := applyCountDeltas(owner, d); err != nil {
			return err
		}
	}
	return nil
}

func hexIDs(ids []bson.ObjectID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.Hex()
	}
	return out
}

// ReplaceTagOverrides sets every set in ids (userID's own unless admin) to
// exactly overrides and rebuilds its Tags from its AutoTags. Empty overrides
// clear them. ids must be unique and overrides pass validateOverrideEntries.
func ReplaceTagOverrides(userID string, admin bool, ids []bson.ObjectID, overrides []string) ([]string, error) {
	overrides = normalizeOverrides(overrides)
	states, err := imageset.TagStatesForSets(ids, userID, admin)
	if err != nil {
		return nil, err
	}
	if err := validateOverrideTags(states, overrides); err != nil {
		return nil, err
	}

	tagsByID := make(map[bson.ObjectID][]string, len(states))
	deltas := make(map[bson.ObjectID]map[bson.ObjectID]int)
	for _, state := range states {
		owner, err := ownerOf(state)
		if err != nil {
			return nil, err
		}
		newTags := ApplyTagRuleOverrides(state.AutoTags, overrides)
		tagsByID[state.ID] = newTags
		if deltas[owner] == nil {
			deltas[owner] = make(map[bson.ObjectID]int)
		}
		addDeltas(deltas[owner], tagCountDeltas(state.Tags, newTags))
	}

	if err := imageset.SaveTagOverrides(overrides, tagsByID); err != nil {
		return nil, err
	}
	return hexIDs(ids), applyOwnerCounts(deltas)
}

// AddTagOverrides adds each override to every set in ids (userID's own
// unless admin), replacing that set's entry of the opposite sign for the same
// id. It never removes an override. Because each incoming override is final
// for its id, Tags becomes Tags plus the includes minus the excludes, with no
// need for AutoTags. ids must be unique and overrides pass
// validateOverrideEntries.
func AddTagOverrides(userID string, admin bool, ids []bson.ObjectID, overrides []string) ([]string, error) {
	overrides = normalizeOverrides(overrides)
	states, err := imageset.TagStatesForSets(ids, userID, admin)
	if err != nil {
		return nil, err
	}
	if err := validateOverrideTags(states, overrides); err != nil {
		return nil, err
	}

	includes, excludes, reversed := splitOverrides(overrides)
	deltas, err := patchCountDeltas(states, includes, excludes)
	if err != nil {
		return nil, err
	}

	if err := imageset.MergeTagOverrides(ids, userID, admin, imageset.OverrideMerge{
		Drop:     reversed,
		Add:      overrides,
		TagsAdd:  includes,
		TagsDrop: excludes,
	}); err != nil {
		return nil, err
	}
	return hexIDs(ids), applyOwnerCounts(deltas)
}
