package docker

import (
	"context"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// --- mock Docker client ---------------------------------------------------

type mockDockerClient struct {
	messages chan events.Message
	errs     chan error
}

func newMockClient() *mockDockerClient {
	return &mockDockerClient{
		messages: make(chan events.Message, 10),
		errs:     make(chan error, 1),
	}
}

func (m *mockDockerClient) Events(_ context.Context, _ events.ListOptions) (<-chan events.Message, <-chan error) {
	return m.messages, m.errs
}

func (m *mockDockerClient) Ping(_ context.Context) (types.Ping, error) {
	return types.Ping{}, nil
}

func (m *mockDockerClient) Close() error { return nil }

// --- mock repos -----------------------------------------------------------

type mockAppRepo struct {
	apps []models.App
}

func (r *mockAppRepo) List(_ context.Context) ([]models.App, error)                          { return r.apps, nil }
func (r *mockAppRepo) ListByHost(_ context.Context, _ string) ([]models.App, error)          { return nil, nil }
func (r *mockAppRepo) Create(_ context.Context, _ *models.App) error                         { return nil }
func (r *mockAppRepo) Get(_ context.Context, _ string) (*models.App, error)                  { return nil, repo.ErrNotFound }
func (r *mockAppRepo) GetByToken(_ context.Context, _ string) (*models.App, error)           { return nil, repo.ErrNotFound }
func (r *mockAppRepo) Update(_ context.Context, _ *models.App) error                         { return nil }
func (r *mockAppRepo) Delete(_ context.Context, _ string) error                              { return nil }
func (r *mockAppRepo) UpdateToken(_ context.Context, _, _ string) error                      { return nil }
func (r *mockAppRepo) SetDockerEngineID(_ context.Context, _, _ string) error               { return nil }
func (r *mockAppRepo) SetHostComponentID(_ context.Context, _ string, _ *string) error      { return nil }

type mockEventRepo struct {
	created []*models.Event
}

func (r *mockEventRepo) Create(_ context.Context, ev *models.Event) error {
	r.created = append(r.created, ev)
	return nil
}

func (r *mockEventRepo) List(_ context.Context, _ repo.ListFilter) ([]models.Event, int, error) {
	return nil, 0, nil
}
func (r *mockEventRepo) Get(_ context.Context, _ string) (*models.Event, error) {
	return nil, repo.ErrNotFound
}
func (r *mockEventRepo) CountForCategory(_ context.Context, _ repo.CategoryFilter) (int, error) {
	return 0, nil
}
func (r *mockEventRepo) SparklineBuckets(_ context.Context, _ repo.CategoryFilter, _ time.Time, _ time.Duration) ([7]int, error) {
	return [7]int{}, nil
}
func (r *mockEventRepo) LatestPerApp(_ context.Context, _ []string) (map[string]*models.Event, error) {
	return nil, nil
}
func (r *mockEventRepo) DeleteBySeverityBefore(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, nil
}
func (r *mockEventRepo) GroupByTypeAndSeverity(_ context.Context, _ string, _, _ time.Time) ([]repo.EventTypeCount, error) {
	return nil, nil
}
func (r *mockEventRepo) MetricsForApp(_ context.Context, _ string, _, _ time.Time) (repo.EventMetrics, error) {
	return repo.EventMetrics{}, nil
}
func (r *mockEventRepo) CountPerApp(_ context.Context, _ time.Time) ([]repo.AppEventCount, error) {
	return nil, nil
}
func (r *mockEventRepo) Timeseries(_ context.Context, _, _ time.Time, _, _, _ string) ([]repo.TimeseriesBucket, error) {
	return nil, nil
}

// --- helpers -------------------------------------------------------------

func newTestWatcher(appRepo repo.AppRepo, eventRepo repo.EventRepo, dc dockerAPI) *Watcher {
	store := repo.NewStore(appRepo, eventRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	return &Watcher{store: store, client: dc}
}

func containerEvent(action events.Action, name string, extraAttrs map[string]string) events.Message {
	attrs := map[string]string{"name": name}
	for k, v := range extraAttrs {
		attrs[k] = v
	}
	return events.Message{
		Type:   events.ContainerEventType,
		Action: action,
		Actor:  events.Actor{ID: "abc123", Attributes: attrs},
	}
}

// --- tests ---------------------------------------------------------------

func TestSeverityAndText(t *testing.T) {
	tests := []struct {
		action      string
		exitCode    string
		wantSev     string
		wantTextPfx string
	}{
		{"start", "", "info", "Container started"},
		{"stop", "", "warn", "Container stopped"},
		{"die", "0", "info", "Container exited cleanly"},
		{"die", "1", "error", "Container crashed"},
		{"die", "137", "error", "Container crashed"},
		{"restart", "", "warn", "Container restarted"},
		{"kill", "", "warn", "Container killed"},
	}
	for _, tc := range tests {
		sev, text := severityAndText(tc.action, "myapp", tc.exitCode)
		if sev != tc.wantSev {
			t.Errorf("action=%s exit=%s: got severity %q, want %q", tc.action, tc.exitCode, sev, tc.wantSev)
		}
		if len(text) < len(tc.wantTextPfx) || text[:len(tc.wantTextPfx)] != tc.wantTextPfx {
			t.Errorf("action=%s exit=%s: got text %q, want prefix %q", tc.action, tc.exitCode, text, tc.wantTextPfx)
		}
	}
}

func TestHandleEvent_MatchingApp(t *testing.T) {
	apps := &mockAppRepo{apps: []models.App{{ID: "app-1", Name: "myapp"}}}
	evRepo := &mockEventRepo{}
	dc := newMockClient()
	w := newTestWatcher(apps, evRepo, dc)

	msg := containerEvent("start", "myapp", nil)
	if err := w.handleEvent(context.Background(), msg); err != nil {
		t.Fatalf("handleEvent: %v", err)
	}

	if len(evRepo.created) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evRepo.created))
	}
	ev := evRepo.created[0]
	if ev.AppID != "app-1" {
		t.Errorf("AppID: got %q, want %q", ev.AppID, "app-1")
	}
	if ev.Severity != "info" {
		t.Errorf("Severity: got %q, want %q", ev.Severity, "info")
	}
}

