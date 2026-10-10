package notification

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestReasonFor(t *testing.T) {
	sentinels := []error{
		ErrServiceSettings, ErrServiceSignIn, ErrServiceRequest, ErrDownloadFailed,
		ErrSaveFailed, ErrLibraryAccess, ErrSyncStopped,
	}
	for _, sentinel := range sentinels {
		wrapped := fmt.Errorf("outer: %w", fmt.Errorf("%w: inner: %w", sentinel, errors.New("raw detail")))
		if got := reasonFor(wrapped); got != sentinel.Error() {
			t.Errorf("reasonFor(%v) = %q, want %q", wrapped, got, sentinel.Error())
		}
	}
	if got := reasonFor(errors.New("mongo: no documents in result")); got != serverErrorReason {
		t.Errorf("a plain error gave %q, want %q", got, serverErrorReason)
	}
}

func TestFailedItemStoresOnlyTheReason(t *testing.T) {
	setID := bson.NewObjectID()
	raw := fmt.Errorf("%w: open /tmp/kaleidoscope_unzip_1/art/g/1.png: permission denied", ErrSaveFailed)

	item := FailedItem("user1", "art/g", setID, raw)
	if item.Kind != ItemFailed || item.Ref != "art/g" || item.SetID != setID || item.Reason != ErrSaveFailed.Error() {
		t.Errorf("FailedItem = %+v", item)
	}
	if strings.Contains(item.Reason, "/tmp") {
		t.Errorf("raw error text stored: %q", item.Reason)
	}

	r := NewImportReport("pixiv")
	r.Finish(OutcomeFailed, raw)
	if r.Error != ErrSaveFailed.Error() {
		t.Errorf("Finish stored Error %q, want only the reason", r.Error)
	}
	ok := NewImportReport("pixiv")
	ok.Finish(OutcomeCompleted, nil)
	if ok.Error != "" {
		t.Errorf("a successful run stored Error %q", ok.Error)
	}
}
