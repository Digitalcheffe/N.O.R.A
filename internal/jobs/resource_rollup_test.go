package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/digitalcheffe/nora/internal/config"
	"github.com/digitalcheffe/nora/internal/jobs"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/migrations"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// newTestStore opens an in-memory SQLite database with all migrations applied and
// returns both the store and the underlying *sqlx.DB for direct queries in tests.
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
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	return store, db
}

// seedReading inserts a resource_readings row at the given time.
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

// ---- daily rollup tests --------------------------------------------------

func TestRunDailyRollup_AggregatesHourlyData(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	dayEnd := time.Now().UTC().Truncate(24 * time.Hour)
	dayStart := dayEnd.Add(-24 * time.Hour)
	hour1 := dayStart
	hour2 := dayStart.Add(time.Hour)

	_ = store.ResourceRollups.Upsert(ctx, &models.ResourceRollup{
		SourceID: "src-1", SourceType: "docker_container", Metric: "cpu_percent",
		PeriodType: "hour", PeriodStart: hour1, Avg: 20.0, Min: 10.0, Max: 30.0,
	})
	_ = store.ResourceRollups.Upsert(ctx, &models.ResourceRollup{
		SourceID: "src-1", SourceType: "docker_container", Metric: "cpu_percent",
		PeriodType: "hour", PeriodStart: hour2, Avg: 40.0, Min: 5.0, Max: 80.0,
	})

	if err := jobs.RunDailyRollup(ctx, store); err != nil {
		t.Fatalf("RunDailyRollup: %v", err)
	}

	type row struct {
		Avg float64 `db:"avg"`
		Min float64 `db:"min"`
		Max float64 `db:"max"`
	}
	var r row
	err := db.QueryRowxContext(ctx, `
		SELECT avg, min, max FROM resource_rollups
		WHERE source_id = ? AND period_type = 'day' AND period_start = ?`,
		"src-1", dayStart).StructScan(&r)
	if err != nil {
		t.Fatalf("query daily rollup row: %v", err)
	}
	wantAvg := (20.0 + 40.0) / 2.0
	if absf(r.Avg-wantAvg) > 0.001 {
		t.Errorf("daily Avg: got %.4f, want %.4f", r.Avg, wantAvg)
	}
	if r.Min != 5.0 {
		t.Errorf("daily Min: got %.4f, want 5.0", r.Min)
	}
	if r.Max != 80.0 {
		t.Errorf("daily Max: got %.4f, want 80.0", r.Max)
	}
}

// ---- purge tests ---------------------------------------------------------

func TestRunRetentionPurge_DeletesOldReadings(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC()
	old := now.Add(-8 * 24 * time.Hour)
	recent := now.Add(-1 * 24 * time.Hour)

	seedReading(t, store, "src-1", "docker_container", "cpu_percent", 10.0, old)
	seedReading(t, store, "src-1", "docker_container", "cpu_percent", 20.0, recent)

	if err := jobs.RunRetentionPurge(ctx, store); err != nil {
		t.Fatalf("RunRetentionPurge: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM resource_readings").Scan(&count); err != nil {
		t.Fatalf("count readings: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 reading to survive purge (recent), got %d", count)
	}
}

func TestRunRetentionPurge_KeepsRecentReadings(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC()
	seedReading(t, store, "src-1", "docker_container", "mem_percent", 50.0, now.Add(-2*24*time.Hour))
	seedReading(t, store, "src-2", "docker_container", "mem_percent", 55.0, now.Add(-6*24*time.Hour))

	if err := jobs.RunRetentionPurge(ctx, store); err != nil {
		t.Fatalf("RunRetentionPurge: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM resource_readings").Scan(&count); err != nil {
		t.Fatalf("count readings: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 readings kept (both within 7 days), got %d", count)
	}
}

func TestRunRetentionPurge_DeletesOldHourlyRollups(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC()
	oldHour := now.Add(-91 * 24 * time.Hour).Truncate(time.Hour)
	recentHour := now.Add(-1 * time.Hour).Truncate(time.Hour)

	_ = store.ResourceRollups.Upsert(ctx, &models.ResourceRollup{
		SourceID: "src-1", SourceType: "docker_container", Metric: "cpu_percent",
		PeriodType: "hour", PeriodStart: oldHour, Avg: 10, Min: 5, Max: 20,
	})
	_ = store.ResourceRollups.Upsert(ctx, &models.ResourceRollup{
		SourceID: "src-1", SourceType: "docker_container", Metric: "cpu_percent",
		PeriodType: "hour", PeriodStart: recentHour, Avg: 15, Min: 8, Max: 25,
	})

	if err := jobs.RunRetentionPurge(ctx, store); err != nil {
		t.Fatalf("RunRetentionPurge: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM resource_rollups WHERE period_type = 'hour'").Scan(&count); err != nil {
		t.Fatalf("count hourly rollups: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 hourly rollup kept (recent), got %d", count)
	}
}

func TestRunRetentionPurge_NeverDeletesDailyRollups(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	ancient := time.Now().UTC().Add(-5 * 365 * 24 * time.Hour).Truncate(24 * time.Hour)
	_ = store.ResourceRollups.Upsert(ctx, &models.ResourceRollup{
		SourceID: "src-1", SourceType: "docker_container", Metric: "cpu_percent",
		PeriodType: "day", PeriodStart: ancient, Avg: 10, Min: 5, Max: 20,
	})

	if err := jobs.RunRetentionPurge(ctx, store); err != nil {
		t.Fatalf("RunRetentionPurge: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM resource_rollups WHERE period_type = 'day'").Scan(&count); err != nil {
		t.Fatalf("count daily rollups: %v", err)
	}
	if count != 1 {
		t.Errorf("daily rollups should never be deleted, got %d", count)
	}
}

// ---- helpers -------------------------------------------------------------

func absf(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
