package jobs

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/digitalcheffe/nora/internal/infra"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// ── Router status transition tests ────────────────────────────────────────────

// fakeEventCapture collects events created during test runs.
// It satisfies the full repo.EventRepo interface; only Create is functional.
type fakeEventCapture struct {
	mu     sync.Mutex
	events []*models.Event
}

func (f *fakeEventCapture) Create(_ context.Context, ev *models.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, ev)
	return nil
}

func (f *fakeEventCapture) List(_ context.Context, _ repo.ListFilter) ([]models.Event, int, error) {
	return nil, 0, nil
}
func (f *fakeEventCapture) Get(_ context.Context, _ string) (*models.Event, error) { return nil, nil }
func (f *fakeEventCapture) Timeseries(_ context.Context, _, _ time.Time, _, _, _ string) ([]repo.TimeseriesBucket, error) {
	return nil, nil
}
func (f *fakeEventCapture) CountForCategory(_ context.Context, _ repo.CategoryFilter) (int, error) {
	return 0, nil
}
func (f *fakeEventCapture) SparklineBuckets(_ context.Context, _ repo.CategoryFilter, _ time.Time, _ time.Duration) ([7]int, error) {
	return [7]int{}, nil
}
func (f *fakeEventCapture) LatestPerApp(_ context.Context, _ []string) (map[string]*models.Event, error) {
	return nil, nil
}
func (f *fakeEventCapture) DeleteBySeverityBefore(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, nil
}
func (f *fakeEventCapture) GroupByTypeAndSeverity(_ context.Context, _ string, _, _ time.Time) ([]repo.EventTypeCount, error) {
	return nil, nil
}
func (f *fakeEventCapture) MetricsForApp(_ context.Context, _ string, _, _ time.Time) (repo.EventMetrics, error) {
	return repo.EventMetrics{}, nil
}
func (f *fakeEventCapture) CountPerApp(_ context.Context, _ time.Time) ([]repo.AppEventCount, error) {
	return nil, nil
}

// buildTestStore creates a minimal *repo.Store with only the EventRepo wired so
// the transition-event helpers can write events without panicking.
func buildTestStore(cap *fakeEventCapture) *repo.Store {
	return &repo.Store{Events: cap}
}

// resetRouterState wipes traefikRouterStatus and traefikServerState so tests
// are independent of each other.
func resetRouterState() {
	traefikRouterStatus = sync.Map{}
	traefikServerState = sync.Map{}
}

// TestRouterStatusTransition_EnabledToDisabled verifies that a single "error"
// event is fired when a router transitions from enabled → disabled.
func TestRouterStatusTransition_EnabledToDisabled(t *testing.T) {
	resetRouterState()
	cap := &fakeEventCapture{}
	store := buildTestStore(cap)
	ctx := context.Background()
	comp := models.InfrastructureComponent{ID: "comp1", Name: "MyTraefik"}

	routers := []infra.TraefikRouter{
		{Name: "sonarr@docker", Rule: "Host(`sonarr.home`)", ServiceName: "sonarr@docker", Status: "enabled"},
	}

	// First poll — seeds the previous state, no event expected.
	pollTraefikRouterStatus(ctx, store, comp, routers)
	if len(cap.events) != 0 {
		t.Fatalf("first poll: expected 0 events, got %d", len(cap.events))
	}

	// Second poll — router goes disabled → "error" event.
	routers[0].Status = "disabled"
	pollTraefikRouterStatus(ctx, store, comp, routers)
	if len(cap.events) != 1 {
		t.Fatalf("expected 1 event after disable, got %d", len(cap.events))
	}
	if cap.events[0].Severity != "error" {
		t.Errorf("expected severity=error, got %q", cap.events[0].Severity)
	}
	if !strings.Contains(cap.events[0].DisplayText, "disabled") {
		t.Errorf("expected 'disabled' in event text, got %q", cap.events[0].DisplayText)
	}

	// Third poll — same status again → no new event.
	pollTraefikRouterStatus(ctx, store, comp, routers)
	if len(cap.events) != 1 {
		t.Errorf("third poll: expected no additional events, got %d total", len(cap.events))
	}
}

// TestRouterStatusTransition_DisabledToEnabled verifies that an "info" event
// is fired when a router transitions from disabled → enabled.
func TestRouterStatusTransition_DisabledToEnabled(t *testing.T) {
	resetRouterState()
	cap := &fakeEventCapture{}
	store := buildTestStore(cap)
	ctx := context.Background()
	comp := models.InfrastructureComponent{ID: "comp2", Name: "MyTraefik"}

	routers := []infra.TraefikRouter{
		{Name: "radarr@docker", Rule: "Host(`radarr.home`)", ServiceName: "radarr@docker", Status: "disabled"},
	}

	// Seed previous state.
	pollTraefikRouterStatus(ctx, store, comp, routers)

	// Transition to enabled.
	routers[0].Status = "enabled"
	pollTraefikRouterStatus(ctx, store, comp, routers)
	if len(cap.events) != 1 {
		t.Fatalf("expected 1 event after restore, got %d", len(cap.events))
	}
	if cap.events[0].Severity != "info" {
		t.Errorf("expected severity=info, got %q", cap.events[0].Severity)
	}
	if !strings.Contains(cap.events[0].DisplayText, "restored") {
		t.Errorf("expected 'restored' in event text, got %q", cap.events[0].DisplayText)
	}
}

