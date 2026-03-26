package models

// Rollup represents a monthly event count summary for an app.
type Rollup struct {
	AppID     string `db:"app_id"     json:"app_id"`
	Year      int    `db:"year"       json:"year"`
	Month     int    `db:"month"      json:"month"`
	EventType string `db:"event_type" json:"event_type"`
	Severity  string `db:"severity"   json:"severity"`
	Count     int    `db:"count"      json:"count"`
}
