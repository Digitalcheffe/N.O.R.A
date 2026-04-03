package monitor

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// Scheduler loads monitor checks from the database and runs each one on its
// configured interval. Every check runs in its own goroutine, so a slow or
// blocked check cannot delay others. The check list is refreshed every 5
// minutes to pick up newly added or disabled checks without a restart.
//
// Callers can also trigger an immediate reload via TriggerSync — used by the
// API handler so a disabled check's goroutine is cancelled within milliseconds
// rather than waiting up to 5 minutes for the periodic poll.
type Scheduler struct {
	store *repo.Store
	ping  *PingChecker
	url   *URLChecker
	ssl   *SSLChecker
	dns   *DNSChecker

	mu     sync.Mutex
	active map[string]context.CancelFunc // check ID → cancel for that goroutine

	// syncCh is a buffered channel used by TriggerSync to request an immediate
	// syncChecks without blocking the caller.
	syncCh chan struct{}
}

// NewScheduler returns a Scheduler wired to store.
func NewScheduler(store *repo.Store) *Scheduler {
	return &Scheduler{
		store:  store,
		ping:   NewPingChecker(store),
		url:    NewURLChecker(store),
		ssl:    NewSSLChecker(store),
		dns:    NewDNSChecker(store),
		active: make(map[string]context.CancelFunc),
		syncCh: make(chan struct{}, 1),
	}
}

// TriggerSync requests an immediate syncChecks. It is non-blocking: if a sync
// is already queued the call is a no-op (the buffered channel absorbs the
// signal without blocking). Safe to call from any goroutine.
func (s *Scheduler) TriggerSync() {
	select {
	case s.syncCh <- struct{}{}:
	default:
	}
}

// Start performs an initial sync then loops, re-syncing every 5 minutes.
// It blocks until ctx is cancelled, at which point all check goroutines are
// stopped before Start returns.
func (s *Scheduler) Start(ctx context.Context) {
	log.Printf("monitor scheduler: starting")

	if err := s.syncChecks(ctx); err != nil {
		log.Printf("monitor scheduler: initial load: %v", err)
	}

	reloadTicker := time.NewTicker(5 * time.Minute)
	defer reloadTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("monitor scheduler: context cancelled — shutting down")
			s.cancelAll()
			return
		case <-s.syncCh:
			if err := s.syncChecks(ctx); err != nil {
				log.Printf("monitor scheduler: triggered sync: %v", err)
			}
		case <-reloadTicker.C:
			if err := s.syncChecks(ctx); err != nil {
				log.Printf("monitor scheduler: reload: %v", err)
			}
		}
	}
}

// syncChecks reconciles the set of running goroutines against the current
// database state. New enabled checks are started; goroutines for checks that
// have been disabled or deleted are cancelled.
func (s *Scheduler) syncChecks(ctx context.Context) error {
	checks, err := s.store.Checks.List(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Build the set of currently-enabled check IDs.
	enabled := make(map[string]struct{}, len(checks))
	for i := range checks {
		c := &checks[i]
		if !c.Enabled {
			continue
		}
		enabled[c.ID] = struct{}{}

		if _, running := s.active[c.ID]; running {
			continue // already running — do not restart
		}

		checkCtx, cancel := context.WithCancel(ctx)
		s.active[c.ID] = cancel
		go s.runCheckLoop(checkCtx, c)
	}

	// Cancel goroutines for checks that are no longer enabled or have been deleted.
	for id, cancel := range s.active {
		if _, ok := enabled[id]; !ok {
			cancel()
			delete(s.active, id)
		}
	}

	log.Printf("monitor scheduler: %d checks active", len(s.active))
	return nil
}

// cancelAll cancels every running check goroutine and clears the active map.
func (s *Scheduler) cancelAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, cancel := range s.active {
		cancel()
		delete(s.active, id)
	}
}

// runCheckLoop ticks at check.IntervalSecs and dispatches to the appropriate
// checker on every tick. The loop exits when ctx is cancelled.
func (s *Scheduler) runCheckLoop(ctx context.Context, check *models.MonitorCheck) {
	interval := time.Duration(check.IntervalSecs) * time.Second
	if interval < 30*time.Second {
		interval = 30 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("monitor scheduler: started %s check %q target=%s interval=%s",
		check.Type, check.Name, check.Target, interval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.dispatch(ctx, check)
		}
	}
}

// RunAllByType runs every enabled monitor check of the given type immediately.
// Errors per check are logged by dispatch but do not abort the remaining checks.
func (s *Scheduler) RunAllByType(ctx context.Context, checkType string) error {
	checks, err := s.store.Checks.List(ctx)
	if err != nil {
		return err
	}
	for i := range checks {
		c := &checks[i]
		if !c.Enabled || c.Type != checkType {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s.dispatch(ctx, c)
	}
	return nil
}

// dispatch runs the appropriate checker for one cycle of check.
func (s *Scheduler) dispatch(ctx context.Context, check *models.MonitorCheck) {
	var err error
	switch check.Type {
	case "ping":
		err = s.ping.Run(ctx, check)
	case "url":
		err = s.url.Run(ctx, check)
	case "ssl":
		err = s.ssl.Run(ctx, check)
	case "dns":
		err = s.dns.Run(ctx, check)
	default:
		log.Printf("monitor scheduler: unknown check type %q for check %q", check.Type, check.Name)
	}
	if err != nil {
		log.Printf("monitor scheduler: check %q (%s) failed: %v", check.Name, check.Type, err)
	}
}
