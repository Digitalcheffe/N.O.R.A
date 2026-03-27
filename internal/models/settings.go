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
}

// SMTPSettings holds the outbound mail server configuration.
// It is stored in the settings table under key "smtp" as JSON.
type SMTPSettings struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	User string `json:"user"`
	Pass string `json:"pass"`
	From string `json:"from"`
}
