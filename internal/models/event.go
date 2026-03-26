package models

import "time"

// Event represents a single inbound event captured from an application webhook.
type Event struct {
	ID          string    `db:"id"           json:"id"`
	AppID       string    `db:"app_id"       json:"app_id"`
	AppName     string    `db:"app_name"     json:"app_name"`
	ReceivedAt  time.Time `db:"received_at"  json:"received_at"`
	Severity    string    `db:"severity"     json:"severity"`
	DisplayText string    `db:"display_text" json:"display_text"`
	RawPayload  string    `db:"raw_payload"  json:"raw_payload"`
	Fields      string    `db:"fields"       json:"fields"`
}
