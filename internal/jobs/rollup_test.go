package jobs_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/digitalcheffe/nora/internal/jobs"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/google/uuid"
)

// createApp inserts a minimal app and returns its ID.
func createApp(t *testing.T, store *repo.Store) string {
	t.Helper()
	app := &models.App{
		ID:        uuid.NewString(),
		Name:      "test-app-" + uuid.NewString()[:8],
		Token:     uuid.NewString(),
		Config:    json.RawMessage("{}"),
		RateLimit: 100,
	}
	if err := store.Apps.Create(context.Background(), app); err != nil {
		t.Fatalf("create app: %v", err)
	}
	return app.ID
}

// insertAppEvent creates an event attached to a specific app with an event_type
// value embedded in the fields JSON.
func insertAppEvent(t *testing.T, store *repo.Store, appID, severity, eventType string, at time.Time) {
	t.Helper()
	ev := &models.Event{
		ID:          uuid.NewString(),
		AppID:       appID,
		ReceivedAt:  at,
		Severity:    severity,
		DisplayText: "test",
		RawPayload:  "{}",
		Fields:      `{"event_type":"` + eventType + `"}`,
	}
	if err := store.Events.Create(context.Background(), ev); err != nil {
		t.Fatalf("insertAppEvent: %v", err)
	}
}

// ---- rollup tests --------------------------------------------------------

func TestRunMonthlyRollup_CountsByTypeAndSeverity(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	appID := createApp(t, store)

	// Compute previous month bounds the same way RunMonthlyRollup does.
	now := time.Now().UTC()
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	firstOfPrev := firstOfMonth.AddDate(0, -1, 0)
	midPrev := firstOfPrev.Add(15 * 24 * time.Hour)

	// Seed: 3×deploy/info, 2×deploy/warn, 1×restart/info in previous month.
	insertAppEvent(t, store, appID, "info", "deploy", midPrev)
	insertAppEvent(t, store, appID, "info", "deploy", midPrev.Add(time.Hour))
	insertAppEvent(t, store, appID, "info", "deploy", midPrev.Add(2*time.Hour))
	insertAppEvent(t, store, appID, "warn", "deploy", midPrev)
	insertAppEvent(t, store, appID, "warn", "deploy", midPrev.Add(time.Hour))
	insertAppEvent(t, store, appID, "info", "restart", midPrev)

	// This event is in the current month and must NOT be counted.
	insertAppEvent(t, store, appID, "info", "deploy", firstOfMonth.Add(time.Hour))

	if err := jobs.RunMonthlyRollup(ctx, store); err != nil {
		t.Fatalf("RunMonthlyRollup: %v", err)
	}

	year := firstOfPrev.Year()
	month := int(firstOfPrev.Month())
	rollups, err := store.Rollups.ListByPeriod(ctx, year, month)
	if err != nil {
		t.Fatalf("ListByPeriod: %v", err)
	}

	got := make(map[string]int)
	for _, r := range rollups {
		got[r.EventType+"/"+r.Severity] = r.Count
	}

	if got["deploy/info"] != 3 {
		t.Errorf("deploy/info: want 3, got %d", got["deploy/info"])
	}
	if got["deploy/warn"] != 2 {
		t.Errorf("deploy/warn: want 2, got %d", got["deploy/warn"])
	}
	if got["restart/info"] != 1 {
		t.Errorf("restart/info: want 1, got %d", got["restart/info"])
	}
}

func TestRunMonthlyRollup_Idempotent(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	appID := createApp(t, store)

	now := time.Now().UTC()
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	midPrev := firstOfMonth.AddDate(0, -1, 0).Add(15 * 24 * time.Hour)

	insertAppEvent(t, store, appID, "info", "deploy", midPrev)

	if err := jobs.RunMonthlyRollup(ctx, store); err != nil {
		t.Fatalf("first RunMonthlyRollup: %v", err)
	}
	if err := jobs.RunMonthlyRollup(ctx, store); err != nil {
		t.Fatalf("second RunMonthlyRollup: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM rollups").Scan(&count); err != nil {
		t.Fatalf("count rollups: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 rollup row after idempotent runs, got %d", count)
	}
}

func TestRunMonthlyRollup_NoApps_NoError(t *testing.T) {
	store, _ := newTestStore(t)
	if err := jobs.RunMonthlyRollup(context.Background(), store); err != nil {
		t.Fatalf("RunMonthlyRollup with no apps: %v", err)
	}
}

func TestRunMonthlyRollup_EventsOutsideWindowNotCounted(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	appID := createApp(t, store)

	// Only an event in the current month — previous-month rollup must be empty.
	now := time.Now().UTC()
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	insertAppEvent(t, store, appID, "info", "deploy", firstOfMonth.Add(time.Hour))

	if err := jobs.RunMonthlyRollup(ctx, store); err != nil {
		t.Fatalf("RunMonthlyRollup: %v", err)
	}

	firstOfPrev := firstOfMonth.AddDate(0, -1, 0)
	rollups, err := store.Rollups.ListByPeriod(ctx, firstOfPrev.Year(), int(firstOfPrev.Month()))
	if err != nil {
		t.Fatalf("ListByPeriod: %v", err)
	}
	if len(rollups) != 0 {
		t.Errorf("expected 0 rollup rows for previous month, got %d", len(rollups))
	}
}

// TestRollupRunsBeforeRetention verifies that running rollup before retention
// preserves summary data even when the raw events are subsequently purged.
func TestRollupRunsBeforeRetention(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	appID := createApp(t, store)

	now := time.Now().UTC()
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	firstOfPrev := firstOfMonth.AddDate(0, -1, 0)
	midPrev := firstOfPrev.Add(15 * 24 * time.Hour)

	// Only insert if mid-prev is outside the info retention window (>7d ago).
	if now.Sub(midPrev) < 7*24*time.Hour {
		t.Skip("mid-prev is within the info retention window; skipping boundary test")
	}

	insertAppEvent(t, store, appID, "info", "deploy", midPrev)

	// Rollup first (production order), then retention.
	if err := jobs.RunMonthlyRollup(ctx, store); err != nil {
		t.Fatalf("RunMonthlyRollup: %v", err)
	}
	if err := jobs.RunEventRetention(ctx, store); err != nil {
		t.Fatalf("RunEventRetention: %v", err)
	}

	// Raw event should be purged by the info retention window (7d).
	var eventCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM events WHERE app_id = ?", appID).Scan(&eventCount); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if eventCount != 0 {
		t.Errorf("expected expired event to be purged, got %d remaining", eventCount)
	}

	// But the rollup must survive.
	var rollupTotal int
	if err := db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(count), 0) FROM rollups WHERE app_id = ?", appID,
	).Scan(&rollupTotal); err != nil {
		t.Fatalf("sum rollups: %v", err)
	}
	if rollupTotal == 0 {
		t.Error("rollup should have preserved event count before retention purge")
	}
}
