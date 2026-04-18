package snapshot

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// ── In-memory SnapshotRepo for tests ─────────────────────────────────────────

type memSnapshotRepo struct {
	mu   sync.Mutex
	rows []models.Snapshot
}

func (r *memSnapshotRepo) GetLatest(_ context.Context, entityID, metricKey string) (*models.Snapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var latest *models.Snapshot
	for i := range r.rows {
		row := &r.rows[i]
		if row.EntityID == entityID && row.MetricKey == metricKey {
			if latest == nil || row.CapturedAt.After(latest.CapturedAt) {
				latest = row
			}
		}
	}
	if latest == nil {
		return nil, repo.ErrNotFound
	}
	cp := *latest
	return &cp, nil
}

func (r *memSnapshotRepo) Insert(_ context.Context, s *models.Snapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows = append(r.rows, *s)
	return nil
}

func (r *memSnapshotRepo) Prune(_ context.Context, entityID, metricKey string, limit int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Collect matching rows sorted by time (already insertion-ordered here).
	var matching []models.Snapshot
	var other []models.Snapshot
	for _, row := range r.rows {
		if row.EntityID == entityID && row.MetricKey == metricKey {
			matching = append(matching, row)
		} else {
			other = append(other, row)
		}
	}
	if len(matching) > limit {
		matching = matching[len(matching)-limit:]
	}
	r.rows = append(other, matching...)
	return nil
}

// ── In-memory EventRepo stub ──────────────────────────────────────────────────

type memEventRepo struct {
	mu     sync.Mutex
	events []models.Event
}

func (r *memEventRepo) Create(_ context.Context, ev *models.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, *ev)
	return nil
}

func (r *memEventRepo) List(_ context.Context, _ repo.ListFilter) ([]models.Event, int, error) {
	return nil, 0, nil
}
func (r *memEventRepo) Get(_ context.Context, _ string) (*models.Event, error) {
	return nil, errors.New("not implemented")
}
func (r *memEventRepo) Timeseries(_ context.Context, _, _ time.Time, _, _, _ string) ([]repo.TimeseriesBucket, error) {
	return nil, nil
}
func (r *memEventRepo) CountForCategory(_ context.Context, _ repo.CategoryFilter) (int, error) {
	return 0, nil
}
func (r *memEventRepo) LatestPerApp(_ context.Context, _ []string) (map[string]*models.Event, error) {
	return nil, nil
}
func (r *memEventRepo) DeleteByLevelBefore(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, nil
}
func (r *memEventRepo) GroupByTypeAndLevel(_ context.Context, _ string, _, _ time.Time) ([]repo.EventTypeCount, error) {
	return nil, nil
}
func (r *memEventRepo) MetricsForApp(_ context.Context, _ string, _, _ time.Time) (repo.EventMetrics, error) {
	return repo.EventMetrics{}, nil
}
func (r *memEventRepo) CountPerApp(_ context.Context, _ time.Time) ([]repo.AppEventCount, error) {
	return nil, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func buildTestStore(snapRepo *memSnapshotRepo, evRepo *memEventRepo) *repo.Store {
	return &repo.Store{
		Snapshots: snapRepo,
		Events:    evRepo,
	}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestCaptureSnapshot_FirstRead_NoChange(t *testing.T) {
	snapRepo := &memSnapshotRepo{}
	evRepo := &memEventRepo{}
	store := buildTestStore(snapRepo, evRepo)

	prev, changed := captureSnapshot(context.Background(), store,
		"physical_host", "entity-1", "ssl_days_remaining", "45", time.Now())

	if changed {
		t.Error("want changed=false on first read, got true")
	}
	if prev != "" {
		t.Errorf("want prev='', got %q", prev)
	}
	if len(snapRepo.rows) != 1 {
		t.Errorf("want 1 snapshot row, got %d", len(snapRepo.rows))
	}
}

func TestCaptureSnapshot_SameValue_NoChange(t *testing.T) {
	snapRepo := &memSnapshotRepo{}
	evRepo := &memEventRepo{}
	store := buildTestStore(snapRepo, evRepo)

	now := time.Now()
	captureSnapshot(context.Background(), store, "physical_host", "e1", "metric", "42", now)
	_, changed := captureSnapshot(context.Background(), store, "physical_host", "e1", "metric", "42", now.Add(time.Minute))

	if changed {
		t.Error("want changed=false when value unchanged")
	}
}

func TestCaptureSnapshot_DifferentValue_Changed(t *testing.T) {
	snapRepo := &memSnapshotRepo{}
	evRepo := &memEventRepo{}
	store := buildTestStore(snapRepo, evRepo)

	now := time.Now()
	captureSnapshot(context.Background(), store, "physical_host", "e1", "metric", "42", now)
	prev, changed := captureSnapshot(context.Background(), store, "physical_host", "e1", "metric", "99", now.Add(time.Minute))

	if !changed {
		t.Error("want changed=true when value differs")
	}
	if prev != "42" {
		t.Errorf("want prev=42, got %q", prev)
	}
}

func TestCaptureSnapshot_Prunes_To48(t *testing.T) {
	snapRepo := &memSnapshotRepo{}
	evRepo := &memEventRepo{}
	store := buildTestStore(snapRepo, evRepo)

	now := time.Now()
	for i := 0; i < 60; i++ {
		captureSnapshot(context.Background(), store, "physical_host", "e1", "metric",
			"val", now.Add(time.Duration(i)*time.Minute))
	}

	count := 0
	for _, row := range snapRepo.rows {
		if row.EntityID == "e1" && row.MetricKey == "metric" {
			count++
		}
	}
	if count > snapshotRetain {
		t.Errorf("want at most %d rows after prune, got %d", snapshotRetain, count)
	}
}

func TestStorageCondition(t *testing.T) {
	tests := []struct{ pct float64; want string }{
		{50, "ok"},
		{79.9, "ok"},
		{80, "warn"},
		{89.9, "warn"},
		{90, "error"},
		{99, "error"},
	}
	for _, tt := range tests {
		got := storageCondition(tt.pct)
		if got != tt.want {
			t.Errorf("storageCondition(%.1f) = %q, want %q", tt.pct, got, tt.want)
		}
	}
}

func TestSSLCondition(t *testing.T) {
	tests := []struct{ days int; want string }{
		{-1, "critical"},
		{0, "critical"},
		{1, "error"},
		{7, "error"},
		{8, "warn"},
		{30, "warn"},
		{31, "ok"},
		{365, "ok"},
	}
	for _, tt := range tests {
		got := sslCondition(tt.days)
		if got != tt.want {
			t.Errorf("sslCondition(%d) = %q, want %q", tt.days, got, tt.want)
		}
	}
}

func TestDiskHealthCondition(t *testing.T) {
	tests := []struct{ status, want string }{
		{"normal", "ok"},
		{"warning", "warn"},
		{"critical", "error"},
		{"failing", "error"},
		{"", "ok"},
	}
	for _, tt := range tests {
		got := diskHealthCondition(tt.status)
		if got != tt.want {
			t.Errorf("diskHealthCondition(%q) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestSSLEventTitle_Improvement(t *testing.T) {
	level, title := sslEventTitle("check", "ok", "warn", 45)
	if level != "info" {
		t.Errorf("want level=info on improvement, got %q", level)
	}
	if title == "" {
		t.Error("want non-empty title")
	}
}

func TestSSLEventTitle_Expiry(t *testing.T) {
	level, _ := sslEventTitle("check", "critical", "ok", 0)
	if level != "critical" {
		t.Errorf("want level=critical for expired cert, got %q", level)
	}
}
