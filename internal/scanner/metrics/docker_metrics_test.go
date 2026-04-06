package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"

	"github.com/digitalcheffe/nora/internal/infra"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// --- mock resourcePollerAPI -----------------------------------------------

type mockResourceClient struct {
	containers []container.Summary
	statsMap   map[string]container.StatsResponse
	statsErr   error
}

func (m *mockResourceClient) ContainerList(_ context.Context, _ container.ListOptions) ([]container.Summary, error) {
	return m.containers, nil
}

func (m *mockResourceClient) ContainerStats(_ context.Context, containerID string, _ bool) (container.StatsResponseReader, error) {
	if m.statsErr != nil {
		return container.StatsResponseReader{}, m.statsErr
	}
	stats := m.statsMap[containerID]
	b, _ := json.Marshal(stats)
	return container.StatsResponseReader{Body: io.NopCloser(strings.NewReader(string(b)))}, nil
}

// --- mock ResourceReadingRepo ---------------------------------------------

type mockResourceReadingRepo struct {
	readings []*models.ResourceReading
}

func (r *mockResourceReadingRepo) Create(_ context.Context, reading *models.ResourceReading) error {
	r.readings = append(r.readings, reading)
	return nil
}

func (r *mockResourceReadingRepo) LatestMetrics(_ context.Context, _ string, _ []string) (map[string]map[string]float64, error) {
	return map[string]map[string]float64{}, nil
}

func (r *mockResourceReadingRepo) BackfillAppID(_ context.Context, _, _ string) (int64, error) {
	return 0, nil
}

// --- mock AppRepo ---------------------------------------------------------

type mockMetricsAppRepo struct {
	apps []models.App
}

func (r *mockMetricsAppRepo) List(_ context.Context) ([]models.App, error)                         { return r.apps, nil }
func (r *mockMetricsAppRepo) ListByHost(_ context.Context, _ string) ([]models.App, error)         { return nil, nil }
func (r *mockMetricsAppRepo) Create(_ context.Context, _ *models.App) error                        { return nil }
func (r *mockMetricsAppRepo) Get(_ context.Context, _ string) (*models.App, error)                 { return nil, repo.ErrNotFound }
func (r *mockMetricsAppRepo) GetByToken(_ context.Context, _ string) (*models.App, error)          { return nil, repo.ErrNotFound }
func (r *mockMetricsAppRepo) Update(_ context.Context, _ *models.App) error                        { return nil }
func (r *mockMetricsAppRepo) Delete(_ context.Context, _ string) error                             { return nil }
func (r *mockMetricsAppRepo) UpdateToken(_ context.Context, _, _ string) error                     { return nil }
func (r *mockMetricsAppRepo) SetDockerEngineID(_ context.Context, _, _ string) error               { return nil }
func (r *mockMetricsAppRepo) SetHostComponentID(_ context.Context, _ string, _ *string) error      { return nil }

// --- mock EventRepo -------------------------------------------------------

type mockMetricsEventRepo struct {
	repo.EventRepo
	created []*models.Event
}

func (r *mockMetricsEventRepo) Create(_ context.Context, ev *models.Event) error {
	r.created = append(r.created, ev)
	return nil
}

func (r *mockMetricsEventRepo) List(_ context.Context, _ repo.ListFilter) ([]models.Event, int, error) {
	return nil, 0, nil
}
func (r *mockMetricsEventRepo) Get(_ context.Context, _ string) (*models.Event, error) {
	return nil, repo.ErrNotFound
}
func (r *mockMetricsEventRepo) CountForCategory(_ context.Context, _ repo.CategoryFilter) (int, error) {
	return 0, nil
}
func (r *mockMetricsEventRepo) SparklineBuckets(_ context.Context, _ repo.CategoryFilter, _ time.Time, _ time.Duration) ([7]int, error) {
	return [7]int{}, nil
}
func (r *mockMetricsEventRepo) LatestPerApp(_ context.Context, _ []string) (map[string]*models.Event, error) {
	return nil, nil
}
func (r *mockMetricsEventRepo) DeleteBySeverityBefore(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, nil
}
func (r *mockMetricsEventRepo) GroupByTypeAndSeverity(_ context.Context, _ string, _, _ time.Time) ([]repo.EventTypeCount, error) {
	return nil, nil
}
func (r *mockMetricsEventRepo) MetricsForApp(_ context.Context, _ string, _, _ time.Time) (repo.EventMetrics, error) {
	return repo.EventMetrics{}, nil
}
func (r *mockMetricsEventRepo) CountPerApp(_ context.Context, _ time.Time) ([]repo.AppEventCount, error) {
	return nil, nil
}
func (r *mockMetricsEventRepo) Timeseries(_ context.Context, _, _ time.Time, _, _, _ string) ([]repo.TimeseriesBucket, error) {
	return nil, nil
}

