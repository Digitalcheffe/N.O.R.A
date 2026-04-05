package models

import "time"

// AppMetricSnapshot stores the latest polled value for a single api_polling
// metric on an app. Only one row per (app_id, metric_name) is kept — Upsert
// replaces the previous value in-place.
type AppMetricSnapshot struct {
	ID         string    `db:"id"          json:"id"`
	AppID      string    `db:"app_id"      json:"app_id"`
	ProfileID  string    `db:"profile_id"  json:"profile_id"`
	MetricName string    `db:"metric_name" json:"metric_name"`
	Label      string    `db:"label"       json:"label"`
	Value      string    `db:"value"       json:"value"`
	ValueType  string    `db:"value_type"  json:"value_type"`
	PolledAt   time.Time `db:"polled_at"   json:"polled_at"`
}
