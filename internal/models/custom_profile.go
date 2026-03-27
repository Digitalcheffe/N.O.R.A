package models

import "time"

// CustomProfile is a user-created app profile stored in the database.
type CustomProfile struct {
	ID          string    `db:"id"           json:"id"`
	Name        string    `db:"name"         json:"name"`
	YAMLContent string    `db:"yaml_content" json:"yaml_content"`
	CreatedAt   time.Time `db:"created_at"   json:"created_at"`
}
