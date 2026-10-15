package imageset

import (
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
