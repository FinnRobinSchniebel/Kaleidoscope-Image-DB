package imageset

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"maps"
	"slices"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestCarriedNotifications(t *testing.T) {

	//made in order, so they sort as made
	entry, t1, t2 := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	kept := heldCopyChange{service: "pixiv", sourceID: "7", absorbedKept: true}
	lost := heldCopyChange{service: "pixiv", sourceID: "7", survivorLost: true}
	replaced := heldCopyChange{service: "pixiv", sourceID: "7", survivorLost: true, absorbedKept: true}
	heldOn := func(id bson.ObjectID) *ImageSetMongo {
		return &ImageSetMongo{ID: id, Sources: []SourceInfo{{Name: "pixiv", SourceID: "7", ImageChangeState: ImageChangePending}}}
	}
	wrote := func(target bson.ObjectID, changes ...heldCopyChange) mergeTargetOutcome {
		return mergeTargetOutcome{setID: target, heldCopies: changes}
	}

	tests := []struct {
		name   string
		record func(c *carriedNotifications)
		want   map[notification.PendingUpdateKey][]bson.ObjectID
	}{
		{"a target's held copy lost", func(c *carriedNotifications) {
			c.recordMergeWrite(entry, wrote(t1, lost))
		}, map[notification.PendingUpdateKey][]bson.ObjectID{kept.makeKey(t1): nil}},

		{"kept on two targets, then the entry removed", func(c *carriedNotifications) {
			c.recordMergeWrite(entry, wrote(t2, kept))
			c.recordMergeWrite(entry, wrote(t1, kept))
			c.recordSetRemoved(heldOn(entry))
		}, map[notification.PendingUpdateKey][]bson.ObjectID{kept.makeKey(entry): {t1, t2}}},

		{"a chain: kept on T1, which is kept on T2", func(c *carriedNotifications) {
			c.recordMergeWrite(entry, wrote(t1, kept))
			c.recordSetRemoved(heldOn(entry))
			c.recordMergeWrite(t1, wrote(t2, kept))
			c.recordSetRemoved(heldOn(t1))
		}, map[notification.PendingUpdateKey][]bson.ObjectID{kept.makeKey(entry): {t2}}},

		{"interrupted: kept on T1, the entry not removed", func(c *carriedNotifications) {
			c.recordMergeWrite(entry, wrote(t1, kept))
		}, map[notification.PendingUpdateKey][]bson.ObjectID{kept.makeKey(entry): {entry, t1}}},

		{"replaced: the target's own lost and the entry's kept on it", func(c *carriedNotifications) {
			c.recordMergeWrite(entry, wrote(t1, replaced))
			c.recordSetRemoved(heldOn(entry))
		}, map[notification.PendingUpdateKey][]bson.ObjectID{kept.makeKey(t1): nil, kept.makeKey(entry): {t1}}},

		{"the entry's not kept (a less), then removed", func(c *carriedNotifications) {
			c.recordMergeWrite(entry, wrote(t1))
			c.recordSetRemoved(heldOn(entry))
		}, map[notification.PendingUpdateKey][]bson.ObjectID{kept.makeKey(entry): nil}},

		{"nothing recorded", func(c *carriedNotifications) {}, map[notification.PendingUpdateKey][]bson.ObjectID{}},
	}

	for _, tc := range tests {

		var carried carriedNotifications
		tc.record(&carried)
		if got := carried.collectOriginLocations(); !maps.EqualFunc(got, tc.want, slices.Equal[[]bson.ObjectID]) {
			t.Errorf("%s: locations %v, want %v", tc.name, got, tc.want)
		}
	}
}
