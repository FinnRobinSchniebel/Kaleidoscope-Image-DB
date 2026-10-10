package services

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"testing"
)

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
	noop := func() ([]notification.ItemResult, error) { return nil, nil }

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
