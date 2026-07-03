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

// Scheduler coordinates task execution across multiple services and users,
// enforcing per-service rate limits and round-robin user rotation.
type Scheduler struct {
	services map[string]*serviceScheduler
	mu       sync.RWMutex
}

// DefaultScheduler is the package-level scheduler used by all service integrations.
var DefaultScheduler = NewScheduler()

func NewScheduler() *Scheduler {
	return &Scheduler{
		services: make(map[string]*serviceScheduler),
	}
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
}

// AddUser adds a user to a service's round-robin rotation.
// Safe to call after Start. No-op if the user is already present.
func (s *Scheduler) AddUser(serviceName, userId string) error {
	s.mu.RLock()
	ss, ok := s.services[serviceName]
	s.mu.RUnlock()
	if !ok {
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

// Start launches a background goroutine for each registered service.
func (s *Scheduler) Start() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, ss := range s.services {
		go ss.run()
	}
}

// Stop signals all service goroutines to exit.
func (s *Scheduler) Stop() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, ss := range s.services {
		close(ss.stop)
	}
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
