package models

import "time"

// User represents an authenticated NORA user (no password hash — never expose it via API).
type User struct {
	ID        string    `db:"id"         json:"id"`
	Email     string    `db:"email"      json:"email"`
	Role      string    `db:"role"       json:"role"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
