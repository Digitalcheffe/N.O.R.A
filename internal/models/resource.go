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

// ResourceRollup is an aggregated summary of resource readings for a time period.
type ResourceRollup struct {
	SourceID    string    `db:"source_id"`
	SourceType  string    `db:"source_type"`
	Metric      string    `db:"metric"`
	PeriodType  string    `db:"period_type"`
	PeriodStart time.Time `db:"period_start"`
	Avg         float64   `db:"avg"`
	Min         float64   `db:"min"`
	Max         float64   `db:"max"`
}
