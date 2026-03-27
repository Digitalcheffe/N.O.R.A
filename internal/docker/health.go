package docker

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"github.com/google/uuid"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// healthAPI is the minimal Docker API subset used by HealthPoller.
type healthAPI interface {
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
	ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error)
}

// HealthPoller reads the HEALTHCHECK status of running containers every 60 s
// and emits events on healthy ↔ unhealthy transitions.
//
// Containers without a HEALTHCHECK defined are silently ignored.
type HealthPoller struct {
	store  *repo.Store
	client healthAPI
	states sync.Map // containerID → lastHealthStatus (string)
}

// NewHealthPoller returns a HealthPoller connected to the Docker daemon.
func NewHealthPoller(store *repo.Store) (*HealthPoller, error) {
	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("docker health poller: %w", err)
	}
	return &HealthPoller{store: store, client: cli}, nil
}

// Start polls container health every 60 s until ctx is cancelled.
func (p *HealthPoller) Start(ctx context.Context) {
	log.Printf("docker health poller: starting")

	p.poll(ctx)

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("docker health poller: stopped")
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

// CheckContainer inspects containerID and processes its health state immediately.
// Called by the Watcher on container start events for responsiveness.
func (p *HealthPoller) CheckContainer(ctx context.Context, containerID string) {
	info, err := p.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return
	}
	name := containerNameFromInspect(info)
	p.processInspect(ctx, containerID, name, info)
}

// poll iterates all running containers and checks their health state.
func (p *HealthPoller) poll(ctx context.Context) {
	containers, err := p.client.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		log.Printf("docker health poller: list containers: %v", err)
		return
	}
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		info, err := p.client.ContainerInspect(ctx, c.ID)
		if err != nil {
			continue
		}
		if name == "" {
			name = containerNameFromInspect(info)
		}
		p.processInspect(ctx, c.ID, name, info)
	}
}

// processInspect applies health-state transition logic for an already-inspected container.
func (p *HealthPoller) processInspect(ctx context.Context, containerID, containerName string, info container.InspectResponse) {
	if info.State == nil || info.State.Health == nil {
		// No HEALTHCHECK defined on this image — nothing to do.
		return
	}

	healthStatus := info.State.Health.Status
	if healthStatus == "" {
		return
	}

	prev, _ := p.states.Load(containerID)
	prevStr, _ := prev.(string)

	p.states.Store(containerID, healthStatus)

	// No transition on first poll or stable state.
	if prevStr == healthStatus || prevStr == "" {
		return
	}

	switch {
	case healthStatus == "unhealthy":
		p.emitHealthEvent(ctx, containerName, "error",
			fmt.Sprintf("Container unhealthy — %s", containerName),
			info.State.Health)
	case healthStatus == "healthy" && prevStr == "unhealthy":
		p.emitHealthEvent(ctx, containerName, "info",
			fmt.Sprintf("Container healthy — %s", containerName),
			nil)
	}
}

// emitHealthEvent creates an event for a health state change.
func (p *HealthPoller) emitHealthEvent(
	ctx context.Context,
	containerName, severity, displayText string,
	health *container.Health,
) {
	appID := p.findAppID(ctx, containerName)

	fields := fmt.Sprintf(
		`{"source_type":"docker_health","container_name":%s}`,
		jsonStr(containerName),
	)

	if health != nil && len(health.Log) > 0 {
		last := health.Log[len(health.Log)-1]
		fields = fmt.Sprintf(
			`{"source_type":"docker_health","container_name":%s,"health_output":%s}`,
			jsonStr(containerName),
			jsonStr(last.Output),
		)
	}

	ev := &models.Event{
		ID:          uuid.New().String(),
		AppID:       appID,
		ReceivedAt:  time.Now().UTC(),
		Severity:    severity,
		DisplayText: displayText,
		RawPayload:  "{}",
		Fields:      fields,
	}
	if err := p.store.Events.Create(ctx, ev); err != nil {
		log.Printf("docker health poller: create event: %v", err)
	}
}

// findAppID looks for an app whose name matches the container name (case-insensitive).
func (p *HealthPoller) findAppID(ctx context.Context, containerName string) string {
	apps, err := p.store.Apps.List(ctx)
	if err != nil {
		return ""
	}
	for _, a := range apps {
		if strings.EqualFold(a.Name, containerName) {
			return a.ID
		}
	}
	return ""
}

// containerNameFromInspect returns the bare container name (without leading slash).
func containerNameFromInspect(info container.InspectResponse) string {
	if info.ContainerJSONBase == nil {
		return ""
	}
	return strings.TrimPrefix(info.Name, "/")
}
