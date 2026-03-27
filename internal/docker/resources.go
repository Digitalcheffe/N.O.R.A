package docker

import (
	"context"
	"encoding/json"
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

// resourcePollerAPI is the minimal Docker API subset needed for resource polling,
// enabling mock injection in tests.
type resourcePollerAPI interface {
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
	ContainerStats(ctx context.Context, containerID string, stream bool) (container.StatsResponseReader, error)
}

// thresholdLevel represents a metric breach level.
type thresholdLevel int

const (
	levelNormal thresholdLevel = iota
	levelWarn
	levelError
)

func (l thresholdLevel) String() string {
	switch l {
	case levelWarn:
		return "warn"
	case levelError:
		return "error"
	default:
		return ""
	}
}

// containerThresholdState tracks the last known threshold level for a container's metrics
// so that events are only emitted on state transitions.
type containerThresholdState struct {
	cpu thresholdLevel
	mem thresholdLevel
}

// ResourcePoller polls CPU and memory stats from all running Docker containers
// every 60 seconds and writes readings to the resource_readings table.
// Threshold crossings generate events.
type ResourcePoller struct {
	store  *repo.Store
	client resourcePollerAPI
	state  sync.Map // containerID -> containerThresholdState
}

// NewResourcePoller creates a ResourcePoller connected to the Docker daemon.
func NewResourcePoller(store *repo.Store) (*ResourcePoller, error) {
	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &ResourcePoller{store: store, client: cli}, nil
}

// newResourcePollerWithClient creates a ResourcePoller with an injected client (for tests).
func newResourcePollerWithClient(store *repo.Store, cli resourcePollerAPI) *ResourcePoller {
	return &ResourcePoller{store: store, client: cli}
}

// Start polls all running containers every 60 seconds until ctx is cancelled.
func (p *ResourcePoller) Start(ctx context.Context) {
	log.Printf("resource poller: starting")
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	// Poll immediately on start, then on each tick.
	p.pollAll(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Printf("resource poller: stopped")
			return
		case <-ticker.C:
			p.pollAll(ctx)
		}
	}
}

// pollAll lists running containers and calls PollContainer for each one.
func (p *ResourcePoller) pollAll(ctx context.Context) {
	containers, err := p.client.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		log.Printf("resource poller: list containers: %v", err)
		return
	}

	// Build a set of currently running container IDs so we can prune stale state.
	running := make(map[string]struct{}, len(containers))
	for _, c := range containers {
		running[c.ID] = struct{}{}
	}
	// Remove threshold state for containers that are no longer running.
	p.state.Range(func(key, _ any) bool {
		if _, ok := running[key.(string)]; !ok {
			p.state.Delete(key)
		}
		return true
	})

	// Resolve app IDs for running containers by matching container name → app name.
	appIDs := p.resolveAppIDs(ctx, containers)

	for _, c := range containers {
		appID := appIDs[c.ID]
		if err := p.PollContainer(ctx, c.ID, appID); err != nil {
			log.Printf("resource poller: poll container %s: %v", c.ID[:12], err)
		}
	}
}

// resolveAppIDs returns a map of containerID → appID by matching container names
// against app names (case-insensitive). Unmatched containers map to "".
func (p *ResourcePoller) resolveAppIDs(ctx context.Context, containers []container.Summary) map[string]string {
	apps, err := p.store.Apps.List(ctx)
	if err != nil {
		log.Printf("resource poller: list apps: %v", err)
		return map[string]string{}
	}

	appByName := make(map[string]string, len(apps))
	for _, a := range apps {
		appByName[strings.ToLower(a.Name)] = a.ID
	}

	result := make(map[string]string, len(containers))
	for _, c := range containers {
		for _, name := range c.Names {
			// Docker names have a leading "/" — strip it.
			trimmed := strings.TrimPrefix(name, "/")
			if id, ok := appByName[strings.ToLower(trimmed)]; ok {
				result[c.ID] = id
				break
			}
		}
	}
	return result
}

