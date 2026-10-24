package services

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"context"
	"errors"
	"os"
	"slices"
	"sync"
	"testing"
	"time"
)

// published records the reports scheduler tests publish, per user.
var published = struct {
	mu     sync.Mutex
	byUser map[string]int
}{byUser: make(map[string]int)}

// TestMain swaps publishReport for a recorder before any test starts a
// scheduler, so no test writes to the database.
func TestMain(m *testing.M) {
	publishReport = func(userID string, _ notification.Payload) error {
		published.mu.Lock()
		defer published.mu.Unlock()
		published.byUser[userID]++
		return nil
	}
	os.Exit(m.Run())
}

func countPublished(userID string) int {
	published.mu.Lock()
	defer published.mu.Unlock()
	return published.byUser[userID]
}

// eventLog records what test tasks did, in order.
type eventLog struct {
	mu     sync.Mutex
	events []string
}

func (l *eventLog) add(event string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, event)
}

func (l *eventLog) list() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.events)
}

// newTestScheduler returns an unstarted scheduler with service "svc" and the
// given users, whose published counts start at 0, and a stop that is safe to call
// more than once; it also runs when the test ends.
func newTestScheduler(t *testing.T, cfg ServiceConfig, users ...string) (*Scheduler, func()) {
	t.Helper()

	published.mu.Lock()
	for _, user := range users {
		delete(published.byUser, user)
	}
	published.mu.Unlock()

	s := NewScheduler()
	s.RegisterService("svc", cfg)
	for _, user := range users {
		if err := s.AddUser("svc", user); err != nil {
			t.Fatal(err)
		}
	}

	var once sync.Once
	stop := func() { once.Do(s.Stop) }
	t.Cleanup(stop)
	return s, stop
}

func enqueueTest(t *testing.T, s *Scheduler, user string, task Task) {
	t.Helper()
	if err := s.Enqueue("svc", user, task); err != nil {
		t.Fatal(err)
	}
}

func waitFor(t *testing.T, done <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestSyncProgressPublishesOnce(t *testing.T) {
	p := newSyncProgress("pixiv")
	p.taskStarted()
	if p.end(notification.OutcomeCancelled, nil) {
		t.Error("end published while a task was executing")
	}
	if !p.taskFinished([]notification.ItemResult{{Kind: notification.ItemAdded, Ref: "1"}}) {
		t.Error("taskFinished didn't publish a sync that ended during the task")
	}
	if len(p.report.Added) != 1 || p.report.Outcome != notification.OutcomeCancelled {
		t.Errorf("report = %+v, want the task's result and the cancelled outcome", p.report)
	}

	p.taskStarted()
	if p.taskFinished([]notification.ItemResult{{Kind: notification.ItemAdded, Ref: "2"}}) || len(p.report.Added) != 1 {
		t.Error("results after publishing were recorded or published again")
	}
	if p.end(notification.OutcomeFailed, nil) || p.report.Outcome != notification.OutcomeCancelled {
		t.Error("a second end changed or republished the report")
	}

	idle := newSyncProgress("pixiv")
	if !idle.end(notification.OutcomeCompleted, nil) {
		t.Error("end with no task executing didn't publish")
	}
}

func TestEnqueueAttachesActiveSync(t *testing.T) {
	s := NewScheduler()
	s.RegisterService("svc", ServiceConfig{})
	if err := s.AddUser("svc", "u"); err != nil {
		t.Fatal(err)
	}
	noop := func(context.Context) ([]notification.ItemResult, error) { return nil, nil }

	first := newSyncProgress("svc")
	s.activeSyncs.Store(syncKey("svc", "u"), first)
	if err := s.Enqueue("svc", "u", noop); err != nil {
		t.Fatal(err)
	}
	second := newSyncProgress("svc")
	s.activeSyncs.Store(syncKey("svc", "u"), second)
	if err := s.Enqueue("svc", "u", noop); err != nil {
		t.Fatal(err)
	}

	ss, _ := s.service("svc")
	for i, want := range []*syncProgress{first, second} {
		next, _, ok := ss.nextTask()
		if !ok || next.progress != want {
			t.Errorf("task %d attached to %p, want %p", i, next.progress, want)
		}
	}
}

func TestCompleteSyncLeavesNewerSync(t *testing.T) {
	s := NewScheduler()
	stale := newSyncProgress("svc")
	stale.end(notification.OutcomeCancelled, nil) // already ended, so nothing is published
	newer := newSyncProgress("svc")
	s.activeSyncs.Store(syncKey("svc", "u"), newer)

	s.completeSync("svc", "u", stale, nil)

	if active, ok := s.activeSyncs.Load(syncKey("svc", "u")); !ok || active != newer {
		t.Error("a stale sync's done released the newer sync's guard")
	}
}

func TestPausedTaskLetsOtherUsersRun(t *testing.T) {

	//2 per turn, so a run loop that kept A's turn after the pause would resume A before B
	s, _ := newTestScheduler(t, ServiceConfig{QueriesPerTurn: 2}, "turn-a", "turn-b")
	var log eventLog
	finished := make(chan struct{})

	enqueueTest(t, s, "turn-a", func(ctx context.Context) ([]notification.ItemResult, error) {
		log.add("A starts")
		if err := imageset.Pause(ctx); err != nil {
			t.Errorf("pause: %v", err)
		}
		log.add("A ends")
		close(finished)
		return nil, nil
	})
	enqueueTest(t, s, "turn-b", func(context.Context) ([]notification.ItemResult, error) {
		log.add("B")
		return nil, nil
	})

	s.Start()
	waitFor(t, finished, "A to finish")

	if got, want := log.list(), []string{"A starts", "B", "A ends"}; !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v: B must run while A is paused", got, want)
	}
}

