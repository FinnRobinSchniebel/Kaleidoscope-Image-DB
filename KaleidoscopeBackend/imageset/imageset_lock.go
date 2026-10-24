package imageset

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// userSetLocks serializes every write to a user's image sets, from the read a
// write depends on through the write itself. Assumes one backend process per
// volume. Lock order: take it before tagging's per-user tag lock, never after.
var userSetLocks sync.Map // userID hex -> *sync.Mutex

// ErrLibraryBusy is returned when a scheduler task gives up waiting for a user's
// set lock. Its text is also the reason on the item the task skips.
var ErrLibraryBusy = errors.New("the library was busy")

// pauseKey is the context key for the scheduler's pause function.
type pauseKey struct{}

// WithPause returns ctx carrying pause, the scheduler's function for pausing the
// task ctx belongs to. pause returns nil once the task is resumed, or an error
// when it should stop trying.
func WithPause(ctx context.Context, pause func(context.Context) error) context.Context {
	return context.WithValue(ctx, pauseKey{}, pause)
}

// Pause pauses the scheduler task ctx belongs to until the scheduler resumes it,
// so other users are served meanwhile. Outside a scheduler task it's an error.
func Pause(ctx context.Context) error {

	pause, ok := ctx.Value(pauseKey{}).(func(context.Context) error)
	if !ok {
		return errors.New("pausing outside a scheduler task")
	}
	return pause(ctx)
}

// findUserSetLock returns userID's set mutex, creating it on first use.
func findUserSetLock(userID string) *sync.Mutex {
	v, _ := userSetLocks.LoadOrStore(userID, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// LockUserSets blocks until userID's set lock is free and returns its unlock,
// for use as defer LockUserSets(userID)(). Not reentrant: a function holding it
// must not call another that takes it.
func LockUserSets(userID string) (unlock func()) {
	mu := findUserSetLock(userID)
	mu.Lock()
	return mu.Unlock
}

// lockUserSetsOrPause takes userID's set lock like LockUserSets, except in a
// scheduler task (ctx carries a pause function): while the lock is busy, the
// task pauses and tries again when resumed. A task that gives up gets
// ErrLibraryBusy; a cancelled ctx gets its own error and never takes the lock.
func lockUserSetsOrPause(ctx context.Context, userID string) (unlock func(), err error) {

	mu := findUserSetLock(userID)
	pause, canPause := ctx.Value(pauseKey{}).(func(context.Context) error)
	if !canPause {
		mu.Lock()
		return mu.Unlock, nil
	}

	for {

		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if mu.TryLock() {
			return mu.Unlock, nil
		}

		if err := pause(ctx); err != nil {

			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			return nil, fmt.Errorf("%w: %w", ErrLibraryBusy, err)
		}
	}
}