// PollContainer fetches one-shot stats for containerID, writes resource readings,
// and emits threshold events on level transitions.
func (p *ResourcePoller) PollContainer(ctx context.Context, containerID string, appID string) error {
	resp, err := p.client.ContainerStats(ctx, containerID, false)
	if err != nil {
		return fmt.Errorf("container stats: %w", err)
	}
	defer resp.Body.Close()

	var stats container.StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return fmt.Errorf("decode stats: %w", err)
	}

	cpuPct := calculateCPUPercent(
		stats.CPUStats.CPUUsage.TotalUsage,
		stats.PreCPUStats.CPUUsage.TotalUsage,
		stats.CPUStats.SystemUsage,
		stats.PreCPUStats.SystemUsage,
		len(stats.CPUStats.CPUUsage.PercpuUsage),
		int(stats.CPUStats.OnlineCPUs),
	)
	memPct := calculateMemPercent(stats.MemoryStats.Usage, stats.MemoryStats.Limit)
	memBytes := float64(stats.MemoryStats.Usage)

	now := time.Now().UTC()
	sourceID := containerID
	if appID != "" {
		sourceID = appID
	}

	for _, m := range []struct {
		metric string
		value  float64
	}{
		{"cpu_percent", cpuPct},
		{"mem_percent", memPct},
		{"mem_bytes", memBytes},
	} {
		r := &models.ResourceReading{
			ID:         uuid.New().String(),
			SourceID:   sourceID,
			SourceType: "docker_container",
			Metric:     m.metric,
			Value:      m.value,
			RecordedAt: now,
		}
		if err := p.store.Resources.Create(ctx, r); err != nil {
			log.Printf("resource poller: write reading %s: %v", m.metric, err)
		}
	}

	// Resolve display name for event messages (strip leading "/").
	shortID := containerID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}
	containerName := shortID
	if len(stats.Name) > 0 {
		containerName = strings.TrimPrefix(stats.Name, "/")
	}

	p.checkThresholds(ctx, containerID, appID, containerName, cpuPct, memPct, now)
	return nil
}

// checkThresholds compares current metric levels against the last recorded state
// and creates an event on each transition.
func (p *ResourcePoller) checkThresholds(
	ctx context.Context,
	containerID, appID, containerName string,
	cpuPct, memPct float64,
	now time.Time,
) {
	prev := containerThresholdState{}
	if v, ok := p.state.Load(containerID); ok {
		prev = v.(containerThresholdState)
	}

	newCPU := thresholdFor(cpuPct)
	newMem := thresholdFor(memPct)

	if newCPU != prev.cpu {
		p.emitThresholdEvent(ctx, appID, containerName, "CPU", cpuPct, prev.cpu, newCPU, now)
	}
	if newMem != prev.mem {
		p.emitThresholdEvent(ctx, appID, containerName, "Memory", memPct, prev.mem, newMem, now)
	}

	p.state.Store(containerID, containerThresholdState{cpu: newCPU, mem: newMem})
}

// thresholdFor maps a percentage value to its threshold level.
func thresholdFor(pct float64) thresholdLevel {
	switch {
	case pct > 95:
		return levelError
	case pct > 80:
		return levelWarn
	default:
		return levelNormal
	}
}

// emitThresholdEvent writes a single event for a metric threshold transition.
func (p *ResourcePoller) emitThresholdEvent(
	ctx context.Context,
	appID, containerName, metric string,
	pct float64,
	from, to thresholdLevel,
	now time.Time,
) {
	if p.store.Events == nil {
		return
	}

	var severity, text string
	switch to {
	case levelError:
		severity = "error"
		text = fmt.Sprintf("High %s — %s: %.1f%%", metric, containerName, pct)
	case levelWarn:
		severity = "warn"
		text = fmt.Sprintf("High %s — %s: %.1f%%", metric, containerName, pct)
	case levelNormal:
		// Recovery from a previous breach.
		severity = "info"
		prevSev := from.String()
		text = fmt.Sprintf("%s recovered — %s: %.1f%% (was %s)", metric, containerName, pct, prevSev)
	}

	ev := &models.Event{
		ID:          uuid.New().String(),
		AppID:       appID,
		ReceivedAt:  now,
		Severity:    severity,
		DisplayText: text,
		RawPayload:  "{}",
		Fields: fmt.Sprintf(
			`{"source_type":"docker_container","metric":"%s","value":%.2f}`,
			strings.ToLower(metric), pct,
		),
	}

	if err := p.store.Events.Create(ctx, ev); err != nil {
		log.Printf("resource poller: create threshold event: %v", err)
	}
}

// calculateCPUPercent computes the CPU usage percentage from a pair of stat snapshots.
// It follows the Docker documentation formula:
//
//	cpuDelta / systemDelta * numCPUs * 100
func calculateCPUPercent(
	totalUsage, prevTotalUsage,
	systemUsage, prevSystemUsage uint64,
	percpuLen, onlineCPUs int,
) float64 {
	cpuDelta := float64(totalUsage) - float64(prevTotalUsage)
	systemDelta := float64(systemUsage) - float64(prevSystemUsage)
	if systemDelta <= 0 || cpuDelta < 0 {
		return 0
	}
	numCPUs := percpuLen
	if numCPUs == 0 {
		numCPUs = onlineCPUs
	}
	if numCPUs == 0 {
		return 0
	}
	return (cpuDelta / systemDelta) * float64(numCPUs) * 100
}

// calculateMemPercent returns memory usage as a percentage of the container limit.
func calculateMemPercent(usage, limit uint64) float64 {
	if limit == 0 {
		return 0
	}
	return (float64(usage) / float64(limit)) * 100
}