func TestHandleEvent_NoMatchingApp_NullAppID(t *testing.T) {
	apps := &mockAppRepo{apps: []models.App{}}
	evRepo := &mockEventRepo{}
	dc := newMockClient()
	w := newTestWatcher(apps, evRepo, dc)

	msg := containerEvent("stop", "unknown-container", nil)
	if err := w.handleEvent(context.Background(), msg); err != nil {
		t.Fatalf("handleEvent: %v", err)
	}

	if len(evRepo.created) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evRepo.created))
	}
	ev := evRepo.created[0]
	if ev.AppID != "" {
		t.Errorf("AppID should be empty (null) for unmatched container, got %q", ev.AppID)
	}
	if ev.Severity != "warn" {
		t.Errorf("Severity: got %q, want %q", ev.Severity, "warn")
	}
}

func TestHandleEvent_CaseInsensitiveMatch(t *testing.T) {
	apps := &mockAppRepo{apps: []models.App{{ID: "app-2", Name: "Sonarr"}}}
	evRepo := &mockEventRepo{}
	dc := newMockClient()
	w := newTestWatcher(apps, evRepo, dc)

	// Docker reports the container name in lowercase
	msg := containerEvent("restart", "sonarr", nil)
	if err := w.handleEvent(context.Background(), msg); err != nil {
		t.Fatalf("handleEvent: %v", err)
	}

	if evRepo.created[0].AppID != "app-2" {
		t.Errorf("expected case-insensitive match, AppID=%q", evRepo.created[0].AppID)
	}
}

func TestHandleEvent_DieNonZeroExit(t *testing.T) {
	apps := &mockAppRepo{}
	evRepo := &mockEventRepo{}
	dc := newMockClient()
	w := newTestWatcher(apps, evRepo, dc)

	msg := containerEvent("die", "crasher", map[string]string{"exitCode": "137"})
	if err := w.handleEvent(context.Background(), msg); err != nil {
		t.Fatalf("handleEvent: %v", err)
	}

	ev := evRepo.created[0]
	if ev.Severity != "error" {
		t.Errorf("Severity: got %q, want error", ev.Severity)
	}
}

func TestHandleEvent_DieZeroExit(t *testing.T) {
	apps := &mockAppRepo{}
	evRepo := &mockEventRepo{}
	dc := newMockClient()
	w := newTestWatcher(apps, evRepo, dc)

	msg := containerEvent("die", "graceful", map[string]string{"exitCode": "0"})
	if err := w.handleEvent(context.Background(), msg); err != nil {
		t.Fatalf("handleEvent: %v", err)
	}

	ev := evRepo.created[0]
	if ev.Severity != "info" {
		t.Errorf("Severity: got %q, want info", ev.Severity)
	}
}

func TestHandleEvent_NonContainerEventsIgnored(t *testing.T) {
	apps := &mockAppRepo{}
	evRepo := &mockEventRepo{}
	dc := newMockClient()
	w := newTestWatcher(apps, evRepo, dc)

	msg := events.Message{
		Type:   events.ImageEventType,
		Action: "pull",
		Actor:  events.Actor{Attributes: map[string]string{"name": "nginx"}},
	}
	if err := w.handleEvent(context.Background(), msg); err != nil {
		t.Fatalf("handleEvent: %v", err)
	}
	if len(evRepo.created) != 0 {
		t.Errorf("expected no events for non-container type, got %d", len(evRepo.created))
	}
}

func TestStream_ProcessesEvents(t *testing.T) {
	apps := &mockAppRepo{}
	evRepo := &mockEventRepo{}
	dc := newMockClient()
	w := newTestWatcher(apps, evRepo, dc)

	ctx, cancel := context.WithCancel(context.Background())

	// Send a start event then cancel
	dc.messages <- containerEvent("start", "nginx", nil)

	go func() {
		// Give stream goroutine time to process the message before cancelling
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_ = w.stream(ctx) // returns ctx.Err()

	if len(evRepo.created) == 0 {
		t.Error("expected at least one event to be created")
	}
}

func TestStream_ReturnsOnError(t *testing.T) {
	apps := &mockAppRepo{}
	evRepo := &mockEventRepo{}
	dc := newMockClient()
	w := newTestWatcher(apps, evRepo, dc)

	ctx := context.Background()

	// Send an error to simulate daemon disconnect
	dc.errs <- errDaemonDisconnect

	err := w.stream(ctx)
	if err == nil {
		t.Error("expected stream to return an error on daemon disconnect")
	}
}

// sentinel error used in tests
var errDaemonDisconnect = &daemonError{"daemon disconnected"}

type daemonError struct{ msg string }

func (e *daemonError) Error() string { return e.msg }
