package models

// DigestSchedule holds the user-configured digest delivery schedule.
// It is stored in the settings table under key "digest_schedule" as JSON.
type DigestSchedule struct {
	// Frequency is one of: "daily", "weekly", "monthly".
	Frequency string `json:"frequency"`
	// DayOfWeek is 0–6 (0 = Sunday). Only used when Frequency = "weekly".
	DayOfWeek int `json:"day_of_week"`
	// DayOfMonth is 1–28. Only used when Frequency = "monthly".
	DayOfMonth int `json:"day_of_month"`
	// SendHour is 0–23 in the server timezone (NORA_TIMEZONE). Nil means use the default (17).
	SendHour *int `json:"send_hour,omitempty"`
	// Timezone is populated by the server on reads only; it reflects the NORA_TIMEZONE setting
	// so the UI can display which timezone send_hour applies to. Not persisted.
	Timezone string `json:"timezone,omitempty"`
}

// EffectiveSendHour returns the configured send hour, defaulting to 17 when unset.
func (s DigestSchedule) EffectiveSendHour() int {
	if s.SendHour == nil {
		return 17
	}
	return *s.SendHour
}

// SMTPSettings holds the outbound mail server configuration.
// It is stored in the settings table under key "smtp" as JSON.
type SMTPSettings struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	User string `json:"user"`
	Pass string `json:"pass"`
	From string `json:"from"`
	To   string `json:"to"`
}

// PasswordPolicy defines password complexity requirements enforced at every
// password entry point (create user, change password, registration).
// It is stored in the settings table under key "password_policy" as JSON.
type PasswordPolicy struct {
	MinLength        int  `json:"min_length"`
	RequireUppercase bool `json:"require_uppercase"`
	RequireNumber    bool `json:"require_number"`
	RequireSpecial   bool `json:"require_special"`
}

// DefaultPasswordPolicy returns sensible defaults (min 8 chars, no other rules).
func DefaultPasswordPolicy() PasswordPolicy {
	return PasswordPolicy{MinLength: 8}
}
