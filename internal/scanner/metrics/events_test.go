package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// ── Threshold function tests ──────────────────────────────────────────────────

func TestCPUThreshold(t *testing.T) {
	tests := []struct {
		pct  float64
		want thresholdLevel
	}{
		{0, levelNormal},
		{50, levelNormal},
		{90, levelNormal}, // exactly 90 is not above 90
		{90.1, levelWarn},
		{100, levelWarn},
	}
	for _, tc := range tests {
		got := cpuThreshold(tc.pct)
		if got != tc.want {
			t.Errorf("cpuThreshold(%.1f) = %v, want %v", tc.pct, got, tc.want)
		}
	}
}

func TestMemThreshold(t *testing.T) {
	tests := []struct {
		pct  float64
		want thresholdLevel
	}{
		{0, levelNormal},
		{90, levelNormal},
		{91, levelWarn},
	}
	for _, tc := range tests {
		got := memThreshold(tc.pct)
		if got != tc.want {
			t.Errorf("memThreshold(%.1f) = %v, want %v", tc.pct, got, tc.want)
		}
	}
}

func TestTempThreshold(t *testing.T) {
	tests := []struct {
		tempC float64
		want  thresholdLevel
	}{
		{20, levelNormal},
		{80, levelNormal},
		{80.1, levelWarn},
		{90, levelWarn},
		{90.1, levelError},
		{100, levelError},
	}
	for _, tc := range tests {
		got := tempThreshold(tc.tempC)
		if got != tc.want {
			t.Errorf("tempThreshold(%.1f) = %v, want %v", tc.tempC, got, tc.want)
		}
	}
}

// ── ThresholdTracker tests ────────────────────────────────────────────────────

// stubEventRepo implements repo.EventRepo, recording only Create calls.
type stubEventRepo struct {
	events []*models.Event
}

func (r *stubEventRepo) Create(_ context.Context, ev *models.Event) error {
	r.events = append(r.events, ev)
	return nil
}
func (r *stubEventRepo) List(_ context.Context, _ repo.ListFilter) ([]models.Event, int, error) {
	return nil, 0, nil
}
func (r *stubEventRepo) Get(_ context.Context, _ string) (*models.Event, error) { return nil, nil }
func (r *stubEventRepo) Timeseries(_ context.Context, _, _ time.Time, _, _, _ string) ([]repo.TimeseriesBucket, error) {
	return nil, nil
}
func (r *stubEventRepo) CountForCategory(_ context.Context, _ repo.CategoryFilter) (int, error) {
	return 0, nil
}
func (r *stubEventRepo) SparklineBuckets(_ context.Context, _ repo.CategoryFilter, _ time.Time, _ time.Duration) ([7]int, error) {
	return [7]int{}, nil
}
func (r *stubEventRepo) LatestPerApp(_ context.Context, _ []string) (map[string]*models.Event, error) {
	return nil, nil
}
func (r *stubEventRepo) DeleteByLevelBefore(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, nil
}
func (r *stubEventRepo) GroupByTypeAndLevel(_ context.Context, _ string, _, _ time.Time) ([]repo.EventTypeCount, error) {
	return nil, nil
}
func (r *stubEventRepo) MetricsForApp(_ context.Context, _ string, _, _ time.Time) (repo.EventMetrics, error) {
	return repo.EventMetrics{}, nil
}
func (r *stubEventRepo) CountPerApp(_ context.Context, _ time.Time) ([]repo.AppEventCount, error) {
	return nil, nil
}

func newStubStore() (*repo.Store, *stubEventRepo) {
	evRepo := &stubEventRepo{}
	return &repo.Store{Events: evRepo}, evRepo
}

func TestThresholdTracker_FiresOnCrossing(t *testing.T) {
	store, evRepo := newStubStore()
	tr := newThresholdTracker()

	titleFn := func(l thresholdLevel) string {
		if l == levelNormal {
			return "recovered"
		}
		return "high"
	}

	// First call at levelNormal — no event emitted (no prior state transition).
	tr.CheckAndFire(context.Background(), store, "e1", "host", "physical_host", "cpu_percent", levelNormal, titleFn)
	if got := len(evRepo.events); got != 0 {
		t.Fatalf("expected 0 events after first normal, got %d", got)
	}

	// Cross into warn — one warn event.
	tr.CheckAndFire(context.Background(), store, "e1", "host", "physical_host", "cpu_percent", levelWarn, titleFn)
	if got := len(evRepo.events); got != 1 {
		t.Fatalf("expected 1 event after warn crossing, got %d", got)
	}
	if evRepo.events[0].Level != "warn" {
		t.Errorf("expected warn level, got %s", evRepo.events[0].Level)
	}

	// Same level again — no additional event.
	tr.CheckAndFire(context.Background(), store, "e1", "host", "physical_host", "cpu_percent", levelWarn, titleFn)
	if got := len(evRepo.events); got != 1 {
		t.Fatalf("expected still 1 event after second warn reading, got %d", got)
	}

	// Recovery → info event.
	tr.CheckAndFire(context.Background(), store, "e1", "host", "physical_host", "cpu_percent", levelNormal, titleFn)
	if got := len(evRepo.events); got != 2 {
		t.Fatalf("expected 2 events after recovery, got %d", got)
	}
	if evRepo.events[1].Level != "info" {
		t.Errorf("expected info level for recovery event, got %s", evRepo.events[1].Level)
	}
}

func TestThresholdTracker_IndependentKeys(t *testing.T) {
	store, evRepo := newStubStore()
	tr := newThresholdTracker()
	titleFn := func(_ thresholdLevel) string { return "t" }

	// Two different entities crossing the same metric should each fire independently.
	tr.CheckAndFire(context.Background(), store, "e1", "host1", "physical_host", "cpu_percent", levelWarn, titleFn)
	tr.CheckAndFire(context.Background(), store, "e2", "host2", "physical_host", "cpu_percent", levelWarn, titleFn)
	if got := len(evRepo.events); got != 2 {
		t.Fatalf("expected 2 events for 2 entities, got %d", got)
	}
}

func TestThresholdTracker_EscalatesFromWarnToError(t *testing.T) {
	store, evRepo := newStubStore()
	tr := newThresholdTracker()
	titleFn := func(l thresholdLevel) string {
		switch l {
		case levelError:
			return "error"
		case levelWarn:
			return "warn"
		default:
			return "ok"
		}
	}

	// warn → error escalation fires an event.
	tr.CheckAndFire(context.Background(), store, "e1", "h", "physical_host", "temp", levelWarn, titleFn)
	tr.CheckAndFire(context.Background(), store, "e1", "h", "physical_host", "temp", levelError, titleFn)
	if got := len(evRepo.events); got != 2 {
		t.Fatalf("expected 2 events (warn + error), got %d", got)
	}
	if evRepo.events[1].Level != "error" {
		t.Errorf("expected error level, got %s", evRepo.events[1].Level)
	}
}
