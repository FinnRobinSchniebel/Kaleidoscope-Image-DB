package tagging

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"

	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/services"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// RegistrationHookFunc implements services.RegistrationListener, keeping the
// Untracked system tag in sync with service connect/disconnect.
type RegistrationHookFunc struct{}

func (RegistrationHookFunc) OnServiceRegistrationChanged(userId, serviceName string) {
	if err := RecomputeUntrackedForService(userId, serviceName); err != nil {
		log.Printf("------ Warning: recomputing Untracked tag for user [%s] service [%s]: %s ------", userId, serviceName, err)
	}
}

// isLostMedia reports whether every one of sources has SourceMissing set.
// False for a set with no sources at all - there's nothing to have gone missing.
func isLostMedia(sources []imageset.SourceInfo) bool {
	if len(sources) == 0 {
		return false
	}
	for _, s := range sources {
		if !s.SourceMissing {
			return false
		}
	}
	return true
}

// isUntracked reports whether none of sources have a service currently
// registered for userID in the scheduler - i.e. no source is actively
// synced. True for a set with no sources at all.
func isUntracked(userID string, sources []imageset.SourceInfo) bool {
	if len(sources) == 0 {
		return true
	}
	for _, s := range sources {
		if services.DefaultScheduler.IsUserRegistered(s.Name, userID) {
			return false
		}
	}
	return true
}

// ensureSystemAutoTags returns the AutoTag ids for names (each expected to
// be one of the reserved system tag names), creating any that don't yet
// exist for userID. Races on first creation are resolved by re-fetching,
// relying on the existing {user_id, name} unique index (see
// findAutoTagsByNames/insertAutoTag/findAutoTagByName in auto_tags.go, the
// file that owns AutoTagsDB).
func ensureSystemAutoTags(userID bson.ObjectID, names []string) (map[string]bson.ObjectID, error) {
	existing, err := findAutoTagsByNames(userID, names)
	if err != nil {
		return nil, fmt.Errorf("resolving system auto tags: %w", err)
	}

	result := make(map[string]bson.ObjectID, len(names))
	for _, d := range existing {
		result[d.Name] = d.ID
	}

	for _, name := range names {
		if _, ok := result[name]; ok {
			continue
		}
		doc := AutoTagDoc{ID: bson.NewObjectID(), UserID: userID, Name: name, System: true}
		if err := insertAutoTag(doc); err != nil {
			if !errors.Is(err, ErrAutoTagNameExists) {
				return nil, fmt.Errorf("creating system auto tag %q: %w", name, err)
			}
			found, ferr := findAutoTagByName(userID, name)
			if ferr != nil {
				return nil, fmt.Errorf("resolving system auto tag %q after race: %w", name, ferr)
			}
			result[name] = found.ID
			continue
		}
		result[name] = doc.ID
	}
	return result, nil
}

// RecomputeSystemTags reconciles set's Lost Media/Untracked membership
// against its current Sources, mutating set.AutoTags/set.Tags in place and
// adjusting stored counts, then refreshes Untagged's Count. Like
// ProcessSourceTags, it does not persist set - callers already own the
// eventual UpdateImageSet/insert.
func RecomputeSystemTags(userID string, set *imageset.ImageSetMongo) error {
	uid, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return fmt.Errorf("parsing user id: %w", err)
	}

	want := map[string]bool{
		lostMediaTagName: isLostMedia(set.Sources),
		untrackedTagName: isUntracked(userID, set.Sources),
	}
	ids, err := ensureSystemAutoTags(uid, systemAutoTagNames)
	if err != nil {
		return err
	}

	changed := false
	for name, id := range ids {
		has := slices.Contains(set.AutoTags, id)
		if want[name] == has {
			continue
		}
		changed = true
		if want[name] {
			set.AutoTags = append(set.AutoTags, id)
		} else {
			set.AutoTags = slices.DeleteFunc(set.AutoTags, func(x bson.ObjectID) bool { return x == id })
		}
	}
	if changed {
		if err := rebuildTagsAndAdjustCounts(uid, set); err != nil {
			return err
		}
	}
	return refreshUntaggedCount(uid)
}

