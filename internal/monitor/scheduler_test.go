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
