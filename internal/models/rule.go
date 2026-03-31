package models

import "time"

// Rule is a notification rule that evaluates incoming events and dispatches
// outbound notifications when conditions match.
type Rule struct {
	ID              string    `db:"id"               json:"id"`
	Name            string    `db:"name"             json:"name"`
	Enabled         bool      `db:"enabled"          json:"enabled"`
	SourceID        *string   `db:"source_id"        json:"source_id"`
	SourceType      *string   `db:"source_type"      json:"source_type"`
	Severity        *string   `db:"severity"         json:"severity"`
	Conditions      string    `db:"conditions"       json:"conditions"`      // JSON array
	ConditionLogic  string    `db:"condition_logic"  json:"condition_logic"` // AND | OR
	DeliveryEmail   bool      `db:"delivery_email"   json:"delivery_email"`
	DeliveryPush    bool      `db:"delivery_push"    json:"delivery_push"`
	DeliveryWebhook bool      `db:"delivery_webhook" json:"delivery_webhook"`
	WebhookURL      *string   `db:"webhook_url"      json:"webhook_url"`
	NotifTitle      string    `db:"notif_title"      json:"notif_title"`
	NotifBody       string    `db:"notif_body"       json:"notif_body"`
	CreatedAt       time.Time `db:"created_at"       json:"created_at"`
	UpdatedAt       time.Time `db:"updated_at"       json:"updated_at"`
}

// RuleExecution logs a single delivery attempt for an alert rule.
type RuleExecution struct {
	ID       string    `db:"id"       json:"id"`
	RuleID   string    `db:"rule_id"  json:"rule_id"`
	EventID  string    `db:"event_id" json:"event_id"`
	FiredAt  time.Time `db:"fired_at" json:"fired_at"`
	Delivery string    `db:"delivery" json:"delivery"` // email | push | webhook
	Success  bool      `db:"success"  json:"success"`
	Error    *string   `db:"error"    json:"error,omitempty"`
}

// RuleCondition is a single condition in a rule's conditions array.
// Operator values: is | is_not | contains | does_not_contain
// Field values: display_text | severity | any key from event payload JSON
type RuleCondition struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}