// --- helpers --------------------------------------------------------------

func newTestResourcePoller(appRepo repo.AppRepo, eventRepo repo.EventRepo, resRepo repo.ResourceReadingRepo, cli resourcePollerAPI) *ResourcePoller {
	store := repo.NewStore(appRepo, eventRepo, nil, nil, resRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	return newResourcePollerWithClient(store, "", cli)
}

func makeStats(totalCPU, prevTotalCPU, systemCPU, prevSystemCPU uint64, percpu []uint64, memUsage, memLimit uint64) container.StatsResponse {
	return container.StatsResponse{
		Name: "/test-container",
		CPUStats: container.CPUStats{
			CPUUsage: container.CPUUsage{
				TotalUsage:  totalCPU,
				PercpuUsage: percpu,
			},
			SystemUsage: systemCPU,
		},
		PreCPUStats: container.CPUStats{
			CPUUsage: container.CPUUsage{
				TotalUsage: prevTotalCPU,
			},
			SystemUsage: prevSystemCPU,
		},
		MemoryStats: container.MemoryStats{
			Usage: memUsage,
			Limit: memLimit,
		},
	}
}

// --- calculateCPUPercent tests -------------------------------------------

func TestCalculateCPUPercent_Normal(t *testing.T) {
	// Container consumed 250ms of one core on a 4-core 1-second window.
	// cpuDelta=250_000_000  systemDelta=4_000_000_000  numCPUs=4
	// result = (250_000_000 / 4_000_000_000) * 4 * 100 = 25%
	got := calculateCPUPercent(250_000_000, 0, 4_000_000_000, 0, 4, 0)
	if got != 25.0 {
		t.Errorf("got %.4f, want 25.0", got)
	}
}

func TestCalculateCPUPercent_FullCore(t *testing.T) {
	// One full core on a 4-core system.
	// systemDelta = 4e9 ns (system elapsed time across all 4 CPUs)
	// cpuDelta    = 1e9 ns (container consumed one full core)
	// result      = (1e9 / 4e9) * 4 * 100 = 100% (one core fully utilised)
	got := calculateCPUPercent(1_000_000_000, 0, 4_000_000_000, 0, 4, 0)
	if got != 100.0 {
		t.Errorf("got %.4f, want 100.0", got)
	}
}

func TestCalculateCPUPercent_ZeroSystemDelta(t *testing.T) {
	got := calculateCPUPercent(100, 0, 0, 0, 4, 0)
	if got != 0 {
		t.Errorf("expected 0 on zero systemDelta, got %.4f", got)
	}
}

func TestCalculateCPUPercent_FallbackOnlineCPUs(t *testing.T) {
	// PercpuUsage empty; OnlineCPUs provides the count (same arithmetic as FullCore)
	got := calculateCPUPercent(1_000_000_000, 0, 4_000_000_000, 0, 0, 4)
	if got != 100.0 {
		t.Errorf("got %.4f, want 100.0", got)
	}
}

func TestCalculateCPUPercent_NoCPUInfo(t *testing.T) {
	got := calculateCPUPercent(100, 0, 1000, 0, 0, 0)
	if got != 0 {
		t.Errorf("expected 0 when no CPU count info, got %.4f", got)
	}
}

// --- calculateMemPercent tests -------------------------------------------

func TestCalculateMemPercent_Normal(t *testing.T) {
	got := calculateMemPercent(512*1024*1024, 1024*1024*1024) // 512 MB of 1 GB = 50%
	if got != 50.0 {
		t.Errorf("got %.4f, want 50.0", got)
	}
}

func TestCalculateMemPercent_ZeroLimit(t *testing.T) {
	got := calculateMemPercent(100, 0)
	if got != 0 {
		t.Errorf("expected 0 on zero limit, got %.4f", got)
	}
}

func TestCalculateMemPercent_Full(t *testing.T) {
	got := calculateMemPercent(1024, 1024)
	if got != 100.0 {
		t.Errorf("got %.4f, want 100.0", got)
	}
}

// --- thresholdFor tests ---------------------------------------------------

func TestThresholdFor(t *testing.T) {
	tests := []struct {
		pct  float64
		want thresholdLevel
	}{
		{0, levelNormal},
		{80, levelNormal},
		{80.1, levelWarn},
		{95, levelWarn},
		{95.1, levelError},
		{100, levelError},
	}
	for _, tc := range tests {
		got := thresholdFor(tc.pct)
		if got != tc.want {
			t.Errorf("thresholdFor(%.1f) = %v, want %v", tc.pct, got, tc.want)
		}
	}
}

// --- PollContainer tests --------------------------------------------------

func TestPollContainer_WritesThreeReadings(t *testing.T) {
	resRepo := &mockResourceReadingRepo{}
	evRepo := &mockMetricsEventRepo{}
	cli := &mockResourceClient{
		statsMap: map[string]container.StatsResponse{
			"ctr1": makeStats(250_000_000, 0, 4_000_000_000, 0, []uint64{0, 0, 0, 0}, 512*1024*1024, 1024*1024*1024),
		},
	}

	p := newTestResourcePoller(&mockMetricsAppRepo{}, evRepo, resRepo, cli)

	if err := p.PollContainer(context.Background(), "ctr1", "app-1"); err != nil {
		t.Fatalf("PollContainer: %v", err)
	}

	if len(resRepo.readings) != 3 {
		t.Fatalf("expected 3 readings (cpu_percent, mem_percent, mem_bytes), got %d", len(resRepo.readings))
	}

	metrics := make(map[string]float64)
	for _, r := range resRepo.readings {
		metrics[r.Metric] = r.Value
		if r.SourceType != "docker_container" {
			t.Errorf("SourceType: got %q, want %q", r.SourceType, "docker_container")
		}
		if r.SourceID != "app-1" {
			t.Errorf("SourceID: got %q, want %q (should use appID when set)", r.SourceID, "app-1")
		}
	}

	for _, metric := range []string{"cpu_percent", "mem_percent", "mem_bytes"} {
		if _, ok := metrics[metric]; !ok {
			t.Errorf("missing reading for metric %q", metric)
		}
	}
}

func TestPollContainer_SourceIDIsStableUUIDWhenNoApp(t *testing.T) {
	resRepo := &mockResourceReadingRepo{}
	cli := &mockResourceClient{
		statsMap: map[string]container.StatsResponse{
			// stats.Name is "/test-container" (set in makeStats helper).
			"ctr1": makeStats(0, 0, 1000, 0, []uint64{0}, 0, 1024),
		},
	}

	p := newTestResourcePoller(&mockMetricsAppRepo{}, &mockMetricsEventRepo{}, resRepo, cli)
	if err := p.PollContainer(context.Background(), "ctr1", ""); err != nil {
		t.Fatalf("PollContainer: %v", err)
	}

	// engineID="" and containerName="test-container" (leading "/" stripped from "/test-container").
	wantID := infra.StableContainerSourceID("", "test-container")
	for _, r := range resRepo.readings {
		if r.SourceID != wantID {
			t.Errorf("expected stable SourceID %q, got %q", wantID, r.SourceID)
		}
	}
}

// --- threshold transition tests ------------------------------------------

func TestThreshold_NormalToWarnCreatesOneEvent(t *testing.T) {
	evRepo := &mockMetricsEventRepo{}
	resRepo := &mockResourceReadingRepo{}
	cli := &mockResourceClient{
		statsMap: map[string]container.StatsResponse{
			// CPU 85% (warn), memory 50% (normal)
			"ctr1": makeStats(850_000_000, 0, 1_000_000_000, 0, []uint64{0}, 512*1024*1024, 1024*1024*1024),
		},
	}

	p := newTestResourcePoller(&mockMetricsAppRepo{}, evRepo, resRepo, cli)
	if err := p.PollContainer(context.Background(), "ctr1", ""); err != nil {
		t.Fatalf("PollContainer: %v", err)
	}

	if len(evRepo.created) != 1 {
		t.Fatalf("expected 1 threshold event, got %d", len(evRepo.created))
	}
	ev := evRepo.created[0]
	if ev.Level != "warn" {
		t.Errorf("Severity: got %q, want %q", ev.Level, "warn")
	}
	if !strings.Contains(ev.Title, "CPU") {
		t.Errorf("DisplayText should mention CPU, got %q", ev.Title)
	}
}

func TestThreshold_NoEventOnSecondPollSameLevel(t *testing.T) {
	evRepo := &mockMetricsEventRepo{}
	resRepo := &mockResourceReadingRepo{}
	cli := &mockResourceClient{
		statsMap: map[string]container.StatsResponse{
			"ctr1": makeStats(850_000_000, 0, 1_000_000_000, 0, []uint64{0}, 512*1024*1024, 1024*1024*1024),
		},
	}

	p := newTestResourcePoller(&mockMetricsAppRepo{}, evRepo, resRepo, cli)
	_ = p.PollContainer(context.Background(), "ctr1", "")
	_ = p.PollContainer(context.Background(), "ctr1", "")

	if len(evRepo.created) != 1 {
		t.Errorf("expected exactly 1 event (no storm on sustained threshold), got %d", len(evRepo.created))
	}
}

func TestThreshold_WarnToErrorCreatesEvent(t *testing.T) {
	evRepo := &mockMetricsEventRepo{}
	resRepo := &mockResourceReadingRepo{}
	cli := &mockResourceClient{
		statsMap: map[string]container.StatsResponse{
			"ctr1": makeStats(850_000_000, 0, 1_000_000_000, 0, []uint64{0}, 0, 1024),
		},
	}

	p := newTestResourcePoller(&mockMetricsAppRepo{}, evRepo, resRepo, cli)
	_ = p.PollContainer(context.Background(), "ctr1", "") // warn

	cli.statsMap["ctr1"] = makeStats(980_000_000, 0, 1_000_000_000, 0, []uint64{0}, 0, 1024)
	_ = p.PollContainer(context.Background(), "ctr1", "") // error

	if len(evRepo.created) != 2 {
		t.Fatalf("expected 2 events (warn + error transition), got %d", len(evRepo.created))
	}
	if evRepo.created[1].Level != "error" {
		t.Errorf("second event Severity: got %q, want error", evRepo.created[1].Level)
	}
}

func TestThreshold_RecoveryCreatesInfoEvent(t *testing.T) {
	evRepo := &mockMetricsEventRepo{}
	resRepo := &mockResourceReadingRepo{}
	cli := &mockResourceClient{
		statsMap: map[string]container.StatsResponse{
			"ctr1": makeStats(850_000_000, 0, 1_000_000_000, 0, []uint64{0}, 0, 1024),
		},
	}

	p := newTestResourcePoller(&mockMetricsAppRepo{}, evRepo, resRepo, cli)
	_ = p.PollContainer(context.Background(), "ctr1", "") // warn

	cli.statsMap["ctr1"] = makeStats(500_000_000, 0, 1_000_000_000, 0, []uint64{0}, 0, 1024)
	_ = p.PollContainer(context.Background(), "ctr1", "") // normal (recovery)

	if len(evRepo.created) != 2 {
		t.Fatalf("expected 2 events (warn + recovery), got %d", len(evRepo.created))
	}
	recovery := evRepo.created[1]
	if recovery.Level != "info" {
		t.Errorf("recovery event Severity: got %q, want info", recovery.Level)
	}
	if !strings.Contains(recovery.Title, "recovered") {
		t.Errorf("recovery DisplayText should contain 'recovered', got %q", recovery.Title)
	}
}

func TestPollContainer_MissingContainerReturnsError(t *testing.T) {
	cli := &mockResourceClient{statsErr: errors.New("No such container: missing")}
	p := newTestResourcePoller(&mockMetricsAppRepo{}, &mockMetricsEventRepo{}, &mockResourceReadingRepo{}, cli)

	err := p.PollContainer(context.Background(), "missing", "")
	if err == nil {
		t.Error("expected error for missing container, got nil")
	}
}

// --- pollAll tests --------------------------------------------------------

func TestPollAll_CleansUpStaleState(t *testing.T) {
	evRepo := &mockMetricsEventRepo{}
	resRepo := &mockResourceReadingRepo{}
	cli := &mockResourceClient{
		containers: []container.Summary{
			{ID: "ctr1", Names: []string{"/app1"}},
			{ID: "ctr2", Names: []string{"/app2"}},
		},
		statsMap: map[string]container.StatsResponse{
			"ctr1": makeStats(850_000_000, 0, 1_000_000_000, 0, []uint64{0}, 0, 1024),
			"ctr2": makeStats(850_000_000, 0, 1_000_000_000, 0, []uint64{0}, 0, 1024),
		},
	}
	p := newTestResourcePoller(&mockMetricsAppRepo{}, evRepo, resRepo, cli)
	p.pollAll(context.Background())

	if _, ok := p.state.Load("ctr1"); !ok {
		t.Error("expected state for ctr1")
	}
	if _, ok := p.state.Load("ctr2"); !ok {
		t.Error("expected state for ctr2")
	}

	// ctr2 disappears
	cli.containers = []container.Summary{{ID: "ctr1", Names: []string{"/app1"}}}
	p.pollAll(context.Background())

	if _, ok := p.state.Load("ctr2"); ok {
		t.Error("expected stale state for ctr2 to be cleaned up after it stopped running")
	}
}
