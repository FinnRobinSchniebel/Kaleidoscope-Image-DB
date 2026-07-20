package services

import (
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

type serviceScheduler struct {
	config  ServiceConfig
	users   []*userEntry
	byUser  map[string]*userEntry
	mu      sync.Mutex
	lastRun time.Time
	stop    chan struct{}
}

// ServiceProvider is implemented by every external service integration.
// Register a provider once at startup via DefaultScheduler.RegisterProvider.
type ServiceProvider interface {
	Name() string
	Config() ServiceConfig
	TestCredentials(userId string, creds ExternalApiKeys) error
	OnCredentialsUpdated(userId string, creds ExternalApiKeys)
	OnCredentialsRemoved(userId string)
	OnSyncSettingsUpdated(userId string)
	RestoreSchedules()
	Sync(userId string) error
}

// Scheduler coordinates task execution across multiple services and users,
// enforcing per-service rate limits and round-robin user rotation.
type Scheduler struct {
	services  map[string]*serviceScheduler
	mu        sync.RWMutex
	providers map[string]ServiceProvider // keyed by service name

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

func NewScheduler() *Scheduler {
	return &Scheduler{
		services:      make(map[string]*serviceScheduler),
		providers:     make(map[string]ServiceProvider),
		periodicByKey: make(map[string]*periodicEntry),
		periodicWake:  make(chan struct{}, 1),
		periodicStop:  make(chan struct{}),
	}
}

// service returns the serviceScheduler registered for name, if any.
func (s *Scheduler) service(name string) (*serviceScheduler, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ss, ok := s.services[name]
	return ss, ok
}

// provider returns the ServiceProvider registered for name, if any.
func (s *Scheduler) provider(name string) (ServiceProvider, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.providers[name]
	return p, ok
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
	ss, ok := s.service(serviceName)
	if !ok {
		fmt.Printf("Error: Services: failed to add user [%s] to service: %s. Service not registered", userId, serviceName)
		return fmt.Errorf("service %q not registered", serviceName)
	}

	if _, created := ss.ensureUser(userId); created {
		fmt.Printf("Services: Added user [%s] to service: %s", userId, serviceName)
	}
	return nil
}

// RemoveUser removes a user from a service's rotation and discards their queued tasks.
func (s *Scheduler) RemoveUser(serviceName, userId string) error {
	ss, ok := s.service(serviceName)
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
	ss, ok := s.service(serviceName)
	if !ok {
		return fmt.Errorf("service %q not registered", serviceName)
	}

	u, _ := ss.ensureUser(userId)

	u.mu.Lock()
	u.tasks = append(u.tasks, task)
	u.mu.Unlock()
	return nil
}

// RegisterProvider registers a service via its ServiceProvider interface.
func (s *Scheduler) RegisterProvider(p ServiceProvider) {
	s.RegisterService(p.Name(), p.Config())
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
	p, ok := s.provider(serviceName)
	if !ok {
		return fmt.Errorf("service %q not registered", serviceName)
	}
	return p.Sync(userId)
}

// RemoveService cleans up a user's service: fires OnCredentialsRemoved, cancels
// any periodic job, removes the user from the scheduler rotation, and deletes
// the stored credentials from the database.
func (s *Scheduler) RemoveService(serviceName, userId string) error {
	p, ok := s.provider(serviceName)
	if !ok {
		return fmt.Errorf("service %q not registered", serviceName)
	}
	p.OnCredentialsRemoved(userId)
	s.CancelPeriodic(serviceName, userId)
	_ = s.RemoveUser(serviceName, userId)
	return DeleteServiceInfo(userId, serviceName)
}

// ensureUser returns the userEntry for userId within ss, creating it and
// adding it to the rotation if it doesn't already exist. The bool reports
// whether the entry was newly created.
func (ss *serviceScheduler) ensureUser(userId string) (*userEntry, bool) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if u, exists := ss.byUser[userId]; exists {
		return u, false
	}
	u := &userEntry{userId: userId}
	ss.users = append(ss.users, u)
	ss.byUser[userId] = u
	return u, true
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

// fireCredentialHook calls the registered provider's OnCredentialsUpdated for
// serviceName, if any.
func (s *Scheduler) fireCredentialHook(serviceName, userId string, creds ExternalApiKeys) {
	if p, ok := s.provider(serviceName); ok {
		p.OnCredentialsUpdated(userId, creds)
	}
}

// fireSyncSettingsHook calls the registered provider's OnSyncSettingsUpdated
// for serviceName, if any.
func (s *Scheduler) fireSyncSettingsHook(serviceName, userId string) {
	if p, ok := s.provider(serviceName); ok {
		p.OnSyncSettingsUpdated(userId)
	}
}

// TestCredentials runs the registered provider's TestCredentials for serviceName.
func (s *Scheduler) TestCredentials(serviceName, userId string, creds ExternalApiKeys) error {
	p, ok := s.provider(serviceName)
	if !ok {
		return fmt.Errorf("no hook for testing credentials was found")
	}
	return p.TestCredentials(userId, creds)
}
