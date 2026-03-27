package monitor

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// URLChecker is a stub for the URL check runner, implemented in T-14.
type URLChecker struct{ store *repo.Store }

// SSLChecker is a stub for the SSL check runner, implemented in T-15.
type SSLChecker struct{ store *repo.Store }

// Scheduler loads monitor checks from the database and runs each one on its
// configured interval. Every check runs in its own goroutine, so a slow or
// blocked check cannot delay others. The check list is refreshed every 5
// minutes to pick up newly added or disabled checks without a restart.
type Scheduler struct {
	store *repo.Store
	ping  *PingChecker
	url   *URLChecker
	ssl   *SSLChecker

	mu     sync.Mutex
	active map[string]context.CancelFunc // check ID → cancel for that goroutine
}

// NewScheduler returns a Scheduler wired to store.
func NewScheduler(store *repo.Store) *Scheduler {
	return &Scheduler{
		store:  store,
		ping:   NewPingChecker(store),
		url:    &URLChecker{store: store},
		ssl:    &SSLChecker{store: store},
		active: make(map[string]context.CancelFunc),
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

// dispatch runs the appropriate checker for one cycle of check.
func (s *Scheduler) dispatch(ctx context.Context, check *models.MonitorCheck) {
	var err error
	switch check.Type {
	case "ping":
		err = s.ping.Run(ctx, check)
	case "url":
		// T-14: URL checker not yet implemented.
		log.Printf("monitor scheduler: url check %q skipped — url checker not yet implemented (T-14)", check.Name)
	case "ssl":
		// T-15: SSL checker not yet implemented.
		log.Printf("monitor scheduler: ssl check %q skipped — ssl checker not yet implemented (T-15)", check.Name)
	default:
		log.Printf("monitor scheduler: unknown check type %q for check %q", check.Type, check.Name)
	}
	if err != nil {
		log.Printf("monitor scheduler: check %q (%s) failed: %v", check.Name, check.Type, err)
	}
}
