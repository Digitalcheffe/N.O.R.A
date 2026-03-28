package models

import (
	"database/sql/driver"
	"errors"
	"fmt"
)

// ConfigJSON is stored as a TEXT JSON string in SQLite but serialises as a
// plain JSON object (not a quoted string) when encoded to the API response.
//
// It implements sql.Scanner so sqlx can load it directly from a TEXT column,
// and driver.Valuer so it is stored back as TEXT.
type ConfigJSON []byte

// MarshalJSON outputs the raw bytes verbatim — no extra quoting.
func (c ConfigJSON) MarshalJSON() ([]byte, error) {
	if len(c) == 0 {
		return []byte("{}"), nil
	}
	return []byte(c), nil
}

// UnmarshalJSON captures the incoming JSON bytes as-is.
func (c *ConfigJSON) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("ConfigJSON: UnmarshalJSON on nil pointer")
	}
	*c = append((*c)[0:0], data...)
	return nil
}

// Scan implements sql.Scanner. SQLite returns TEXT columns as string; we
// accept both string and []byte.
func (c *ConfigJSON) Scan(src any) error {
	switch v := src.(type) {
	case string:
		*c = ConfigJSON(v)
	case []byte:
		dst := make([]byte, len(v))
		copy(dst, v)
		*c = ConfigJSON(dst)
	case nil:
		*c = ConfigJSON("{}")
	default:
		return fmt.Errorf("ConfigJSON: cannot scan type %T", src)
	}
	return nil
}

// Value implements driver.Valuer — stores the JSON as TEXT in SQLite.
func (c ConfigJSON) Value() (driver.Value, error) {
	if len(c) == 0 {
		return "{}", nil
	}
	return string(c), nil
}
