package services

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// CREATING A SERVICE
// This is important information on how to design a service (like the pixiv service) and what needs to be implemented for it to work with the schedulers
// 1. each service must implement the Service provider interface functions
// 2. Sync must call done(err) exactly once when the sync finishes (nil) or fails (the error) to allow for a new sync to start for the user and to publish its report. Failure to do so will result in the user being unable to sync again until a server restart
// 3. Each task returns the results of what it did; the scheduler records them in the sync's report
// 4. Rotation membership (AddUser/RemoveUser), periodic scheduling, and the active-sync guard are all managed centrally by the Scheduler (see RestoreAllSchedules, fireCredentialHook, fireSyncSettingsHook, RemoveService), which schedules jobs using the provider's own Sync method. OnCredentialsUpdated/OnCredentialsRemoved only need to handle service-specific state, e.g. invalidating a cached session.
// 5. Each task gets a ctx: pass it to every imageset call and download, and never take imageset locks yourself. A busy library lock pauses the task while the scheduler serves other users, and removing the service cancels it. A task may call imageset.Pause(ctx) to yield on its own.

// Task is a unit of work submitted to the scheduler by a service integration.
// It returns what it did, for the sync's report; an error is only logged. ctx is
// cancelled when the user's service is removed or the scheduler stops; pass it to
// every imageset call and download, so the task can pause on a busy lock or stop.
type Task func(ctx context.Context) ([]notification.ItemResult, error)

// publishReport stores a finished sync report; a variable so tests can record
// reports instead of writing them.
var publishReport = notification.Publish

// logRecoveredPanic recovers a panic in the goroutine it's deferred in and logs
// it with its stack. Defer it directly: recover only works there.
func logRecoveredPanic(what string) {
	if r := recover(); r != nil {
		log.Printf("------ Warning: %s panicked: %v ------\n%s", what, r, debug.Stack())
	}
}

// taskRun is one task between its start and its final event, which may pause
// several times on the way.
type taskRun struct {
	scheduler *serviceScheduler
	user      *userEntry
	ctx       context.Context
	progress  *syncProgress
	resume    chan struct{}  //the run loop signals a paused task to continue
	events    chan taskEvent //buffered, so a cancelled task's unread final event never blocks
	pauses    int            //touched only by the task's own goroutine
}

// taskEvent is what a task reports to the run loop: that it paused, or that it
// finished with these results.
type taskEvent struct {
	paused  bool
	results []notification.ItemResult
	err     error
}

// SyncFunc type is used to pass ServiceProvider.Sync(...) function between functions.
// Warning: This function should not be called outside the manager_services and periodic scheduler!
// Calling it for a service will begin the sync process for a service.
// When a sync concludes or fails it MUST call done exactly once, with nil or the failure.
type SyncFunc func(userId string, done func(error)) error

// ServiceConfig defines the rate-limiting behaviour for a single API service.
type ServiceConfig struct {
	// Delay is the minimum pause enforced between consecutive requests to this service.
	Delay time.Duration
	// QueriesPerTurn is how many requests are made for one user before rotating to the next.
	QueriesPerTurn int
}

type userEntry struct {
	userId      string
	ctx         context.Context //parent of the user's task contexts
	cancel      context.CancelFunc
	tasks       []queuedTask
	queriesDone int
	mu          sync.Mutex
}

// queuedTask is a task plus the sync it was queued for (nil if none was active).
type queuedTask struct {
	task     Task
	progress *syncProgress
	run      *taskRun //set on an unpause entry: resume this paused run instead of starting task
}

// syncProgress is the in-memory report of one running sync. Results arrive on
// the service's scheduler goroutine while RemoveService can end the sync from
// an HTTP goroutine, so every field is guarded by mu. Once published, the
// report is never changed again.
type syncProgress struct {
	mu        sync.Mutex
	report    *notification.ImportReport
	executing bool //one of this sync's tasks is running or paused right now
	published bool
}

func newSyncProgress(serviceName string) *syncProgress {
	return &syncProgress{report: notification.NewImportReport(serviceName)}
}

