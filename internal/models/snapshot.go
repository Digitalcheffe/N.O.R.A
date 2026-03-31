package models

import "time"

// Snapshot is a point-in-time reading of a slowly-changing condition value
// for a single infrastructure entity. Written by SnapshotScanner implementations
// (REFACTOR-08) and retained for 48 readings per (entity_id, metric_key) pair.
type Snapshot struct {
	ID            string    `db:"id"`
	EntityType    string    `db:"entity_type"`
	EntityID      string    `db:"entity_id"`
	MetricKey     string    `db:"metric_key"`
	MetricValue   string    `db:"metric_value"`
	PreviousValue *string   `db:"previous_value"`
	CapturedAt    time.Time `db:"captured_at"`
}