func TestPausedTaskKeepsUserOrder(t *testing.T) {

	s, _ := newTestScheduler(t, ServiceConfig{}, "order-a")
	var log eventLog
	finished := make(chan struct{})

	enqueueTest(t, s, "order-a", func(ctx context.Context) ([]notification.ItemResult, error) {
		for range 2 {
			if err := imageset.Pause(ctx); err != nil {
				t.Errorf("pause: %v", err)
			}
		}
		log.add("task 1 ends")
		return nil, nil
	})
	enqueueTest(t, s, "order-a", func(context.Context) ([]notification.ItemResult, error) {
		log.add("task 2 starts")
		close(finished)
		return nil, nil
	})

	s.Start()
	waitFor(t, finished, "task 2")

	if got, want := log.list(), []string{"task 1 ends", "task 2 starts"}; !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v: a paused task resumes before the user's next task", got, want)
	}
}

func TestPauseGivesUpAfterLimit(t *testing.T) {

	s, _ := newTestScheduler(t, ServiceConfig{}, "limit-a")
	finished := make(chan struct{})
	var pauses int
	var gaveUp, again error
	var againTook time.Duration

	enqueueTest(t, s, "limit-a", func(ctx context.Context) ([]notification.ItemResult, error) {
		defer close(finished)
		for {
			if gaveUp = imageset.Pause(ctx); gaveUp != nil {
				break
			}
			pauses++
		}

		start := time.Now()
		again = imageset.Pause(ctx)
		againTook = time.Since(start)
		return nil, nil
	})

	s.Start()
	waitFor(t, finished, "the task to give up")

	if pauses != maxTaskPauses || !errors.Is(gaveUp, errTaskGaveUp) {
		t.Errorf("%d pauses then %v; want %d, then errTaskGaveUp", pauses, gaveUp, maxTaskPauses)
	}

	if !errors.Is(again, errTaskGaveUp) || againTook > 100*time.Millisecond {
		t.Errorf("pausing after giving up: %v after %v; want errTaskGaveUp at once", again, againTook)
	}
}

func TestResumeWaitsForDelay(t *testing.T) {

	const delay = 30 * time.Millisecond
	s, _ := newTestScheduler(t, ServiceConfig{Delay: delay}, "delay-a")
	finished := make(chan struct{})
	var paused time.Duration

	enqueueTest(t, s, "delay-a", func(ctx context.Context) ([]notification.ItemResult, error) {
		defer close(finished)
		start := time.Now()
		if err := imageset.Pause(ctx); err != nil {
			t.Errorf("pause: %v", err)
		}
		paused = time.Since(start)
		return nil, nil
	})

	s.Start()
	waitFor(t, finished, "the resume")

	if paused < delay {
		t.Errorf("resumed after %v, want at least the %v Delay", paused, delay)
	}
}

