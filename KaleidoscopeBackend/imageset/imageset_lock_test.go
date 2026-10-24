package imageset

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLockUserSetsPerUser(t *testing.T) {
	unlockFirst := LockUserSets("lock-test-user1")

	otherUser := make(chan struct{})
	go func() {
		LockUserSets("lock-test-user2")()
		close(otherUser)
	}()
	select {
	case <-otherUser:
	case <-time.After(time.Second):
		t.Fatal("another user's lock waited for user1's")
	}

	sameUser := make(chan struct{})
	go func() {
		LockUserSets("lock-test-user1")()
		close(sameUser)
	}()
	select {
	case <-sameUser:
		t.Fatal("a second user1 lock was granted while the first was held")
	case <-time.After(50 * time.Millisecond):
	}

	unlockFirst()
	select {
	case <-sameUser:
	case <-time.After(time.Second):
		t.Fatal("user1's second lock was never granted after the unlock")
	}
}

func TestLockUserSetsOrPause(t *testing.T) {

	//no pause function (zip, HTTP): it waits for the lock as before
	unlockHeld := LockUserSets("pause-test-wait")
	acquired := make(chan struct{})
	go func() {
		unlock, err := lockUserSetsOrPause(context.Background(), "pause-test-wait")
		if err != nil {
			t.Errorf("waiting lock: %v", err)
			return
		}
		unlock()
		close(acquired)
	}()

	select {
	case <-acquired:
		t.Fatal("got the lock while it was held")
	case <-time.After(50 * time.Millisecond):
	}
	unlockHeld()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("never got the lock after it was released")
	}

	//a scheduler task pauses while the lock is busy and takes it once it's free
	unlockHeld = LockUserSets("pause-test-pause")
	pauses := 0
	ctx := WithPause(context.Background(), func(context.Context) error {
		pauses++
		if pauses == 2 {
			unlockHeld()
		}
		return nil
	})
	unlock, err := lockUserSetsOrPause(ctx, "pause-test-pause")
	if err != nil || pauses != 2 {
		t.Fatalf("pausing lock: err %v after %d pauses, want the lock after 2", err, pauses)
	}
	unlock()

	//a task that gives up gets ErrLibraryBusy
	defer LockUserSets("pause-test-busy")()
	giveUp := WithPause(context.Background(), func(context.Context) error { return errors.New("gave up") })
	if _, err := lockUserSetsOrPause(giveUp, "pause-test-busy"); !errors.Is(err, ErrLibraryBusy) {
		t.Errorf("giving up: err %v, want ErrLibraryBusy", err)
	}

	//a pause ended by a cancellation (the user was removed) isn't reported busy either
	removed := WithPause(context.Background(), func(context.Context) error { return context.Canceled })
	if _, err := lockUserSetsOrPause(removed, "pause-test-busy"); !errors.Is(err, context.Canceled) || errors.Is(err, ErrLibraryBusy) {
		t.Errorf("pause cancelled: err %v, want the cancellation and not busy", err)
	}

	//a cancelled task never takes the lock, even a free one, and isn't reported busy
	cancelled, cancel := context.WithCancel(WithPause(context.Background(), func(context.Context) error {
		t.Error("a cancelled task paused")
		return nil
	}))
	cancel()
	if _, err := lockUserSetsOrPause(cancelled, "pause-test-free"); !errors.Is(err, context.Canceled) || errors.Is(err, ErrLibraryBusy) {
		t.Errorf("cancelled: err %v, want the cancellation and not busy", err)
	}
	freeLock := findUserSetLock("pause-test-free")
	if !freeLock.TryLock() {
		t.Error("a cancelled task took the lock")
	} else {
		freeLock.Unlock()
	}
}
