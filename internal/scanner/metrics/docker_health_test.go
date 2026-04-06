package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// ── mock health API ──────────────────────────────────────────────────────────

type mockHealthAPI struct {
	containers []container.Summary
	inspects   map[string]container.InspectResponse
}

func (m *mockHealthAPI) ContainerList(_ context.Context, _ container.ListOptions) ([]container.Summary, error) {
	return m.containers, nil
}

func (m *mockHealthAPI) ContainerInspect(_ context.Context, id string) (container.InspectResponse, error) {
	if info, ok := m.inspects[id]; ok {
		return info, nil
	}
	return container.InspectResponse{}, nil
}

// ── mock repos ───────────────────────────────────────────────────────────────

type mockHealthAppRepo struct {
	apps []models.App
}

func (r *mockHealthAppRepo) List(_ context.Context) ([]models.App, error)                        { return r.apps, nil }
func (r *mockHealthAppRepo) ListByHost(_ context.Context, _ string) ([]models.App, error)        { return nil, nil }
func (r *mockHealthAppRepo) Create(_ context.Context, _ *models.App) error                       { return nil }
func (r *mockHealthAppRepo) Get(_ context.Context, _ string) (*models.App, error)                { return nil, repo.ErrNotFound }
func (r *mockHealthAppRepo) GetByToken(_ context.Context, _ string) (*models.App, error)         { return nil, repo.ErrNotFound }
func (r *mockHealthAppRepo) Update(_ context.Context, _ *models.App) error                       { return nil }
func (r *mockHealthAppRepo) Delete(_ context.Context, _ string) error                            { return nil }
func (r *mockHealthAppRepo) UpdateToken(_ context.Context, _, _ string) error                    { return nil }

type mockHealthEventRepo struct {
	repo.EventRepo
	created []*models.Event
}

func (r *mockHealthEventRepo) Create(_ context.Context, ev *models.Event) error {
	r.created = append(r.created, ev)
	return nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func inspectWithHealth(name, healthStatus string, logOutputs ...string) container.InspectResponse {
	health := &container.Health{Status: healthStatus}
	for _, l := range logOutputs {
		l := l
		health.Log = append(health.Log, &container.HealthcheckResult{Output: l})
	}
	return container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			Name:  "/" + name,
			State: &container.State{Health: health},
		},
	}
}

func inspectNoHealth(name string) container.InspectResponse {
	return container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			Name:  "/" + name,
			State: &container.State{},
		},
	}
}

func newTestHealthPoller(client healthAPI, eventRepo repo.EventRepo) *HealthPoller {
	store := &repo.Store{
		Apps:   &mockHealthAppRepo{},
		Events: eventRepo,
	}
	return &HealthPoller{store: store, client: client}
}

// ── tests ────────────────────────────────────────────────────────────────────

func TestHealthPoller_UnhealthyTransition(t *testing.T) {
	evRepo := &mockHealthEventRepo{}
	client := &mockHealthAPI{
		containers: []container.Summary{{ID: "c1", Names: []string{"/myapp"}}},
		inspects: map[string]container.InspectResponse{
			"c1": inspectWithHealth("myapp", "unhealthy", "health check failed"),
		},
	}
	p := newTestHealthPoller(client, evRepo)
	p.states.Store("c1", "healthy")

	p.poll(context.Background())

	if len(evRepo.created) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evRepo.created))
	}
	ev := evRepo.created[0]
	if ev.Level != "error" {
		t.Errorf("expected severity=error, got %s", ev.Level)
	}
	prefix := "Container unhealthy"
	if len(ev.Title) < len(prefix) || ev.Title[:len(prefix)] != prefix {
		t.Errorf("unexpected display text: %q", ev.Title)
	}
}

func TestHealthPoller_RecoveryTransition(t *testing.T) {
	evRepo := &mockHealthEventRepo{}
	client := &mockHealthAPI{
		containers: []container.Summary{{ID: "c1", Names: []string{"/myapp"}}},
		inspects: map[string]container.InspectResponse{
			"c1": inspectWithHealth("myapp", "healthy"),
		},
	}
	p := newTestHealthPoller(client, evRepo)
	p.states.Store("c1", "unhealthy")

	p.poll(context.Background())

	if len(evRepo.created) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evRepo.created))
	}
	if evRepo.created[0].Level != "info" {
		t.Errorf("expected severity=info for recovery, got %s", evRepo.created[0].Level)
	}
}

func TestHealthPoller_NoHealthCheck(t *testing.T) {
	evRepo := &mockHealthEventRepo{}
	client := &mockHealthAPI{
		containers: []container.Summary{{ID: "c1", Names: []string{"/myapp"}}},
		inspects: map[string]container.InspectResponse{
			"c1": inspectNoHealth("myapp"),
		},
	}
	p := newTestHealthPoller(client, evRepo)
	p.poll(context.Background())

	if len(evRepo.created) != 0 {
		t.Errorf("expected no events for container without HEALTHCHECK, got %d", len(evRepo.created))
	}
}

func TestHealthPoller_NoTransition(t *testing.T) {
	evRepo := &mockHealthEventRepo{}
	client := &mockHealthAPI{
		containers: []container.Summary{{ID: "c1", Names: []string{"/myapp"}}},
		inspects: map[string]container.InspectResponse{
			"c1": inspectWithHealth("myapp", "healthy"),
		},
	}
	p := newTestHealthPoller(client, evRepo)
	p.states.Store("c1", "healthy")

	p.poll(context.Background())

	if len(evRepo.created) != 0 {
		t.Errorf("expected no events on stable healthy state, got %d", len(evRepo.created))
	}
}

func TestHealthPoller_FirstPoll_NoEvent(t *testing.T) {
	evRepo := &mockHealthEventRepo{}
	client := &mockHealthAPI{
		containers: []container.Summary{{ID: "c1", Names: []string{"/myapp"}}},
		inspects: map[string]container.InspectResponse{
			"c1": inspectWithHealth("myapp", "unhealthy"),
		},
	}
	p := newTestHealthPoller(client, evRepo)
	p.poll(context.Background())

	if len(evRepo.created) != 0 {
		t.Errorf("expected no event on first poll (no baseline), got %d", len(evRepo.created))
	}
}

func TestHealthPoller_Run(t *testing.T) {
	evRepo := &mockHealthEventRepo{}
	client := &mockHealthAPI{}
	p := newTestHealthPoller(client, evRepo)

	done := make(chan struct{})
	go func() {
		p.Run(context.Background())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("health poller did not complete within 2 seconds")
	}
}
