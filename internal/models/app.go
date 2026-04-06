package models

import "time"

// App represents a monitored application registered in NORA.
type App struct {
	ID               string     `db:"id"                 json:"id"`
	Name             string     `db:"name"               json:"name"`
	Token            string     `db:"token"              json:"token"`
	ProfileID  string     `db:"profile_id" json:"profile_id"`
	Config     ConfigJSON `db:"config"     json:"config"`
	RateLimit        int        `db:"rate_limit"         json:"rate_limit"`
	CreatedAt        time.Time  `db:"created_at"         json:"created_at"`
}