func (p *syncProgress) taskStarted() {
	p.mu.Lock()
	p.executing = true
	p.mu.Unlock()
}

// taskFinished adds a task's results and reports whether the sync ended while
// the task ran, so the report now needs publishing.
func (p *syncProgress) taskFinished(results []notification.ItemResult) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.executing = false
	if p.published {
		return false
	}
	p.report.Record(results...)
	if p.report.Finished.IsZero() {
		return false
	}
	p.published = true
	return true
}

// end finishes the report, first call only, and reports whether it can be
// published now. While a task is executing, taskFinished publishes it instead.
func (p *syncProgress) end(outcome notification.Outcome, err error) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.report.Finished.IsZero() {
		return false
	}
	p.report.Finish(outcome, err)
	if p.executing {
		return false
	}
	p.published = true
	return true
}

type serviceScheduler struct {
	name    string
	config  ServiceConfig
	users   []*userEntry
	byUser  map[string]*userEntry
	mu      sync.Mutex
	lastRun time.Time
	stop    chan struct{}
	baseCtx context.Context //parent of every user context; cancelled by Stop
}

// ServiceProvider is implemented by every external service integration.
// Register a provider once at startup via DefaultScheduler.RegisterProvider.
type ServiceProvider interface {
	Name() string                                               //returns the name of the service
	Config() ServiceConfig                                      //returns the servie configurations used for the scheduler
	TestCredentials(userId string, creds ExternalApiKeys) error // This function checks if the credentials are valid and the service can be reached
	OnCredentialsUpdated(userId string, creds ExternalApiKeys)  //handles service-specific cleanup when a credential is changed, e.g. invalidating a cached session. Rescheduling the periodic sync is handled centrally by the Scheduler after this returns.
	OnCredentialsRemoved(userId string)                         //handles service-specific cleanup when credentials are removed, e.g. invalidating a cached session. Cancelling the periodic sync and clearing the active-sync guard is handled centrally by RemoveService.
	Sync(userId string, done func(error)) error                 //starts the sync process of the service. Its return is not tied to the completion of the sync. done MUST be called once whenever a sync completes (nil) or fails (the error).
}

// Scheduler coordinates task execution across multiple services and users,
// enforcing per-service rate limits and round-robin user rotation.
type Scheduler struct {
	services  map[string]*serviceScheduler
	mu        sync.RWMutex
	providers map[string]ServiceProvider // keyed by service name

	// baseCtx is every task's root context; Stop cancels it, waking paused tasks.
	baseCtx   context.Context
	cancelAll context.CancelFunc

	// sleeps until the soonest-due entry in periodicHeap, fires it in its own
	// goroutine, reschedules it, and goes back to sleep. This avoids spinning
	periodicMu    sync.Mutex
	periodicHeap  periodicHeap
	periodicByKey map[string]*periodicEntry // keyed by "service/userId"
	periodicWake  chan struct{}             // nudges runPeriodic when the schedule changes
	periodicStop  chan struct{}
	periodicOnce  sync.Once

	// activeSyncs guards against a second concurrent sync for the same
	// service+user, keyed by periodicKey(serviceName, userId). Held for the
	// true duration of a run (see runSync), not just until the launching call
	// returns, since a run's work may continue asynchronously after that.
	activeSyncs sync.Map

	// registrationLocks serializes AddService against RemoveService for the
	// same service+user, keyed by syncKey(serviceName, userId), so the two
	// can't interleave and leave credentials/rotation state inconsistent
	// (e.g. re-adding a user to rotation after their credentials were just
	// deleted). Values are *sync.Mutex, created lazily and never removed:
	// cardinality is bounded by distinct (service, user) pairs ever seen.
	registrationLocks sync.Map
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

// Stop signals all service goroutines and the periodic runner to exit, and
// cancels every task's context, which wakes paused tasks.
func (s *Scheduler) Stop() {
	s.mu.RLock()
	for _, ss := range s.services {
		close(ss.stop)
	}
	s.mu.RUnlock()

	s.cancelAll()
	close(s.periodicStop)
}

func NewScheduler() *Scheduler {
	baseCtx, cancelAll := context.WithCancel(context.Background())
	return &Scheduler{
		services:      make(map[string]*serviceScheduler),
		providers:     make(map[string]ServiceProvider),
		baseCtx:       baseCtx,
		cancelAll:     cancelAll,
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

// IsRegisteredService reports whether a service provider has been registered under name.
// API handlers should use this to validate a client-supplied service name before it is
// used to build a DB field path (e.g. "services.<name>.credentials").
func (s *Scheduler) IsRegisteredService(name string) bool {
	_, ok := s.provider(name)
	return ok
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
		name:    name,
		config:  cfg,
		byUser:  make(map[string]*userEntry),
		stop:    make(chan struct{}),
		baseCtx: s.baseCtx,
	}
	fmt.Printf("Services: A new service \"%s\" has been Registered by the service schedule. Delay: %s, QpT: %d", name, cfg.Delay, cfg.QueriesPerTurn)
}

// RegistrationListener is notified when a user's registration in some
// service's rotation changes, so dependent state elsewhere can be kept in
// sync (e.g. tagging's computed "Untracked" tag). Owned here, not by the
// dependent package, to avoid an import cycle: services must not depend on
// its listeners.
type RegistrationListener interface {
	OnServiceRegistrationChanged(userId, serviceName string)
}

// RegistrationHook is notified by AddUser/RemoveUser on every actual
// registration change (not on no-op calls). nil is a valid, no-op default.
var RegistrationHook RegistrationListener

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
		if RegistrationHook != nil {
			go func() {
				defer logRecoveredPanic("a registration hook")
				RegistrationHook.OnServiceRegistrationChanged(userId, serviceName)
			}()
		}
	}
	return nil
}

