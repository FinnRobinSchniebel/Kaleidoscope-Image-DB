package notification

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestImportReportRecord(t *testing.T) {
	r := NewImportReport("pixiv")
	r.Record(
		ItemResult{Kind: ItemAdded, Ref: "1"},
		ItemResult{Kind: ItemUpdated, Ref: "2"},
		ItemResult{Kind: ItemPending, Ref: "3"},
		ItemResult{Kind: ItemSkipped, Ref: "4"},
		ItemResult{Kind: ItemFailed, Ref: "5"},
		ItemResult{Kind: ItemMissing, Ref: "6"},
		ItemResult{Kind: ItemUnchanged, Ref: "7"},
		ItemResult{Kind: ItemUnchanged, Ref: "8"},
	)
	lists := map[string][]ItemResult{
		"1": r.Added, "2": r.Updated, "3": r.Pending, "4": r.Skipped, "5": r.Failed, "6": r.Missing,
	}
	for ref, list := range lists {
		if len(list) != 1 || list[0].Ref != ref {
			t.Errorf("result %s landed in %v", ref, list)
		}
	}
	if r.Unchanged != 2 {
		t.Errorf("Unchanged = %d, want 2", r.Unchanged)
	}
}

func TestNewNotification(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	report := func(outcome Outcome, results ...ItemResult) *ImportReport {
		r := NewImportReport("pixiv")
		r.Record(results...)
		r.Finish(outcome, nil)
		return r
	}
	tests := []struct {
		name       string
		payload    Payload
		wantType   Type
		wantStatus Status
		wantRead   bool
	}{
		{"nothing to look at", report(OutcomeCompleted, ItemResult{Kind: ItemSkipped}, ItemResult{Kind: ItemUnchanged}), TypeImportReport, "", true},
		{"an added item", report(OutcomeCompleted, ItemResult{Kind: ItemAdded}), TypeImportReport, "", false},
		{"a failed item", report(OutcomeCompleted, ItemResult{Kind: ItemFailed}), TypeImportReport, "", false},
		{"a failed run", report(OutcomeFailed), TypeImportReport, "", false},
		{"a cancel with nothing changed", report(OutcomeCancelled), TypeImportReport, "", true},
		{"suggested merge", SuggestedMerge{}, TypeSuggestedMerge, StatusPending, false},
		{"pending image update", PendingImageUpdate{}, TypePendingImageUpdate, StatusPending, false},
	}
	for _, tt := range tests {
		n := newNotification("user1", tt.payload, now)
		if n.UserID != "user1" || n.Type != tt.wantType || n.Status != tt.wantStatus {
			t.Errorf("%s: user %q, type %q, status %q", tt.name, n.UserID, n.Type, n.Status)
		}
		if !n.CreatedAt.Equal(now) || !n.UpdatedAt.Equal(now) {
			t.Errorf("%s: CreatedAt %v, UpdatedAt %v, want both %v", tt.name, n.CreatedAt, n.UpdatedAt, now)
		}
		if read := !n.ReadAt.IsZero(); read != tt.wantRead {
			t.Errorf("%s: starts read = %t, want %t", tt.name, read, tt.wantRead)
		}
	}

	encoded, err := bson.Marshal(newNotification("user1", report(OutcomeFailed), now))
	if err != nil {
		t.Fatal(err)
	}
	raw := bson.Raw(encoded)
	if origin, err := raw.LookupErr("payload", "origin"); err != nil || origin.StringValue() != "pixiv" {
		t.Errorf("payload.origin = %v (%v), want pixiv", origin, err)
	}
	if _, err := raw.LookupErr("read_at"); err == nil {
		t.Error("an unread notification stored read_at")
	}
}
