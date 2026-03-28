package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/digitalcheffe/nora/internal/jobs"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/google/uuid"
)

// insertEvent creates an event with the given severity at the specified time.
// app_id is empty so it is stored as NULL (the column is nullable).
func insertEvent(t *testing.T, store *repo.Store, severity string, at time.Time) {
	t.Helper()
	ev := &models.Event{
		ID:          uuid.NewString(),
		AppID:       "",
		ReceivedAt:  at,
		Severity:    severity,
		DisplayText: "test",
		RawPayload:  "{}",
		Fields:      "{}",
	}
	if err := store.Events.Create(context.Background(), ev); err != nil {
		t.Fatalf("insertEvent: %v", err)
	}
}

// ---- retention tests -----------------------------------------------------

func TestRunEventRetention_DeletesExpiredEvents(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC()

	// Each severity: one event just past the window (expired) and one inside (live).
	insertEvent(t, store, "debug", now.Add(-25*time.Hour))      // expired (window=24h)
	insertEvent(t, store, "debug", now.Add(-1*time.Hour))       // live
	insertEvent(t, store, "info", now.Add(-8*24*time.Hour))     // expired (window=7d)
	insertEvent(t, store, "info", now.Add(-1*24*time.Hour))     // live
	insertEvent(t, store, "warn", now.Add(-31*24*time.Hour))    // expired (window=30d)
	insertEvent(t, store, "warn", now.Add(-1*24*time.Hour))     // live
	insertEvent(t, store, "error", now.Add(-91*24*time.Hour))   // expired (window=90d)
	insertEvent(t, store, "error", now.Add(-1*24*time.Hour))    // live
	insertEvent(t, store, "critical", now.Add(-91*24*time.Hour)) // expired (window=90d)
	insertEvent(t, store, "critical", now.Add(-1*24*time.Hour)) // live

	if err := jobs.RunEventRetention(ctx, store); err != nil {
		t.Fatalf("RunEventRetention: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM events").Scan(&count); err != nil {
		t.Fatalf("count events: %v", err)
	}
	// 5 expired deleted, 5 live remain.
	if count != 5 {
		t.Errorf("expected 5 events after retention, got %d", count)
	}
}

func TestRunEventRetention_KeepsEventsWithinWindow(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC()

	// All within their retention windows — none should be deleted.
	insertEvent(t, store, "debug", now.Add(-1*time.Hour))
	insertEvent(t, store, "info", now.Add(-6*24*time.Hour))
	insertEvent(t, store, "warn", now.Add(-29*24*time.Hour))
	insertEvent(t, store, "error", now.Add(-89*24*time.Hour))
	insertEvent(t, store, "critical", now.Add(-89*24*time.Hour))

	if err := jobs.RunEventRetention(ctx, store); err != nil {
		t.Fatalf("RunEventRetention: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM events").Scan(&count); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != 5 {
		t.Errorf("expected all 5 events kept, got %d", count)
	}
}

func TestRunEventRetention_NeverDeletesRollups(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC()

	// Expired debug event so retention has something to purge.
	insertEvent(t, store, "debug", now.Add(-48*time.Hour))

	// Create an app and a rollup row.
	app := &models.App{
		ID: uuid.NewString(), Name: "test-app", Token: uuid.NewString(),
		Config: models.ConfigJSON("{}"), RateLimit: 100,
	}
	if err := store.Apps.Create(ctx, app); err != nil {
		t.Fatalf("create app: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO rollups (app_id, year, month, event_type, severity, count)
		VALUES (?, 2025, 1, 'deploy', 'info', 42)`, app.ID); err != nil {
		t.Fatalf("insert rollup: %v", err)
	}

	if err := jobs.RunEventRetention(ctx, store); err != nil {
		t.Fatalf("RunEventRetention: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM rollups").Scan(&count); err != nil {
		t.Fatalf("count rollups: %v", err)
	}
	if count != 1 {
		t.Errorf("rollup rows must never be deleted by retention, got %d", count)
	}
}

func TestRunEventRetention_NoEvents_NoError(t *testing.T) {
	store, _ := newTestStore(t)
	if err := jobs.RunEventRetention(context.Background(), store); err != nil {
		t.Fatalf("RunEventRetention with no data: %v", err)
	}
}