// IsUserRegistered reports whether userId is currently in serviceName's
// rotation, i.e. has been added via AddUser (typically at credential connect
// time) and not since removed via RemoveUser.
func (s *Scheduler) IsUserRegistered(serviceName, userId string) bool {
	ss, ok := s.service(serviceName)
	if !ok {
		return false
	}
	ss.mu.Lock()
	defer ss.mu.Unlock()
	_, exists := ss.byUser[userId]
	return exists
}

// RegisteredServiceNames returns the names of every service integration
// registered via RegisterProvider.
func (s *Scheduler) RegisteredServiceNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.providers))
	for name := range s.providers {
		names = append(names, name)
	}
	return names
}

// RemoveUser removes a user from a service's rotation and discards their queued
// tasks. Cancelling the user's context wakes their paused tasks, which stop
// without writing; each dropped unpause entry is finished in its sync report.
func (s *Scheduler) RemoveUser(serviceName, userId string) error {
	ss, ok := s.service(serviceName)
	if !ok {
		return fmt.Errorf("service %q not registered", serviceName)
	}

	ss.mu.Lock()
	u, exists := ss.byUser[userId]
	if !exists {
		ss.mu.Unlock()
		return nil
	}
	delete(ss.byUser, userId)
	if i := slices.Index(ss.users, u); i >= 0 {
		ss.users = slices.Delete(ss.users, i, i+1)
	}
	ss.mu.Unlock()

	u.cancel()

	//u.mu alone: it can be held for a while by WithUserLock, and ss.mu must not wait on it
	u.mu.Lock()
	dropped := u.tasks
	u.tasks = nil
	u.mu.Unlock()

	for _, entry := range dropped {
		if entry.run != nil && entry.progress != nil && entry.progress.taskFinished(nil) {
			if err := publishReport(userId, entry.progress.report); err != nil {
				fmt.Printf("ERROR: Services: publishing sync report for user %s: %v\n", userId, err)
			}
		}
	}

	fmt.Printf("Services: Removed user [%s] from service: %s", userId, serviceName)
	if RegistrationHook != nil {
		go func() {
			defer logRecoveredPanic("a registration hook")
			RegistrationHook.OnServiceRegistrationChanged(userId, serviceName)
		}()
	}
	return nil
}

