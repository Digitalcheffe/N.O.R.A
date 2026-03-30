package monitor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// --- mock repos ---

type mockCheckRepo struct {
	repo.CheckRepo
	updateStatusCalls []updateStatusCall
	updateErr         error
}

type updateStatusCall struct {
	id      string
	status  string
	details string
}

func (m *mockCheckRepo) UpdateStatus(_ context.Context, id, status, details string, _ time.Time) error {
	m.updateStatusCalls = append(m.updateStatusCalls, updateStatusCall{id: id, status: status, details: details})
	return m.updateErr
}

type mockEventRepo struct {
	repo.EventRepo
	created  []*models.Event
	createErr error
}

func (m *mockEventRepo) Create(_ context.Context, event *models.Event) error {
	m.created = append(m.created, event)
	return m.createErr
}

func newTestStore(checks *mockCheckRepo, events *mockEventRepo) *repo.Store {
	return &repo.Store{Checks: checks, Events: events}
}

// --- helpers ---

func alwaysUp(_ context.Context, _ string) Result {
	return Result{Status: "up", Details: []byte(`{"latency_ms":10}`), CheckedAt: time.Now()}
}

func alwaysDown(_ context.Context, _ string) Result {
	return Result{Status: "down", Details: []byte(`{}`), CheckedAt: time.Now()}
}

func makeCheck(lastStatus, appID string) *models.MonitorCheck {
	return &models.MonitorCheck{
		ID:         "check-1",
		AppID:      appID,
		Name:       "My Host",
		Type:       "ping",
		Target:     "192.168.1.1",
		LastStatus: lastStatus,
	}
}

// --- tests ---

// TestPingChecker_HappyPath verifies that a successful ping marks the check "up".
func TestPingChecker_HappyPath(t *testing.T) {
	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := &PingChecker{store: newTestStore(checks, events), pinger: alwaysUp}

	check := makeCheck("up", "")
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(checks.updateStatusCalls) != 1 {
		t.Fatalf("expected 1 UpdateStatus call, got %d", len(checks.updateStatusCalls))
	}
	if checks.updateStatusCalls[0].status != "up" {
		t.Errorf("expected status=up, got %s", checks.updateStatusCalls[0].status)
	}
	if len(events.created) != 0 {
		t.Errorf("expected no events, got %d", len(events.created))
	}
}

// TestPingChecker_AllFailsDown verifies that 3/3 ping failures mark the check "down".
func TestPingChecker_AllFailsDown(t *testing.T) {
	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := &PingChecker{store: newTestStore(checks, events), pinger: alwaysDown}

	check := makeCheck("up", "app-1") // previous status "up" so we get a state change
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(checks.updateStatusCalls) != 1 {
		t.Fatalf("expected 1 UpdateStatus call, got %d", len(checks.updateStatusCalls))
	}
	if checks.updateStatusCalls[0].status != "down" {
		t.Errorf("expected status=down, got %s", checks.updateStatusCalls[0].status)
	}

	// An error event should have been created because status changed up→down.
	if len(events.created) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events.created))
	}
	ev := events.created[0]
	if ev.Level != "error" {
		t.Errorf("expected level=error, got %s", ev.Level)
	}
	if ev.SourceType != "monitor_check" {
		t.Errorf("expected source_type=monitor_check, got %s", ev.SourceType)
	}
}

// TestPingChecker_Recovery verifies that a down→up transition creates an info event.
func TestPingChecker_Recovery(t *testing.T) {
	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := &PingChecker{store: newTestStore(checks, events), pinger: alwaysUp}

	check := makeCheck("down", "app-1") // was down, now pings succeed
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events.created) != 1 {
		t.Fatalf("expected 1 recovery event, got %d", len(events.created))
	}
	ev := events.created[0]
	if ev.Level != "info" {
		t.Errorf("expected level=info, got %s", ev.Level)
	}
}

// TestPingChecker_EventWithoutApp verifies that a status-change event IS created
// for checks not linked to an app (app_id nullable; events queryable by check_id).
func TestPingChecker_EventWithoutApp(t *testing.T) {
	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := &PingChecker{store: newTestStore(checks, events), pinger: alwaysDown}

	check := makeCheck("up", "") // AppID empty — status changes up→down
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events.created) != 1 {
		t.Errorf("expected 1 event for check without app on status change, got %d", len(events.created))
	}
	if events.created[0].SourceType != "monitor_check" {
		t.Errorf("expected source_type=monitor_check on event, got %s", events.created[0].SourceType)
	}
}

// TestPingChecker_NoEventOnFirstRun verifies that the first execution of a
// check (LastStatus == "") does not emit an event even if the result is down.
func TestPingChecker_NoEventOnFirstRun(t *testing.T) {
	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := &PingChecker{store: newTestStore(checks, events), pinger: alwaysDown}

	check := makeCheck("", "app-1") // no previous status
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events.created) != 0 {
		t.Errorf("expected no event on first run, got %d", len(events.created))
	}
}

// TestPingChecker_UpdateStatusError verifies that an UpdateStatus failure is
// surfaced as an error return.
func TestPingChecker_UpdateStatusError(t *testing.T) {
	checks := &mockCheckRepo{updateErr: errors.New("db error")}
	events := &mockEventRepo{}
	checker := &PingChecker{store: newTestStore(checks, events), pinger: alwaysUp}

	check := makeCheck("up", "")
	if err := checker.Run(context.Background(), check); err == nil {
		t.Fatal("expected error from UpdateStatus, got nil")
	}
}