// RecomputeUntrackedForService re-evaluates the Untracked system tag on
// every image set owned by userID that has a source named serviceName,
// since that source's registration status just changed. Mirrors
// applyAutoTagToSets' query-diff-bulk-write shape.
func RecomputeUntrackedForService(userID, serviceName string) error {
	uid, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return fmt.Errorf("parsing user id: %w", err)
	}

	cursor, err := imageset.Collection.Find(context.Background(), bson.M{"kscope_userid": userID, "sources.name": serviceName})
	if err != nil {
		return fmt.Errorf("finding sets for service %q: %w", serviceName, err)
	}
	defer cursor.Close(context.Background())

	var sets []imageset.ImageSetMongo
	if err := cursor.All(context.Background(), &sets); err != nil {
		return fmt.Errorf("finding sets for service %q: %w", serviceName, err)
	}
	if len(sets) == 0 {
		return nil
	}

	ids, err := ensureSystemAutoTags(uid, []string{untrackedTagName})
	if err != nil {
		return err
	}
	untrackedID := ids[untrackedTagName]

	var models []mongo.WriteModel
	combinedDeltas := make(map[bson.ObjectID]int)
	for _, set := range sets {
		want := isUntracked(userID, set.Sources)
		has := slices.Contains(set.AutoTags, untrackedID)
		if want == has {
			continue
		}
		newAutoTags := slices.Clone(set.AutoTags)
		op := "$pull"
		if want {
			op = "$addToSet"
			newAutoTags = append(newAutoTags, untrackedID)
		} else {
			newAutoTags = slices.DeleteFunc(newAutoTags, func(id bson.ObjectID) bool { return id == untrackedID })
		}
		// TODO: tags is computed from this set's state as of the Find above, so a
		// concurrent write to this set's autotags/tag_rule_overrides between that
		// Find and this BulkWrite can get overwritten with a stale value here,
		// unlike the atomic $addToSet/$pull alongside it. Narrow window, self-heals
		// on the next relevant write; low priority, see read-modify-write-race-review
		// skill. count (also derived from this snapshot, via $inc) does not self-heal
		// the same way.
		tags := ApplyTagRuleOverrides(newAutoTags, set.TagRuleOverrides)
		addDeltas(combinedDeltas, tagCountDeltas(set.Tags, tags))
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": set.ID}).
			SetUpdate(bson.M{op: bson.M{"autotags": untrackedID}, "$set": bson.M{"tags": tags}}))
	}
	if len(models) == 0 {
		return nil
	}
	if _, err := imageset.Collection.BulkWrite(context.Background(), models); err != nil {
		return fmt.Errorf("updating untracked tag for service %q: %w", serviceName, err)
	}
	if err := adjustAutoTagCounts(uid, combinedDeltas); err != nil {
		return err
	}
	return refreshUntaggedCount(uid)
}

// ResolveTagTerm resolves term (a tag name or partial name from a search
// query) to the ids of every AutoTag whose Name contains it. If the
// reserved Untagged AutoTag matches by name, its id is excluded from ids
// and matchEmpty is set instead, since its id never appears literally in
// any set's Tags.
func ResolveTagTerm(userID string, term string) ([]string, bool, error) {
	uid, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return nil, false, fmt.Errorf("parsing user id: %w", err)
	}
	matched, err := SearchAutoTagIDsByName(uid, term)
	if err != nil {
		return nil, false, err
	}
	if len(matched) == 0 {
		return nil, false, nil
	}

	ids, err := ensureSystemAutoTags(uid, []string{untaggedTagName})
	if err != nil {
		return nil, false, err
	}
	untaggedID := ids[untaggedTagName]

	remaining := make([]string, 0, len(matched))
	matchEmpty := false
	for _, id := range matched {
		if id == untaggedID {
			matchEmpty = true
			continue
		}
		remaining = append(remaining, id.Hex())
	}
	return remaining, matchEmpty, nil
}
