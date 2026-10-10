package notification

import (
	"errors"
	"log"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// User-facing failure reasons. A producer wraps its raw error with the one that
// says what failed; only the reason's text is stored, the raw error is logged.
var (
	ErrServiceSettings = errors.New("the service account settings are missing or invalid")
	ErrServiceSignIn   = errors.New("couldn't sign in to the service")
	ErrServiceRequest  = errors.New("couldn't load your bookmarks from the service")
	ErrDownloadFailed  = errors.New("couldn't download the images")
	ErrSaveFailed      = errors.New("couldn't save the image set")
	ErrLibraryAccess   = errors.New("couldn't read or update your library")
	ErrSyncStopped     = errors.New("the sync was stopped")
)

const serverErrorReason = "server error"

// userReasons is checked in order, so a joined error reports its original cause
// before ErrSyncStopped. A new reason must be added here too.
var userReasons = []error{
	ErrServiceSettings,
	ErrServiceSignIn,
	ErrServiceRequest,
	ErrDownloadFailed,
	ErrSaveFailed,
	ErrLibraryAccess,
	ErrSyncStopped,
}

// reasonFor returns the text of the first user reason in err's chain, or
// serverErrorReason when it carries none.
func reasonFor(err error) string {
	for _, reason := range userReasons {
		if errors.Is(err, reason) {
			return reason.Error()
		}
	}
	return serverErrorReason
}

// FailedItem logs err in full and returns a failed item that stores only its
// user-facing reason.
func FailedItem(userID, ref string, setID bson.ObjectID, err error) ItemResult {
	log.Printf("import [%s]: %s failed: %v", userID, ref, err)
	return ItemResult{Kind: ItemFailed, Ref: ref, SetID: setID, Reason: reasonFor(err)}
}