// Enqueue submits a task for a user on the named service. The user must
// already be registered (via AddUser, at connect time or restore-at-startup)
// — this deliberately does not create a rotation entry on demand, so that a
// task chain continuing after RemoveUser has run (e.g. mid-sync credential
// removal) fails here instead of silently resurrecting the user into
// rotation. Callers should treat a failure here as "stop chaining" and route
// it through their normal done()/error handling. The task's results go to the
// user's sync that is active now, even if a cancel and re-add follow.
func (s *Scheduler) Enqueue(serviceName, userId string, task Task) error {
	ss, ok := s.service(serviceName)
	if !ok {
		return fmt.Errorf("service %q not registered", serviceName)
	}

	ss.mu.Lock()
	u, exists := ss.byUser[userId]
	ss.mu.Unlock()
	if !exists {
		return fmt.Errorf("user %s is not registered for service %q", userId, serviceName)
	}

	var progress *syncProgress
	if active, ok := s.activeSyncs.Load(syncKey(serviceName, userId)); ok {
		progress = active.(*syncProgress)
	}

	u.mu.Lock()
	u.tasks = append(u.tasks, queuedTask{task: task, progress: progress})
	u.mu.Unlock()
	return nil
}

// WithUserLock runs fn while holding the same per-(service,user) lock that
// guards userId's task queue on serviceName. Services with their own
// per-user resource cache (e.g. an auth session) should use this to
// serialize building/storing that resource against a concurrent credential
// change, instead of introducing a separate lock — since only one sync ever
// runs per (service,user) at a time, the only real contention this creates
// is against exactly the credential-change path it's meant to guard against.
// Returns an error without calling fn if the user isn't currently registered.
func (s *Scheduler) WithUserLock(serviceName, userId string, fn func() error) error {
	ss, ok := s.service(serviceName)
	if !ok {
		return fmt.Errorf("service %q not registered", serviceName)
	}

	ss.mu.Lock()
	u, exists := ss.byUser[userId]
	ss.mu.Unlock()
	if !exists {
		return fmt.Errorf("user %s is not registered for service %q", userId, serviceName)
	}

	u.mu.Lock()
	defer u.mu.Unlock()
	return fn()
}

// RegisterProvider registers a service via its ServiceProvider interface.
func (s *Scheduler) RegisterProvider(p ServiceProvider) {
	s.RegisterService(p.Name(), p.Config())
	s.mu.Lock()
	s.providers[p.Name()] = p
	s.mu.Unlock()
}

// RestoreAllSchedules re-registers every user's rotation membership and
// periodic sync job for every registered provider, from what's stored in the
// database. Call once at startup after Start.
func (s *Scheduler) RestoreAllSchedules() {
	s.mu.RLock()
	ps := make([]ServiceProvider, 0, len(s.providers))
	for _, p := range s.providers {
		ps = append(ps, p)
	}
	s.mu.RUnlock()

	for _, p := range ps {
		docs, err := GetAllUsersWithService(p.Name())
		if err != nil {
			fmt.Printf("ERROR: Services: could not restore schedules for %s: %v\n", p.Name(), err)
			continue
		}
		for _, doc := range docs {
			_ = s.AddUser(p.Name(), doc.UserId)
			entry := doc.Services[p.Name()]
			if err := s.applySchedule(p, doc.UserId, entry.Sync); err != nil {
				fmt.Printf("ERROR: Services: failed to restore schedule for user: %s, service: %s: %v\n", doc.UserId, p.Name(), err)
			}
		}
		fmt.Printf("Services: restored schedules for %d user(s) on %s\n", len(docs), p.Name())
	}
}

// applySchedule (re)registers userId's periodic sync job for p's service from
// the given sync settings, using p.Sync as the job function. Centralizes what
// each service previously had to wire up itself, so a schedule can never end
// up pointing at the wrong sync function.
func (s *Scheduler) applySchedule(p ServiceProvider, userId string, sync ExternalApiSync) error {
	return s.SchedulePeriodic(p.Name(), userId, sync.SyncIntervalHours, sync.LastSynced, p.Sync)
}

