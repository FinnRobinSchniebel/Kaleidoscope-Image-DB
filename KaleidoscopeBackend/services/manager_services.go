package services

import (
	"container/heap"
	"fmt"
	"sync"
	"time"
)

// Task is a unit of work submitted to the scheduler by a service integration.
type Task func() error

// ServiceConfig defines the rate-limiting behaviour for a single API service.
type ServiceConfig struct {
	// Delay is the minimum pause enforced between consecutive requests to this service.
	Delay time.Duration
	// QueriesPerTurn is how many requests are made for one user before rotating to the next.
	QueriesPerTurn int
}

type userEntry struct {
	userId      string
	tasks       []Task
	queriesDone int
	mu          sync.Mutex
}

// periodicEntry is a single scheduled recurring job, ordered in the scheduler's
// heap by nextRun (soonest first).
type periodicEntry struct {
	key       string // "service/userId"
	interval  time.Duration
	job       func()
	nextRun   time.Time
	index     int // position in periodicHeap; -1 when not in the heap
	cancelled bool
}

type serviceScheduler struct {
	config  ServiceConfig
	users   []*userEntry
	byUser  map[string]*userEntry
	mu      sync.Mutex
	lastRun time.Time
	stop    chan struct{}
}

// CredentialHook is called whenever credentials for a service are created or updated.
type CredentialHook func(userId string, creds ExternalApiKeys)

// CredentialTestHook validates a set of credentials against the external service.
// It is called synchronously from the Register endpoint; the error (if any) is
// returned to the caller so the frontend can display connection status.
type CredentialTestHook func(userId string, creds ExternalApiKeys) error

