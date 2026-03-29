package models

import "time"

// WebPushSubscription represents a browser push subscription stored in the DB.
type WebPushSubscription struct {
	ID        string    `db:"id"         json:"id"`
	UserID    string    `db:"user_id"    json:"user_id"`
	Endpoint  string    `db:"endpoint"   json:"endpoint"`
	P256DH    string    `db:"p256dh"     json:"p256dh"`
	Auth      string    `db:"auth"       json:"auth"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
