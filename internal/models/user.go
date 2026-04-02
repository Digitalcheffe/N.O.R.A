package models

import "time"

// User represents an authenticated NORA user (no password hash — never expose it via API).
type User struct {
	ID          string    `db:"id"           json:"id"`
	Email       string    `db:"email"        json:"email"`
	Role        string    `db:"role"         json:"role"`
	CreatedAt   time.Time `db:"created_at"   json:"created_at"`
	TOTPEnabled bool `db:"totp_enabled" json:"totp_enabled"`
	TOTPGrace   bool `db:"totp_grace"   json:"totp_grace"`
	TOTPExempt  bool `db:"totp_exempt"  json:"totp_exempt"`
}
