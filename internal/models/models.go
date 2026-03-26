package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// RawMessage is a json.RawMessage that can be scanned from SQLite TEXT columns
// and written back as TEXT via the driver.Valuer interface.
type RawMessage json.RawMessage

// Scan implements sql.Scanner — converts a TEXT/BLOB/nil column into RawMessage.
func (r *RawMessage) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		*r = RawMessage(v)
	case []byte:
		cp := make([]byte, len(v))
		copy(cp, v)
		*r = cp
	case nil:
		*r = RawMessage("{}")
	default:
		return fmt.Errorf("RawMessage: cannot scan %T", src)
	}
	return nil
}

// Value implements driver.Valuer — stores as TEXT in SQLite.
func (r RawMessage) Value() (driver.Value, error) {
	if r == nil {
		return "{}", nil
	}
	return string(r), nil
}

// MarshalJSON delegates to the underlying json.RawMessage.
func (r RawMessage) MarshalJSON() ([]byte, error) {
	return json.RawMessage(r).MarshalJSON()
}

// UnmarshalJSON delegates to the underlying json.RawMessage.
func (r *RawMessage) UnmarshalJSON(data []byte) error {
	return (*json.RawMessage)(r).UnmarshalJSON(data)
}

// User represents a NORA user account.
type User struct {
	ID           string    `db:"id"`
	Email        string    `db:"email"`
	PasswordHash string    `db:"password_hash"`
	Role         string    `db:"role"`
	CreatedAt    time.Time `db:"created_at"`
}

// PhysicalHost represents a bare metal or Proxmox node.
type PhysicalHost struct {
	ID        string    `db:"id"`
	Name      string    `db:"name"`
	IP        string    `db:"ip"`
	Type      string    `db:"type"`
	Notes     *string   `db:"notes"`
	CreatedAt time.Time `db:"created_at"`
}

// VirtualHost represents a VM, LXC container, or WSL instance.
type VirtualHost struct {
	ID             string    `db:"id"`
	PhysicalHostID *string   `db:"physical_host_id"`
	Name           string    `db:"name"`
	IP             string    `db:"ip"`
	Type           string    `db:"type"`
	CreatedAt      time.Time `db:"created_at"`
}

// DockerEngine represents a Docker daemon.
type DockerEngine struct {
	ID            string    `db:"id"`
	VirtualHostID *string   `db:"virtual_host_id"`
	Name          string    `db:"name"`
	SocketType    string    `db:"socket_type"`
	SocketPath    string    `db:"socket_path"`
	CreatedAt     time.Time `db:"created_at"`
}

// App represents a monitored application.
type App struct {
	ID             string          `db:"id"`
	Name           string          `db:"name"`
	Token          string          `db:"token"`
	ProfileID      *string         `db:"profile_id"`
	DockerEngineID *string         `db:"docker_engine_id"`
	Config         RawMessage `db:"config"`
	RateLimit      int             `db:"rate_limit"`
	CreatedAt      time.Time       `db:"created_at"`
}

// Event represents a captured ingest event.
type Event struct {
	ID          string          `db:"id"`
	AppID       string          `db:"app_id"`
	ReceivedAt  time.Time       `db:"received_at"`
	Severity    string          `db:"severity"`
	DisplayText string          `db:"display_text"`
	RawPayload  RawMessage `db:"raw_payload"`
	Fields      RawMessage `db:"fields"`
}

// MonitorCheck represents an active monitoring check.
type MonitorCheck struct {
	ID             string     `db:"id"`
	AppID          *string    `db:"app_id"`
	Name           string     `db:"name"`
	Type           string     `db:"type"`
	Target         string     `db:"target"`
	IntervalSecs   int        `db:"interval_secs"`
	ExpectedStatus *int       `db:"expected_status"`
	SSLWarnDays    int        `db:"ssl_warn_days"`
	SSLCritDays    int        `db:"ssl_crit_days"`
	Enabled        bool       `db:"enabled"`
	LastCheckedAt  *time.Time `db:"last_checked_at"`
	LastStatus     *string    `db:"last_status"`
	LastResult     *string    `db:"last_result"`
	CreatedAt      time.Time  `db:"created_at"`
}

// Rollup is a monthly event count rollup.
type Rollup struct {
	AppID     string `db:"app_id"`
	Year      int    `db:"year"`
	Month     int    `db:"month"`
	EventType string `db:"event_type"`
	Severity  string `db:"severity"`
	Count     int    `db:"count"`
}

// Metric is an app-level periodic metric snapshot.
type Metric struct {
	AppID           string    `db:"app_id"`
	Period          time.Time `db:"period"`
	EventsPerHour   int       `db:"events_per_hour"`
	AvgPayloadBytes int       `db:"avg_payload_bytes"`
	PeakPerMinute   int       `db:"peak_per_minute"`
}

// ResourceReading is a raw resource metric reading.
type ResourceReading struct {
	ID         string    `db:"id"`
	SourceID   string    `db:"source_id"`
	SourceType string    `db:"source_type"`
	Metric     string    `db:"metric"`
	Value      float64   `db:"value"`
	RecordedAt time.Time `db:"recorded_at"`
}

// ResourceRollup is a rolled-up resource metric.
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

// AlertRule is a v2 stub for notification rules.
type AlertRule struct {
	ID             string          `db:"id"`
	AppID          *string         `db:"app_id"`
	Name           string          `db:"name"`
	Conditions     RawMessage `db:"conditions"`
	ConditionLogic string          `db:"condition_logic"`
	NotifTitle     string          `db:"notif_title"`
	NotifBody      string          `db:"notif_body"`
	Enabled        bool            `db:"enabled"`
	CreatedAt      time.Time       `db:"created_at"`
}

// WebPushSubscription is a v2 stub for Web Push subscriptions.
type WebPushSubscription struct {
	ID        string    `db:"id"`
	UserID    string    `db:"user_id"`
	Endpoint  string    `db:"endpoint"`
	P256DH    string    `db:"p256dh"`
	Auth      string    `db:"auth"`
	CreatedAt time.Time `db:"created_at"`
}