// ServiceProvider is implemented by every external service integration.
// Register a provider once at startup via DefaultScheduler.RegisterProvider.
type ServiceProvider interface {
	Name() string
	Config() ServiceConfig
	TestCredentials(userId string, creds ExternalApiKeys) error
	OnCredentialsUpdated(userId string, creds ExternalApiKeys)
	OnCredentialsRemoved(userId string)
	RestoreSchedules()
	Sync(userId string) error
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

// Scheduler coordinates task execution across multiple services and users,
// enforcing per-service rate limits and round-robin user rotation.
type Scheduler struct {
	services      map[string]*serviceScheduler
	mu            sync.RWMutex
	credHooks     map[string]CredentialHook     // keyed by service name
	credTestHooks map[string]CredentialTestHook // keyed by service name
	providers     map[string]ServiceProvider    // keyed by service name

	// sleeps until the soonest-due entry in periodicHeap, fires it in its own
	// goroutine, reschedules it, and goes back to sleep. This avoids spinning
	periodicMu    sync.Mutex
	periodicHeap  periodicHeap
	periodicByKey map[string]*periodicEntry // keyed by "service/userId"
	periodicWake  chan struct{}             // nudges runPeriodic when the schedule changes
	periodicStop  chan struct{}
	periodicOnce  sync.Once
}

// DefaultScheduler is the package-level scheduler used by all service integrations.
var DefaultScheduler = NewScheduler()

func NewScheduler() *Scheduler {
	return &Scheduler{
		services:      make(map[string]*serviceScheduler),
		credHooks:     make(map[string]CredentialHook),
		credTestHooks: make(map[string]CredentialTestHook),
		providers:     make(map[string]ServiceProvider),
		periodicByKey: make(map[string]*periodicEntry),
		periodicWake:  make(chan struct{}, 1),
		periodicStop:  make(chan struct{}),
	}
}

// RegisterCredentialHook registers a callback to be fired whenever credentials
// for serviceName are created or updated via the Register API endpoint.
// Replaces any previously registered hook for the same service.
func (s *Scheduler) RegisterCredentialHook(serviceName string, hook CredentialHook) {
	s.mu.Lock()
	s.credHooks[serviceName] = hook
	s.mu.Unlock()
}

// fireCredentialHook calls the registered hook for serviceName, if any.
func (s *Scheduler) fireCredentialHook(serviceName, userId string, creds ExternalApiKeys) {
	s.mu.RLock()
	hook := s.credHooks[serviceName]
	s.mu.RUnlock()
	if hook != nil {
		hook(userId, creds)
	}
}

// RegisterCredentialTestHook registers a synchronous validator for serviceName.
// The hook is called by the Register endpoint and its error is returned to the client.
func (s *Scheduler) RegisterCredentialTestHook(serviceName string, hook CredentialTestHook) {
	s.mu.Lock()
	s.credTestHooks[serviceName] = hook
	s.mu.Unlock()
}

// TestCredentials runs the registered test hook for serviceName, if any.
// Returns nil when no hook is registered (treat as untestable, not an error).
func (s *Scheduler) TestCredentials(serviceName, userId string, creds ExternalApiKeys) error {
	s.mu.RLock()
	hook := s.credTestHooks[serviceName]
	s.mu.RUnlock()
	if hook == nil {
		return fmt.Errorf("no hook for testing credentials was found")
	}
	return hook(userId, creds)
}

// RegisterService registers an API service with its scheduling config.
// Must be called before Start.
func (s *Scheduler) RegisterService(name string, cfg ServiceConfig) {
	if cfg.QueriesPerTurn <= 0 {
		cfg.QueriesPerTurn = 1
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.services[name] = &serviceScheduler{
		config: cfg,
		byUser: make(map[string]*userEntry),
		stop:   make(chan struct{}),
	}
	fmt.Printf("Services: A new service \"%s\" has been Registered by the service schedule. Delay: %s, QpT: %d", name, cfg.Delay, cfg.QueriesPerTurn)
}

// AddUser adds a user to a service's round-robin rotation.
// Safe to call after Start. No-op if the user is already present.
func (s *Scheduler) AddUser(serviceName, userId string) error {
	s.mu.RLock()
	ss, ok := s.services[serviceName]
	s.mu.RUnlock()
	if !ok {
		fmt.Printf("Error: Services: failed to add user [%s] to service: %s. Service not registered", userId, serviceName)
		return fmt.Errorf("service %q not registered", serviceName)
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()
	if _, exists := ss.byUser[userId]; exists {
		return nil
	}
	u := &userEntry{userId: userId}
	ss.users = append(ss.users, u)
	ss.byUser[userId] = u

	fmt.Printf("Services: Added user [%s] to service: %s", userId, serviceName)
	return nil
}

// RemoveUser removes a user from a service's rotation and discards their queued tasks.
func (s *Scheduler) RemoveUser(serviceName, userId string) error {
	s.mu.RLock()
	ss, ok := s.services[serviceName]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("service %q not registered", serviceName)
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()
	if _, exists := ss.byUser[userId]; !exists {
		return nil
	}
	delete(ss.byUser, userId)
	for i, u := range ss.users {
		if u.userId == userId {
			ss.users = append(ss.users[:i], ss.users[i+1:]...)
			break
		}
	}

	fmt.Printf("Services: Removed user [%s] from service: %s", userId, serviceName)
	return nil
}

// Enqueue submits a task for a user on the named service.
// The user is added to the rotation automatically if not already present.
func (s *Scheduler) Enqueue(serviceName, userId string, task Task) error {
	s.mu.RLock()
	ss, ok := s.services[serviceName]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("service %q not registered", serviceName)
	}

	ss.mu.Lock()
	u, exists := ss.byUser[userId]
	if !exists {
		u = &userEntry{userId: userId}
		ss.users = append(ss.users, u)
		ss.byUser[userId] = u
	}
	ss.mu.Unlock()

	u.mu.Lock()
	u.tasks = append(u.tasks, task)
	u.mu.Unlock()
	return nil
}

// Start launches a background goroutine for each registered service, plus the
// single goroutine that drives all periodic jobs.
func (s *Scheduler) Start() {
	s.mu.RLock()
	for _, ss := range s.services {
		go ss.run()
	}
	s.mu.RUnlock()

	s.periodicOnce.Do(func() {
		go s.runPeriodic()
	})
}

// Stop signals all service goroutines and the periodic runner to exit.
func (s *Scheduler) Stop() {
	s.mu.RLock()
	for _, ss := range s.services {
		close(ss.stop)
	}
	s.mu.RUnlock()

	close(s.periodicStop)
}

// SchedulePeriodic registers a recurring job for userId on serviceName.
// job is called once per interval; any previous schedule for this user+service
// is replaced. The job is not called immediately — the first call happens after
// one full interval has elapsed from the time it is scheduled.
//
// All periodic jobs across every service share a single driver goroutine
// (see runPeriodic): jobs are kept in a min-heap ordered by their next run
// time, and the driver sleeps only until the soonest one is due, firing it in
// its own goroutine at that point. This keeps periodic scheduling to one
// long-lived goroutine no matter how many jobs are registered.
func (s *Scheduler) SchedulePeriodic(serviceName, userId string, interval time.Duration, job func()) error {
	s.mu.RLock()
	_, ok := s.services[serviceName]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("service %q not registered", serviceName)
	}

	key := serviceName + "/" + userId
	entry := &periodicEntry{
		key:      key,
		interval: interval,
		job:      job,
		nextRun:  time.Now().Add(interval),
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
	key := serviceName + "/" + userId
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
// It sleeps until the soonest-due entry in periodicHeap, then fires every
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
	for _, e := range due {
		if e.cancelled {
			continue
		}
		e.nextRun = now.Add(e.interval)
		heap.Push(&s.periodicHeap, e)
	}
	s.periodicMu.Unlock()

	for _, e := range due {
		if e.cancelled {
			continue
		}
		go e.job()
	}
}

// RegisterProvider registers a service via its ServiceProvider interface,
// replacing the three separate Register* calls that were previously required.
func (s *Scheduler) RegisterProvider(p ServiceProvider) {
	s.RegisterService(p.Name(), p.Config())
	s.RegisterCredentialHook(p.Name(), p.OnCredentialsUpdated)
	s.RegisterCredentialTestHook(p.Name(), p.TestCredentials)
	s.mu.Lock()
	s.providers[p.Name()] = p
	s.mu.Unlock()
}

// RestoreAllSchedules calls RestoreSchedules on every registered provider.
// Call once at startup after Start.
func (s *Scheduler) RestoreAllSchedules() {
	s.mu.RLock()
	ps := make([]ServiceProvider, 0, len(s.providers))
	for _, p := range s.providers {
		ps = append(ps, p)
	}
	s.mu.RUnlock()
	for _, p := range ps {
		p.RestoreSchedules()
	}
}

// SyncUser triggers an on-demand sync for userId on the named service.
func (s *Scheduler) SyncUser(serviceName, userId string) error {
	s.mu.RLock()
	p, ok := s.providers[serviceName]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("service %q not registered", serviceName)
	}
	return p.Sync(userId)
}

// RemoveService cleans up a user's service: fires OnCredentialsRemoved, cancels
// any periodic job, removes the user from the scheduler rotation, and deletes
// the stored credentials from the database.
func (s *Scheduler) RemoveService(serviceName, userId string) error {
	s.mu.RLock()
	p, ok := s.providers[serviceName]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("service %q not registered", serviceName)
	}
	p.OnCredentialsRemoved(userId)
	s.CancelPeriodic(serviceName, userId)
	_ = s.RemoveUser(serviceName, userId)
	return DeleteServiceInfo(userId, serviceName)
}

func (ss *serviceScheduler) run() {
	for {
		select {
		case <-ss.stop:
			return
		default:
		}

		task, ok := ss.nextTask()
		if !ok {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		if wait := ss.config.Delay - time.Since(ss.lastRun); wait > 0 {
			time.Sleep(wait)
		}

		task()
		ss.lastRun = time.Now()
	}
}

// nextTask picks the next task from the front user.
// When a user exhausts their QueriesPerTurn quota, they are rotated to the back.
// Users with no pending tasks are skipped (and rotated past).
func (ss *serviceScheduler) nextTask() (Task, bool) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	for range len(ss.users) {
		front := ss.users[0]
		front.mu.Lock()

		if len(front.tasks) == 0 {
			front.mu.Unlock()
			ss.users = append(ss.users[1:], ss.users[0])
			continue
		}

		task := front.tasks[0]
		front.tasks = front.tasks[1:]
		front.queriesDone++
		rotate := front.queriesDone >= ss.config.QueriesPerTurn
		if rotate {
			front.queriesDone = 0
		}
		front.mu.Unlock()

		if rotate {
			ss.users = append(ss.users[1:], ss.users[0])
		}

		return task, true
	}

	return nil, false
}
