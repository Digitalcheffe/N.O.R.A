package docker

import (
	"context"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"

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
		Apps:   &mockAppRepo{},
		Events: eventRepo,
	}
	return &HealthPoller{store: store, client: client}
}

// ── tests ────────────────────────────────────────────────────────────────────

// TestHealthPoller_UnhealthyTransition verifies an error event fires when
// a container transitions from healthy to unhealthy.
func TestHealthPoller_UnhealthyTransition(t *testing.T) {
	evRepo := &mockEventRepo{}
	client := &mockHealthAPI{
		containers: []container.Summary{{ID: "c1", Names: []string{"/myapp"}}},
		inspects: map[string]container.InspectResponse{
			"c1": inspectWithHealth("myapp", "unhealthy", "health check failed"),
		},
	}
	p := newTestHealthPoller(client, evRepo)

	// Seed prior state as healthy.
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

// TestHealthPoller_RecoveryTransition verifies an info event fires on unhealthy→healthy.
func TestHealthPoller_RecoveryTransition(t *testing.T) {
	evRepo := &mockEventRepo{}
	client := &mockHealthAPI{
		containers: []container.Summary{{ID: "c1", Names: []string{"/myapp"}}},
		inspects: map[string]container.InspectResponse{
			"c1": inspectWithHealth("myapp", "healthy"),
		},
	}
	p := newTestHealthPoller(client, evRepo)

	// Seed prior state as unhealthy.
	p.states.Store("c1", "unhealthy")

	p.poll(context.Background())

	if len(evRepo.created) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evRepo.created))
	}
	ev := evRepo.created[0]
	if ev.Level != "info" {
		t.Errorf("expected severity=info for recovery, got %s", ev.Level)
	}
}

// TestHealthPoller_NoHealthCheck verifies no event fires for containers without HEALTHCHECK.
func TestHealthPoller_NoHealthCheck(t *testing.T) {
	evRepo := &mockEventRepo{}
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

// TestHealthPoller_NoTransition verifies no event fires when state is unchanged.
func TestHealthPoller_NoTransition(t *testing.T) {
	evRepo := &mockEventRepo{}
	client := &mockHealthAPI{
		containers: []container.Summary{{ID: "c1", Names: []string{"/myapp"}}},
		inspects: map[string]container.InspectResponse{
			"c1": inspectWithHealth("myapp", "healthy"),
		},
	}
	p := newTestHealthPoller(client, evRepo)

	// Same state already stored.
	p.states.Store("c1", "healthy")

	p.poll(context.Background())

	if len(evRepo.created) != 0 {
		t.Errorf("expected no events on stable healthy state, got %d", len(evRepo.created))
	}
}

// TestHealthPoller_FirstPoll_NoEvent verifies no event fires on the first poll
// (no baseline) even if the container is unhealthy.
func TestHealthPoller_FirstPoll_NoEvent(t *testing.T) {
	evRepo := &mockEventRepo{}
	client := &mockHealthAPI{
		containers: []container.Summary{{ID: "c1", Names: []string{"/myapp"}}},
		inspects: map[string]container.InspectResponse{
			"c1": inspectWithHealth("myapp", "unhealthy"),
		},
	}
	p := newTestHealthPoller(client, evRepo)

	// No prior state — first poll.
	p.poll(context.Background())

	if len(evRepo.created) != 0 {
		t.Errorf("expected no event on first poll (no baseline), got %d", len(evRepo.created))
	}
}

// TestHealthPoller_Run verifies a single poll pass completes without error.
func TestHealthPoller_Run(t *testing.T) {
	evRepo := &mockEventRepo{}
	client := &mockHealthAPI{}
	p := newTestHealthPoller(client, evRepo)

	done := make(chan struct{})
	go func() {
		p.Run(context.Background())
		close(done)
	}()

	select {
	case <-done:
		// completed cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("health poller did not complete within 2 seconds")
	}
}
