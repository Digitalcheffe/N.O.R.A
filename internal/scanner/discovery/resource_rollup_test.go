package discovery_test

import (
	"context"
	"testing"
	"time"

	"github.com/digitalcheffe/nora/internal/config"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/internal/scanner/discovery"
	"github.com/digitalcheffe/nora/migrations"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func newTestStore(t *testing.T) (*repo.Store, *sqlx.DB) {
	t.Helper()
	cfg := &config.Config{DBPath: ":memory:"}
	db, err := repo.Open(cfg, migrations.Files)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	store := repo.NewStore(
		repo.NewAppRepo(db),
		repo.NewEventRepo(db),
		repo.NewCheckRepo(db),
		repo.NewRollupRepo(db),
		repo.NewResourceReadingRepo(db),
		repo.NewResourceRollupRepo(db),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		repo.NewAppMetricSnapshotRepo(db),
	)
	return store, db
}

func seedReading(t *testing.T, store *repo.Store, sourceID, sourceType, metric string, value float64, at time.Time) {
	t.Helper()
	err := store.Resources.Create(context.Background(), &models.ResourceReading{
		ID:         uuid.NewString(),
		SourceID:   sourceID,
		SourceType: sourceType,
		Metric:     metric,
		Value:      value,
		RecordedAt: at,
	})
	if err != nil {
		t.Fatalf("seed reading: %v", err)
	}
}

func absf(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func TestRunHourlyRollup_ComputesAvgMinMax(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	hourEnd := time.Now().UTC().Truncate(time.Hour)
	hourStart := hourEnd.Add(-time.Hour)
	mid := hourStart.Add(30 * time.Minute)

	seedReading(t, store, "src-1", "docker_container", "cpu_percent", 10.0, hourStart.Add(time.Minute))
	seedReading(t, store, "src-1", "docker_container", "cpu_percent", 30.0, mid)
	seedReading(t, store, "src-1", "docker_container", "cpu_percent", 20.0, hourEnd.Add(-time.Minute))

	if err := discovery.RunHourlyRollup(ctx, store); err != nil {
		t.Fatalf("RunHourlyRollup: %v", err)
	}

	aggs, err := store.ResourceRollups.AggregateHourlyRollups(ctx, hourStart, hourEnd.Add(time.Hour))
	if err != nil {
		t.Fatalf("AggregateHourlyRollups: %v", err)
	}
	if len(aggs) != 1 {
		t.Fatalf("expected 1 hourly rollup row, got %d", len(aggs))
	}
	a := aggs[0]
	if a.SourceID != "src-1" {
		t.Errorf("SourceID: got %q, want %q", a.SourceID, "src-1")
	}
	wantAvg := (10.0 + 30.0 + 20.0) / 3.0
	if absf(a.Avg-wantAvg) > 0.001 {
		t.Errorf("Avg: got %.4f, want %.4f", a.Avg, wantAvg)
	}
	if a.Min != 10.0 {
		t.Errorf("Min: got %.4f, want 10.0", a.Min)
	}
	if a.Max != 30.0 {
		t.Errorf("Max: got %.4f, want 30.0", a.Max)
	}
}

func TestRunHourlyRollup_MultipleSourcesAndMetrics(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	hourEnd := time.Now().UTC().Truncate(time.Hour)
	hourStart := hourEnd.Add(-time.Hour)
	mid := hourStart.Add(30 * time.Minute)

	seedReading(t, store, "src-a", "docker_container", "cpu_percent", 50.0, mid)
	seedReading(t, store, "src-a", "docker_container", "mem_percent", 60.0, mid)
	seedReading(t, store, "src-b", "docker_container", "cpu_percent", 70.0, mid)

	if err := discovery.RunHourlyRollup(ctx, store); err != nil {
		t.Fatalf("RunHourlyRollup: %v", err)
	}

	aggs, err := store.ResourceRollups.AggregateHourlyRollups(ctx, hourStart, hourEnd.Add(time.Hour))
	if err != nil {
		t.Fatalf("AggregateHourlyRollups: %v", err)
	}
	if len(aggs) != 3 {
		t.Errorf("expected 3 rollup rows (2 metrics for src-a, 1 for src-b), got %d", len(aggs))
	}
}

func TestRunHourlyRollup_NoReadings_NoError(t *testing.T) {
	store, _ := newTestStore(t)
	if err := discovery.RunHourlyRollup(context.Background(), store); err != nil {
		t.Fatalf("RunHourlyRollup with no data: %v", err)
	}
}

func TestRunHourlyRollup_Idempotent(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	hourEnd := time.Now().UTC().Truncate(time.Hour)
	hourStart := hourEnd.Add(-time.Hour)

	seedReading(t, store, "src-1", "docker_container", "cpu_percent", 40.0, hourStart.Add(time.Minute))

	if err := discovery.RunHourlyRollup(ctx, store); err != nil {
		t.Fatalf("first RunHourlyRollup: %v", err)
	}
	if err := discovery.RunHourlyRollup(ctx, store); err != nil {
		t.Fatalf("second RunHourlyRollup: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM resource_rollups WHERE period_type = 'hour'").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 rollup row after idempotent runs, got %d", count)
	}
}
