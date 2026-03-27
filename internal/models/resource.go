package models

import "time"

// ResourceReading is a single metric sample from a Docker container, host, or VM.
type ResourceReading struct {
	ID         string    `db:"id"`
	SourceID   string    `db:"source_id"`
	SourceType string    `db:"source_type"`
	Metric     string    `db:"metric"`
	Value      float64   `db:"value"`
	RecordedAt time.Time `db:"recorded_at"`
}