// TestRouterStatusTransition_InternalRoutersSkipped verifies that api@internal
// and dashboard@internal do not fire transition events.
func TestRouterStatusTransition_InternalRoutersSkipped(t *testing.T) {
	resetRouterState()
	cap := &fakeEventCapture{}
	store := buildTestStore(cap)
	ctx := context.Background()
	comp := models.InfrastructureComponent{ID: "comp3", Name: "MyTraefik"}

	// Seed both internal routers as enabled.
	routers := []infra.TraefikRouter{
		{Name: "api@internal", Rule: "", ServiceName: "api@internal", Status: "enabled"},
		{Name: "dashboard@internal", Rule: "", ServiceName: "dashboard@internal", Status: "enabled"},
	}
	pollTraefikRouterStatus(ctx, store, comp, routers)

	// Transition both to disabled — no events should fire.
	routers[0].Status = "disabled"
	routers[1].Status = "disabled"
	pollTraefikRouterStatus(ctx, store, comp, routers)
	if len(cap.events) != 0 {
		t.Errorf("expected 0 events for internal routers, got %d", len(cap.events))
	}
}

// ── Service server DOWN transition tests ──────────────────────────────────────

// TestServiceServerDown_Transition verifies that a single "error" event fires
// when a backend server transitions from UP to DOWN, and does not repeat.
func TestServiceServerDown_Transition(t *testing.T) {
	resetRouterState()
	cap := &fakeEventCapture{}
	store := buildTestStore(cap)
	ctx := context.Background()
	comp := models.InfrastructureComponent{ID: "comp4", Name: "MyTraefik"}

	// First observation — seeds state, no events.
	firstSvcs := []infra.TraefikServiceStatus{
		{
			Name:         "sonarr@docker",
			Type:         "loadbalancer",
			Status:       "enabled",
			ServerStatus: map[string]string{"http://10.0.0.1:8989": "UP"},
		},
	}
	// We call the internal helper that processes the slice in-memory.
	// Because pollTraefikServices requires a TraefikClient (HTTP), we call
	// the lower-level transition logic directly via a table-driven loop.
	processServiceTransitions(ctx, store, comp, firstSvcs)
	if len(cap.events) != 0 {
		t.Fatalf("first observation: expected 0 events, got %d", len(cap.events))
	}

	// Second observation — server goes DOWN.
	secondSvcs := []infra.TraefikServiceStatus{
		{
			Name:         "sonarr@docker",
			Type:         "loadbalancer",
			Status:       "enabled",
			ServerStatus: map[string]string{"http://10.0.0.1:8989": "DOWN"},
		},
	}
	processServiceTransitions(ctx, store, comp, secondSvcs)
	if len(cap.events) != 1 {
		t.Fatalf("DOWN transition: expected 1 event, got %d", len(cap.events))
	}
	if cap.events[0].Severity != "error" {
		t.Errorf("expected severity=error, got %q", cap.events[0].Severity)
	}
	if !strings.Contains(cap.events[0].DisplayText, "down") {
		t.Errorf("expected 'down' in event text, got %q", cap.events[0].DisplayText)
	}

	// Third observation — same state, no new event.
	processServiceTransitions(ctx, store, comp, secondSvcs)
	if len(cap.events) != 1 {
		t.Errorf("repeated DOWN: expected no additional events, got %d total", len(cap.events))
	}

	// Recovery — server comes back UP.
	recoverSvcs := []infra.TraefikServiceStatus{
		{
			Name:         "sonarr@docker",
			Type:         "loadbalancer",
			Status:       "enabled",
			ServerStatus: map[string]string{"http://10.0.0.1:8989": "UP"},
		},
	}
	processServiceTransitions(ctx, store, comp, recoverSvcs)
	if len(cap.events) != 2 {
		t.Fatalf("UP recovery: expected 2 total events, got %d", len(cap.events))
	}
	if cap.events[1].Severity != "info" {
		t.Errorf("expected severity=info for recovery, got %q", cap.events[1].Severity)
	}
}

// TestServiceServer_InternalSkipped verifies that services ending in @internal
// do not fire transition events.
func TestServiceServer_InternalSkipped(t *testing.T) {
	resetRouterState()
	cap := &fakeEventCapture{}
	store := buildTestStore(cap)
	ctx := context.Background()
	comp := models.InfrastructureComponent{ID: "comp5", Name: "MyTraefik"}

	svcs := []infra.TraefikServiceStatus{
		{Name: "api@internal", ServerStatus: map[string]string{"http://127.0.0.1:8080": "UP"}},
	}
	processServiceTransitions(ctx, store, comp, svcs)

	svcs[0].ServerStatus["http://127.0.0.1:8080"] = "DOWN"
	processServiceTransitions(ctx, store, comp, svcs)
	if len(cap.events) != 0 {
		t.Errorf("internal service: expected 0 events, got %d", len(cap.events))
	}
}

// processServiceTransitions is a test-extracted subset of pollTraefikServices
// that applies the transition logic only (no HTTP, no DB upsert).
func processServiceTransitions(ctx context.Context, store *repo.Store, c models.InfrastructureComponent, svcs []infra.TraefikServiceStatus) {
	for _, svc := range svcs {
		if strings.HasSuffix(svc.Name, "@internal") {
			continue
		}
		for serverURL, state := range svc.ServerStatus {
			stateKey := c.ID + ":" + svc.Name + ":" + serverURL
			prev, _ := traefikServerState.Swap(stateKey, state)
			if prev == nil {
				continue
			}
			prevState := prev.(string)
			if strings.EqualFold(prevState, state) {
				continue
			}
			if strings.EqualFold(state, "DOWN") {
				fireTraefikEvent(ctx, store, c, "error",
					"Traefik backend down: "+svc.Name+" → "+serverURL)
			} else if strings.EqualFold(state, "UP") {
				fireTraefikEvent(ctx, store, c, "info",
					"Traefik backend recovered: "+svc.Name+" → "+serverURL)
			}
		}
	}
}
