package services

import (
	"container/heap"
	"fmt"
	"time"
)

// periodicEntry is a single scheduled recurring job, ordered in the scheduler's
// heap by nextRun (soonest first).
type periodicEntry struct {
	interval  time.Duration
	job       func()
	nextRun   time.Time
	index     int // position in periodicHeap; -1 when not in the heap
	cancelled bool
}

// syncKey builds the map key used to look up a user's periodic job on a service.
func syncKey(serviceName, userId string) string {
	return serviceName + "/" + userId
}

// periodicHeap is a min-heap of periodicEntry ordered by nextRun.
type periodicHeap []*periodicEntry

func (h periodicHeap) Len() int           { return len(h) }
func (h periodicHeap) Less(i, j int) bool { return h[i].nextRun.Before(h[j].nextRun) }
func (h periodicHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}
func (h *periodicHeap) Push(x any) {
	e := x.(*periodicEntry)
	e.index = len(*h)
	*h = append(*h, e)
}
func (h *periodicHeap) Pop() any {
	old := *h
	n := len(old)
	e := old[n-1]
	old[n-1] = nil
	e.index = -1
	*h = old[:n-1]
	return e
}

// SchedulePeriodic registers a recurring sync for userId on serviceName, run
// every intervalHours. sync is launched once per interval through the same
// guarded path as manual syncs (see Scheduler.runSync): it won't fire if a
// sync for this service+user is already in progress, and LastSynced is
// stamped at launch. Any previous schedule for this user+service is replaced.
// intervalHours == 0 cancels any existing schedule instead (equivalent to
// calling CancelPeriodic) — individual service providers don't need to check
// for this themselves. Positive values are clamped up to MinScheduleInterval
// hours.
//
// lastSynced determines the first run time: if it is zero (never synced) or
// lastSynced+interval already elapsed (e.g. the schedule lapsed during server
// downtime), the job fires immediately to catch up. Otherwise it fires at
// lastSynced+interval, preserving the existing cadence instead of resetting it
// to now+interval.
func (s *Scheduler) SchedulePeriodic(serviceName, userId string, intervalHours int64, lastSynced time.Time, sync SyncFunc) error {
	if intervalHours == 0 {
		s.CancelPeriodic(serviceName, userId)
		return nil
	}

	if _, ok := s.service(serviceName); !ok {
		return fmt.Errorf("service %q not registered", serviceName)
	}
	interval := time.Duration(max(intervalHours, MinScheduleInterval)) * time.Hour

	nextRun := time.Now()
	if !lastSynced.IsZero() {
		if due := lastSynced.Add(interval); due.After(nextRun) {
			nextRun = due
		}
	}

	key := syncKey(serviceName, userId)
	entry := &periodicEntry{
		interval: interval,
		job: func() {
			if err := s.runSync(serviceName, userId, "periodic", sync); err != nil {
				fmt.Printf("ERROR: Services: periodic sync failed for user: %s, service: %s: %v", userId, serviceName, err)
			}
		},
		nextRun: nextRun,
	}

	s.periodicMu.Lock()
	if existing, ok := s.periodicByKey[key]; ok {
		existing.cancelled = true
		if existing.index >= 0 {
			heap.Remove(&s.periodicHeap, existing.index)
		}
	}
	s.periodicByKey[key] = entry
	heap.Push(&s.periodicHeap, entry)
	s.periodicMu.Unlock()

	s.wakePeriodicRunner()
	return nil
}

// CancelPeriodic stops the recurring job for userId on serviceName, if any.
func (s *Scheduler) CancelPeriodic(serviceName, userId string) {
	key := syncKey(serviceName, userId)
	s.periodicMu.Lock()
	if entry, ok := s.periodicByKey[key]; ok {
		entry.cancelled = true
		if entry.index >= 0 {
			heap.Remove(&s.periodicHeap, entry.index)
		}
		delete(s.periodicByKey, key)
	}
	s.periodicMu.Unlock()
}

// wakePeriodicRunner nudges runPeriodic to re-evaluate the schedule, e.g.
// because a newly added job is due sooner than whatever it was sleeping on.
func (s *Scheduler) wakePeriodicRunner() {
	select {
	case s.periodicWake <- struct{}{}:
	default:
	}
}

// runPeriodic is the single goroutine responsible for all periodic jobs.
// Sleeps until the soonest-due entry in periodicHeap, then fires every
// entry that has come due (each in its own goroutine) and reschedules them,
// before going back to sleep. New jobs and cancellations wake it early via
// periodicWake so it never sleeps past a newly-added earlier job.
func (s *Scheduler) runPeriodic() {
	for {
		s.periodicMu.Lock()
		hasNext := s.periodicHeap.Len() > 0
		var wait time.Duration
		if hasNext {
			wait = max(time.Until(s.periodicHeap[0].nextRun), 0)
		}
		s.periodicMu.Unlock()

		if !hasNext {
			select {
			case <-s.periodicStop:
				return
			case <-s.periodicWake:
				continue
			}
		}

		timer := time.NewTimer(wait)
		select {
		case <-s.periodicStop:
			timer.Stop()
			return
		case <-s.periodicWake:
			timer.Stop()
			continue
		case <-timer.C:
			s.firePeriodicDue()
		}
	}
}

// firePeriodicDue pops every entry whose nextRun has arrived, reschedules
// each for now+interval, and launches their jobs in new goroutines. Entries
// cancelled or replaced between being popped and rescheduled are dropped.
func (s *Scheduler) firePeriodicDue() {
	now := time.Now()

	s.periodicMu.Lock()
	due := make([]*periodicEntry, 0, 1)
	for s.periodicHeap.Len() > 0 && !s.periodicHeap[0].nextRun.After(now) {
		e := heap.Pop(&s.periodicHeap).(*periodicEntry)
		due = append(due, e)
	}
	toRun := make([]*periodicEntry, 0, len(due))
	for _, e := range due {
		if e.cancelled {
			continue
		}
		e.nextRun = now.Add(e.interval)
		heap.Push(&s.periodicHeap, e)
		toRun = append(toRun, e)
	}
	s.periodicMu.Unlock()

	for _, e := range toRun {
		go e.job()
	}
}