// SyncUser triggers an on-demand sync for userId on the named service.
func (s *Scheduler) SyncUser(serviceName, userId string) error {
	p, ok := s.provider(serviceName)
	if !ok {
		return fmt.Errorf("service %q not registered", serviceName)
	}
	return s.runSync(serviceName, userId, "manual", p.Sync)
}

// runSync launches sync for userId on serviceName: it guards against a second
// concurrent run for the same service+user, stamps LastSynced, and logs the
// launch as kind ("manual" or "periodic"). sync owns the guard from here on —
// it must call the done callback it receives exactly once, on every return
// path, once the run has truly finished (see SyncFunc).
func (s *Scheduler) runSync(serviceName, userId, kind string, sync SyncFunc) error {
	progress := newSyncProgress(serviceName)
	if _, alreadyRunning := s.activeSyncs.LoadOrStore(syncKey(serviceName, userId), progress); alreadyRunning {
		return fmt.Errorf("%w: %s sync for user %s", ErrSyncInProgress, serviceName, userId)
	}
	done := func(err error) { s.completeSync(serviceName, userId, progress, err) }

	if err := SetServiceLastSynced(userId, serviceName, time.Now()); err != nil {
		fmt.Printf("ERROR: Services: failed to record last synced for user: %s, service: %s: %v", userId, serviceName, err)
	}
	fmt.Printf("Running  [%s] sync: User : %s  Service : %s\n", kind, userId, serviceName)

	return sync(userId, done)
}

// completeSync is a sync's done callback. It ends the report as completed, or
// failed when err is set (logged here, since the report keeps only its reason),
// then releases the guard only if it still holds this sync, so a stale sync's
// done can't end a newer one.
func (s *Scheduler) completeSync(serviceName, userId string, progress *syncProgress, err error) {
	outcome := notification.OutcomeCompleted
	if err != nil {
		outcome = notification.OutcomeFailed
		fmt.Printf("ERROR: Services: %s sync for user %s failed: %v\n", serviceName, userId, err)
	}
	endSync(userId, progress, outcome, err)
	s.activeSyncs.CompareAndDelete(syncKey(serviceName, userId), progress)
}

// cancelSync ends the user's active sync as cancelled without releasing its
// guard. Its report is published now, or once its executing task finishes.
func (s *Scheduler) cancelSync(serviceName, userId string) {
	if active, ok := s.activeSyncs.Load(syncKey(serviceName, userId)); ok {
		endSync(userId, active.(*syncProgress), notification.OutcomeCancelled, nil)
	}
}

// endSync ends progress's report and publishes it, unless one of its tasks is
// still executing or paused; the run loop, or RemoveUser for a paused task it
// drops, publishes it once that task finishes.
func endSync(userId string, progress *syncProgress, outcome notification.Outcome, err error) {
	if !progress.end(outcome, err) {
		return
	}
	if err := publishReport(userId, progress.report); err != nil {
		fmt.Printf("ERROR: Services: publishing sync report for user %s: %v\n", userId, err)
	}
}

// IsSyncing reports whether a sync for userId on serviceName is currently in progress.
func (s *Scheduler) IsSyncing(serviceName, userId string) bool {
	_, syncing := s.activeSyncs.Load(syncKey(serviceName, userId))
	return syncing
}

// clearActiveSync forcibly releases the active-sync guard for a user+service,
// e.g. when their credentials are removed mid-sync and no further completion
// callback should be expected.
func (s *Scheduler) clearActiveSync(serviceName, userId string) {
	s.activeSyncs.Delete(syncKey(serviceName, userId))
}

