package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// JSONText is a json.RawMessage that can be scanned from a SQLite TEXT column.
type JSONText json.RawMessage

func (j *JSONText) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		*j = JSONText(v)
	case []byte:
		*j = JSONText(v)
	case nil:
		*j = JSONText("{}")
	default:
		return fmt.Errorf("JSONText: unsupported type %T", src)
	}
	return nil
}

func (j JSONText) Value() (driver.Value, error) {
	if j == nil {
		return "{}", nil
	}
	return string(j), nil
}

func (j JSONText) MarshalJSON() ([]byte, error) {
	if j == nil {
		return []byte("{}"), nil
	}
	return json.RawMessage(j).MarshalJSON()
}

// DigestRegistryEntry is a named, managed record for every digest entry declared
// across all app profiles. Entries are reconciled at startup and managed via
// the Settings → Digest Registry tab.
type DigestRegistryEntry struct {
	ID            string    `db:"id"             json:"id"`
	ProfileID     string    `db:"profile_id"     json:"profile_id"`
	Source        string    `db:"source"         json:"source"`
	EntryType     string    `db:"entry_type"     json:"entry_type"`
	Name          string    `db:"name"           json:"name"`
	Label         string    `db:"label"          json:"label"`
	Config        JSONText  `db:"config"         json:"config"`
	ProfileSource string    `db:"profile_source" json:"profile_source"`
	Active        bool      `db:"active"         json:"active"`
	CreatedAt     time.Time `db:"created_at"     json:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"     json:"updated_at"`
}
