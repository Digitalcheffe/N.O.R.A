package models

import "time"

// Metric holds per-app hourly event metrics for a specific period.
type Metric struct {
	AppID           string    `db:"app_id"`
	Period          time.Time `db:"period"`
	EventsPerHour   int       `db:"events_per_hour"`
	AvgPayloadBytes int       `db:"avg_payload_bytes"`
	PeakPerMinute   int       `db:"peak_per_minute"`
}