// registrationLock returns the mutex serializing AddService/RemoveService
// for the given service+user, creating it on first use.
func (s *Scheduler) registrationLock(serviceName, userId string) *sync.Mutex {
	lock, _ := s.registrationLocks.LoadOrStore(syncKey(serviceName, userId), &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// AddService validates creds against the service, then stores them and joins
// the user's rotation/periodic schedule. TestCredentials runs first and
// outside the registration lock, since it's a pure external check with no
// shared-state side effects and may be slow: no reason to make a concurrent
// RemoveService wait on it. This is the only path that should ever call
// AddServiceCredentials, so bad credentials can never be persisted or acted
// on by any other caller.
func (s *Scheduler) AddService(serviceName, userId string, creds ExternalApiKeys) error {
	if err := s.TestCredentials(serviceName, userId, creds); err != nil {
		return fmt.Errorf("%w: %v", ErrCredentialTestFailed, err)
	}

	lock := s.registrationLock(serviceName, userId)
	lock.Lock()
	defer lock.Unlock()

	if err := AddServiceCredentials(userId, serviceName, creds); err != nil {
		return err
	}
	_ = s.AddUser(serviceName, userId)
	s.fireCredentialHook(serviceName, userId, creds)
	return nil
}

// RemoveService cleans up a user's service. A running sync is cancelled first,
// before RemoveUser drops its queued tasks, so it reports as cancelled. The
// active-sync guard is cleared last, deliberately: clearing it before
// credentials are removed would let a new SyncUser call slip through while
// reading now-stale credentials. Takes the same registration lock as
// AddService so the two can't interleave.
func (s *Scheduler) RemoveService(serviceName, userId string) error {
	p, ok := s.provider(serviceName)
	if !ok {
		return fmt.Errorf("service %q not registered", serviceName)
	}

	lock := s.registrationLock(serviceName, userId)
	lock.Lock()
	defer lock.Unlock()

	s.cancelSync(serviceName, userId)
	p.OnCredentialsRemoved(userId)
	s.CancelPeriodic(serviceName, userId)
	_ = s.RemoveUser(serviceName, userId)
	err := DeleteServiceInfo(userId, serviceName)
	s.clearActiveSync(serviceName, userId)
	return err
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
	ctx, cancel := context.WithCancel(ss.baseCtx)
	u := &userEntry{userId: userId, ctx: ctx, cancel: cancel}
	ss.users = append(ss.users, u)
	ss.byUser[userId] = u
	return u, true
}

// pauseTask is the scheduler's pause function (see imageset.WithPause), run on
// the task's goroutine: it queues an unpause entry at the front of the user's
// queue, tells the run loop the task paused, and waits until that entry comes
// round. It never pauses once cancelled or after maxTaskPauses (errTaskGaveUp).
func (run *taskRun) pauseTask(_ context.Context) error {

	if err := run.ctx.Err(); err != nil {
		return err
	}

	if run.pauses >= maxTaskPauses {
		return errTaskGaveUp
	}
	run.pauses++

	if !run.scheduler.queueAtFront(run.user, queuedTask{progress: run.progress, run: run}) {
		return context.Canceled //the user was removed
	}
	run.events <- taskEvent{paused: true}

	select {
	case <-run.resume:
		return nil
	case <-run.ctx.Done():
		return run.ctx.Err()
	}
}

// startTask runs entry's task on its own goroutine, with a context that carries
// pauseTask and is cancelled with user. The goroutine always sends exactly one
// final event; a panic is logged and becomes a failed item instead of crashing
// the backend.
func (ss *serviceScheduler) startTask(user *userEntry, entry queuedTask) *taskRun {

	run := &taskRun{
		scheduler: ss,
		user:      user,
		ctx:       user.ctx,
		progress:  entry.progress,
		resume:    make(chan struct{}),
		events:    make(chan taskEvent, 1),
	}
	ctx := imageset.WithPause(run.ctx, run.pauseTask)

	go func() {

		var final taskEvent
		defer func() {

			if r := recover(); r != nil {
				log.Printf("------ Warning: a %s task for user [%s] panicked: %v ------\n%s", ss.name, user.userId, r, debug.Stack())
				err := fmt.Errorf("task panicked: %v", r)
				final = taskEvent{results: []notification.ItemResult{notification.FailedItem(user.userId, ss.name, bson.NilObjectID, err)}, err: err}
			}
			run.events <- final
		}()
		final.results, final.err = entry.task(ctx)
	}()
	return run
}

// queueAtFront puts entry at the front of user's queue. It returns false when
// user has since been removed or replaced by a newer entry for the same id.
func (ss *serviceScheduler) queueAtFront(user *userEntry, entry queuedTask) bool {

	ss.mu.Lock()
	defer ss.mu.Unlock()
	if ss.byUser[user.userId] != user {
		return false
	}

	user.mu.Lock()
	user.tasks = append([]queuedTask{entry}, user.tasks...)
	user.mu.Unlock()
	return true
}

// endTurn moves user to the back of the rotation and resets its turn count.
func (ss *serviceScheduler) endTurn(user *userEntry) {

	ss.mu.Lock()
	defer ss.mu.Unlock()
	i := slices.Index(ss.users, user)
	if i < 0 {
		return
	}
	ss.users = append(slices.Delete(ss.users, i, i+1), user)

	user.mu.Lock()
	user.queriesDone = 0
	user.mu.Unlock()
}

// awaitFinalEvent returns run's final event, skipping any paused event sent
// before it was cancelled.
func (run *taskRun) awaitFinalEvent() taskEvent {
	for {
		if event := <-run.events; !event.paused {
			return event
		}
	}
}

// run is the service's loop: one entry at a time, it waits out Delay, starts the
// task or resumes a paused one, and waits until it pauses or finishes. A pause
// ends the user's turn; a finished task is recorded in its sync report.
func (ss *serviceScheduler) run() {
	for {
		select {
		case <-ss.stop:
			return
		default:
		}

		next, user, ok := ss.nextTask()
		if !ok {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		if wait := ss.config.Delay - time.Since(ss.lastRun); wait > 0 {
			time.Sleep(wait)
		}

		var event taskEvent
		if run := next.run; run == nil {

			if next.progress != nil {
				next.progress.taskStarted()
			}
			event = <-ss.startTask(user, next).events

		} else {

			select {
			case run.resume <- struct{}{}:
				event = <-run.events
			case event = <-run.events:
			case <-run.ctx.Done():
				event = run.awaitFinalEvent()
			}
		}
		ss.lastRun = time.Now()

		if event.paused {
			ss.endTurn(user)
			continue
		}

		if event.err != nil {
			fmt.Printf("ERROR: Services: task failed for user %s: %v\n", user.userId, event.err)
		}

		if next.progress != nil && next.progress.taskFinished(event.results) {
			if err := publishReport(user.userId, next.progress.report); err != nil {
				fmt.Printf("ERROR: Services: publishing sync report for user %s: %v\n", user.userId, err)
			}
		}
	}
}

// nextTask picks the next task from the front user.
// When a user exhausts their QueriesPerTurn quota, they are rotated to the back.
// Users with no pending tasks are skipped (and rotated past).
func (ss *serviceScheduler) nextTask() (queuedTask, *userEntry, bool) {
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

		next := front.tasks[0]
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

		return next, front, true
	}

	return queuedTask{}, nil, false
}

// fireCredentialHook runs the registered provider's service-specific
// OnCredentialsUpdated (e.g. invalidating a cached session), then
// centrally reschedules the periodic sync from the newly stored settings.
func (s *Scheduler) fireCredentialHook(serviceName, userId string, creds ExternalApiKeys) {
	p, ok := s.provider(serviceName)
	if !ok {
		return
	}
	p.OnCredentialsUpdated(userId, creds)
	s.fireSyncSettingsHook(serviceName, userId)
}

// fireSyncSettingsHook reloads userId's stored sync settings for serviceName
// and reschedules their periodic job accordingly.
func (s *Scheduler) fireSyncSettingsHook(serviceName, userId string) {
	p, ok := s.provider(serviceName)
	if !ok {
		return
	}
	sync, err := GetServiceSync(userId, serviceName)
	if err != nil {
		fmt.Printf("ERROR: Services: failed to load sync settings for user: %s, service: %s: %v\n", userId, serviceName, err)
		return
	}
	if err := s.applySchedule(p, userId, *sync); err != nil {
		fmt.Printf("ERROR: Services: failed to apply schedule for user: %s, service: %s: %v\n", userId, serviceName, err)
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
