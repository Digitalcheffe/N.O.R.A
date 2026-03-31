// Package snapshot provides SnapshotScanner implementations for each
// infrastructure integration type supported by NORA.
//
// Each scanner captures slowly-changing condition data — SSL cert expiry,
// storage utilisation, OS versions, update counts, disk health — and writes
// point-in-time snapshot rows to the snapshots table every 30 minutes
// (scanner.SnapshotInterval).
//
// Design rules:
//   - Read the previous snapshot from the DB; compare to the new value.
//   - Fire an event only when the condition changes — never on unchanged values.
//   - Write a debug event on every successful snapshot run.
//   - Retain the last 48 readings per (entity_id, metric_key); prune after insert.
//   - Source attribution on events matches the REFACTOR-01 vocabulary:
//     physical_host, monitor_check, or snmp_host.
package snapshot

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/google/uuid"
)

const snapshotRetain = 48

// storageCondition maps a utilisation percentage to a condition bucket.
// < 80 % → "ok"; 80–90 % → "warn"; ≥ 90 % → "error".
func storageCondition(pct float64) string {
	switch {
	case pct >= 90:
		return "error"
	case pct >= 80:
		return "warn"
	default:
		return "ok"
	}
}

// sslCondition maps days-remaining to a condition bucket.
// ≤ 0 → "critical"; ≤ 7 → "error"; ≤ 30 → "warn"; > 30 → "ok".
func sslCondition(days int) string {
	switch {
	case days <= 0:
		return "critical"
	case days <= 7:
		return "error"
	case days <= 30:
		return "warn"
	default:
		return "ok"
	}
}

// diskHealthCondition maps a disk status string to a normalised condition bucket.
// "normal" → "ok"; "warning" → "warn"; "critical"/"failing" → "error".
func diskHealthCondition(status string) string {
	switch status {
	case "warning":
		return "warn"
	case "critical", "failing":
		return "error"
	default:
		return "ok"
	}
}

// captureSnapshot reads the previous snapshot from the DB, inserts the new
// value, prunes to snapshotRetain rows, and returns (previousValue, changed).
// On the first reading for a key, previousValue is "" and changed is false.
func captureSnapshot(
	ctx context.Context,
	store *repo.Store,
	entityType, entityID, metricKey, metricValue string,
	now time.Time,
) (prevValue string, changed bool) {
	prevSnap, err := store.Snapshots.GetLatest(ctx, entityID, metricKey)
	var prev *string
	if err == nil {
		prev = &prevSnap.MetricValue
		changed = prevSnap.MetricValue != metricValue
	}

	s := &models.Snapshot{
		ID:            uuid.New().String(),
		EntityType:    entityType,
		EntityID:      entityID,
		MetricKey:     metricKey,
		MetricValue:   metricValue,
		PreviousValue: prev,
		CapturedAt:    now,
	}
	if insertErr := store.Snapshots.Insert(ctx, s); insertErr != nil {
		log.Printf("snapshot: insert %s/%s: %v", entityID, metricKey, insertErr)
	}
	if pruneErr := store.Snapshots.Prune(ctx, entityID, metricKey, snapshotRetain); pruneErr != nil {
		log.Printf("snapshot: prune %s/%s: %v", entityID, metricKey, pruneErr)
	}

	if prev != nil {
		return *prev, changed
	}
	return "", false
}

// writeSnapshotEvent persists a single event for a snapshot condition change.
func writeSnapshotEvent(
	ctx context.Context,
	store *repo.Store,
	sourceID, sourceName, sourceType, level, title string,
) {
	payload := fmt.Sprintf(
		`{"bucket":"snapshot","source_id":%q,"source_name":%q}`,
		sourceID, sourceName,
	)
	ev := &models.Event{
		ID:         uuid.New().String(),
		Level:      level,
		SourceName: sourceName,
		SourceType: sourceType,
		SourceID:   sourceID,
		Title:      title,
		Payload:    payload,
		CreatedAt:  time.Now().UTC(),
	}
	if err := store.Events.Create(ctx, ev); err != nil {
		log.Printf("snapshot: write event for %s (%s): %v", sourceName, sourceID, err)
	}
}

// writeDebugEvent writes a debug-level completion event for a snapshot pass.
func writeDebugEvent(ctx context.Context, store *repo.Store, sourceID, sourceName, sourceType string) {
	writeSnapshotEvent(ctx, store, sourceID, sourceName, sourceType, "debug",
		fmt.Sprintf("[snapshot] %s snapshot completed", sourceName))
}

// storageEventTitle returns a human-readable event title for a storage
// utilisation condition change on a named pool.
func storageEventTitle(componentName, poolKey, newCondition string, pct float64) (level, title string) {
	switch newCondition {
	case "error":
		return "error", fmt.Sprintf("[snapshot] Storage critical — %s %s: %.1f%%", componentName, poolKey, pct)
	case "warn":
		return "warn", fmt.Sprintf("[snapshot] Storage high — %s %s: %.1f%%", componentName, poolKey, pct)
	default:
		return "info", fmt.Sprintf("[snapshot] Storage recovered — %s %s: %.1f%%", componentName, poolKey, pct)
	}
}
