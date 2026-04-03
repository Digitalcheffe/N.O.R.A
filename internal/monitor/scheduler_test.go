package monitor

import (
	"context"
	"testing"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// mockListCheckRepo is a CheckRepo that returns a fixed list from List.
type mockListCheckRepo struct {
	repo.CheckRepo
	checks []models.MonitorCheck
}

func (m *mockListCheckRepo) List(_ context.Context) ([]models.MonitorCheck, error) {
	return m.checks, nil
}

func (m *mockListCheckRepo) UpdateStatus(_ context.Context, _, _, _ string, _ time.Time) error {
	return nil
}

// TestScheduler_StartStop verifies that the scheduler starts cleanly with an
// empty check list and shuts down when the context is cancelled, without
// blocking or leaking goroutines.
func TestScheduler_StartStop(t *testing.T) {
	store := &repo.Store{
		Checks: &mockListCheckRepo{checks: []models.MonitorCheck{}},
		Events: &mockEventRepo{},
	}

	sched := NewScheduler(store)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		sched.Start(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// clean shutdown
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not shut down within 2 seconds after context cancel")
	}
}

// TestScheduler_StartsCheckGoroutine verifies that an enabled check causes a
// goroutine to be registered in the active map after syncChecks.
func TestScheduler_StartsCheckGoroutine(t *testing.T) {
	check := models.MonitorCheck{
		ID:           "chk-1",
		Name:         "Test Ping",
		Type:         "ping",
		Target:       "127.0.0.1",
		IntervalSecs: 300,
		Enabled:      true,
	}

	store := &repo.Store{
		Checks: &mockListCheckRepo{checks: []models.MonitorCheck{check}},
		Events: &mockEventRepo{},
	}

	sched := NewScheduler(store)
	// Override the pinger so the goroutine never makes real network calls.
	sched.ping.pinger = alwaysUp

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sched.syncChecks(ctx); err != nil {
		t.Fatalf("syncChecks: %v", err)
	}

	sched.mu.Lock()
	n := len(sched.active)
	sched.mu.Unlock()

	if n != 1 {
		t.Errorf("expected 1 active goroutine after sync, got %d", n)
	}
}

// TestScheduler_TriggerSync_CancelsGoroutine verifies that calling TriggerSync
// after a check is disabled causes its goroutine to be cancelled without
// waiting for the 5-minute periodic reload. This is the core of the fix for
// "pause silently no-ops on non-URL checks".
func TestScheduler_TriggerSync_CancelsGoroutine(t *testing.T) {
	check := models.MonitorCheck{
		ID:           "chk-trigger",
		Name:         "Ping Target",
		Type:         "ping",
		Target:       "127.0.0.1",
		IntervalSecs: 300,
		Enabled:      true, // starts enabled
	}

	// mr is a mutable repo — the test swaps the check list between syncs to
	// simulate the API writing enabled=false to the database.
	mr := &mockListCheckRepo{checks: []models.MonitorCheck{check}}

	store := &repo.Store{
		Checks: mr,
		Events: &mockEventRepo{},
	}

	sched := NewScheduler(store)
	sched.ping.pinger = alwaysUp

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the scheduler in the background.
	go sched.Start(ctx)

	// Wait for the initial sync to register the goroutine.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		sched.mu.Lock()
		n := len(sched.active)
		sched.mu.Unlock()
		if n == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	sched.mu.Lock()
	if len(sched.active) != 1 {
		sched.mu.Unlock()
		t.Fatalf("expected 1 active goroutine before disable, got %d", len(sched.active))
	}
	sched.mu.Unlock()

	// Simulate the API disabling the check: update the list the repo returns,
	// then call TriggerSync — the goroutine should be cancelled promptly.
	disabled := check
	disabled.Enabled = false
	mr.checks = []models.MonitorCheck{disabled}
	sched.TriggerSync()

	// The goroutine should be gone well within 1 second — not the 5-minute wait.
	deadline = time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		sched.mu.Lock()
		n := len(sched.active)
		sched.mu.Unlock()
		if n == 0 {
			return // pass
		}
		time.Sleep(10 * time.Millisecond)
	}

	sched.mu.Lock()
	remaining := len(sched.active)
	sched.mu.Unlock()
	if remaining != 0 {
		t.Errorf("expected 0 active goroutines after TriggerSync disable, got %d", remaining)
	}
}

// TestScheduler_TriggerSync_NonBlocking verifies that TriggerSync never blocks
// even when called multiple times rapidly without the scheduler draining the channel.
func TestScheduler_TriggerSync_NonBlocking(t *testing.T) {
	store := &repo.Store{
		Checks: &mockListCheckRepo{checks: []models.MonitorCheck{}},
		Events: &mockEventRepo{},
	}
	sched := NewScheduler(store)

	// Call many times in a tight loop — must not block or panic.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			sched.TriggerSync()
		}
		close(done)
	}()

	select {
	case <-done:
		// pass
	case <-time.After(1 * time.Second):
		t.Fatal("TriggerSync blocked")
	}
}

// TestScheduler_DisabledCheckNotStarted verifies that a disabled check does not
// get a goroutine in the active map.
func TestScheduler_DisabledCheckNotStarted(t *testing.T) {
	check := models.MonitorCheck{
		ID:           "chk-disabled",
		Name:         "Disabled",
		Type:         "ping",
		Target:       "192.168.0.1",
		IntervalSecs: 300,
		Enabled:      false, // disabled
	}

	store := &repo.Store{
		Checks: &mockListCheckRepo{checks: []models.MonitorCheck{check}},
		Events: &mockEventRepo{},
	}

	sched := NewScheduler(store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sched.syncChecks(ctx); err != nil {
		t.Fatalf("syncChecks: %v", err)
	}

	sched.mu.Lock()
	n := len(sched.active)
	sched.mu.Unlock()

	if n != 0 {
		t.Errorf("expected 0 active goroutines for disabled check, got %d", n)
	}
}
