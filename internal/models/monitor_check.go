package models

import "time"

// MonitorCheck represents an active health check configured in NORA.
type MonitorCheck struct {
	ID             string     `db:"id"              json:"id"`
	AppID          string     `db:"app_id"          json:"app_id,omitempty"`
	Name           string     `db:"name"            json:"name"`
	Type           string     `db:"type"            json:"type"`
	Target         string     `db:"target"          json:"target"`
	IntervalSecs   int        `db:"interval_secs"   json:"interval_secs"`
	ExpectedStatus int        `db:"expected_status" json:"expected_status,omitempty"`
	SSLWarnDays    int        `db:"ssl_warn_days"   json:"ssl_warn_days"`
	SSLCritDays    int        `db:"ssl_crit_days"   json:"ssl_crit_days"`
	Enabled        bool       `db:"enabled"         json:"enabled"`
	LastCheckedAt  *time.Time `db:"last_checked_at" json:"last_checked_at,omitempty"`
	LastStatus     string     `db:"last_status"     json:"last_status,omitempty"`
	LastResult     string     `db:"last_result"     json:"last_result,omitempty"`
	CreatedAt      time.Time  `db:"created_at"      json:"created_at"`
}