// pauseBesideBlockedTask enqueues a task for user that pauses once, then a task
// for other that blocks until release closes, so the run loop is busy while user's
// task stays paused. It starts s and returns once both have happened; pauseErr
// receives the pause's result.
func pauseBesideBlockedTask(t *testing.T, s *Scheduler, user, other string, release <-chan struct{}) (pauseErr <-chan error, taskDone <-chan struct{}) {
	t.Helper()

	errs := make(chan error, 2)
	done := make(chan struct{})
	enqueueTest(t, s, user, func(ctx context.Context) ([]notification.ItemResult, error) {
		defer close(done)
		errs <- imageset.Pause(ctx)

		//once cancelled, a task never pauses again
		start := time.Now()
		if err := imageset.Pause(ctx); err == nil || time.Since(start) > 100*time.Millisecond {
			t.Errorf("pausing after cancellation: %v after %v; want an error at once", err, time.Since(start))
		}
		return nil, nil
	})

	blocked := make(chan struct{})
	enqueueTest(t, s, other, func(context.Context) ([]notification.ItemResult, error) {
		close(blocked)
		<-release
		return nil, nil
	})

	s.Start()
	waitFor(t, blocked, "the other user's task")
	return errs, done
}

func TestRemoveUserWakesPausedTask(t *testing.T) {

	s, _ := newTestScheduler(t, ServiceConfig{}, "remove-a", "remove-b")
	progress := newSyncProgress("svc")
	s.activeSyncs.Store(syncKey("svc", "remove-a"), progress)
	release := make(chan struct{})
	defer close(release)
	pauseErr, taskDone := pauseBesideBlockedTask(t, s, "remove-a", "remove-b", release)

	s.cancelSync("svc", "remove-a")
	if countPublished("remove-a") != 0 {
		t.Fatal("the report was published while its task was paused")
	}

	if err := s.RemoveUser("svc", "remove-a"); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-pauseErr:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("pause returned %v, want a cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the paused task was never woken")
	}
	waitFor(t, taskDone, "the woken task to return")

	if n := countPublished("remove-a"); n != 1 {
		t.Errorf("report published %d times, want once", n)
	}
}

func TestStopWakesPausedTask(t *testing.T) {

	s, stop := newTestScheduler(t, ServiceConfig{}, "stop-a", "stop-b")
	release := make(chan struct{})
	defer close(release)
	pauseErr, taskDone := pauseBesideBlockedTask(t, s, "stop-a", "stop-b", release)

	stop()

	select {
	case err := <-pauseErr:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("pause returned %v, want a cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Stop never woke the paused task")
	}
	waitFor(t, taskDone, "the woken task to return")

	//the run loop is still busy with the other task, so only the real pause's unpause entry
	//waits in the queue; a cancelled task must not queue another
	ss, _ := s.service("svc")
	ss.mu.Lock()
	user := ss.byUser["stop-a"]
	ss.mu.Unlock()
	user.mu.Lock()
	queued := len(user.tasks)
	user.mu.Unlock()
	if queued != 1 {
		t.Errorf("%d entries in the stopped user's queue, want only the one unpause", queued)
	}
}

func TestTaskPanicIsReported(t *testing.T) {

	s, _ := newTestScheduler(t, ServiceConfig{}, "panic-a")
	progress := newSyncProgress("svc")
	s.activeSyncs.Store(syncKey("svc", "panic-a"), progress)
	secondRan := make(chan struct{})

	enqueueTest(t, s, "panic-a", func(context.Context) ([]notification.ItemResult, error) {
		panic("a bug in a task")
	})
	enqueueTest(t, s, "panic-a", func(context.Context) ([]notification.ItemResult, error) {
		close(secondRan)
		return nil, nil
	})

	s.Start()
	waitFor(t, secondRan, "the task after the panic")

	progress.mu.Lock()
	failed := slices.Clone(progress.report.Failed)
	progress.mu.Unlock()
	if len(failed) != 1 || failed[0].Reason != "server error" {
		t.Errorf("failed items %+v, want one with reason \"server error\"", failed)
	}
}
