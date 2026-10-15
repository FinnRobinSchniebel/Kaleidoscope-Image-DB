package imageset

import "sync"

// userSetLocks serializes every write to a user's image sets, from the read a
// write depends on through the write itself. Assumes one backend process per
// volume. Lock order: take it before tagging's per-user tag lock, never after.
var userSetLocks sync.Map // userID hex -> *sync.Mutex

// LockUserSets blocks until userID's set lock is free and returns its unlock,
// for use as defer LockUserSets(userID)(). Not reentrant: a function holding it
// must not call another that takes it.
func LockUserSets(userID string) (unlock func()) {
	v, _ := userSetLocks.LoadOrStore(userID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}
