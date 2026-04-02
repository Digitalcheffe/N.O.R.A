package config

import (
	"log"
	"os"
)

type Config struct {
	Secret         string
	DBPath         string
	Port           string
	LogLevel       string // "debug" enables verbose request logging; default is minimal
	DigestSchedule string
	Timezone       string // IANA timezone name used for digest scheduling (e.g. "America/New_York")
	VAPIDPublic    string
	VAPIDPrivate   string
	TemplatesPath  string
	IconsPath      string
	// Bootstrap admin credentials — used only when the users table is empty.
	AdminEmail    string
	AdminPassword string
}

func Load() *Config {
	if os.Getenv("NORA_DEV_MODE") != "" {
		log.Println("warning: NORA_DEV_MODE is no longer supported — auth is always required")
	}

	cfg := &Config{
		Secret:         os.Getenv("NORA_SECRET"),
		DBPath:         getEnvStr("NORA_DB_PATH", "/data/nora.db"),
		TemplatesPath:  getEnvStr("NORA_TEMPLATES_PATH", "/data/templates"),
		IconsPath:      getEnvStr("NORA_ICONS_PATH", "/data/icons"),
		Port:           getEnvStr("NORA_PORT", "8081"),
		LogLevel:       getEnvStr("NORA_LOG_LEVEL", "info"),
		DigestSchedule: getEnvStr("NORA_DIGEST_SCHEDULE", "0 8 1 * *"),
		Timezone:       getEnvStr("NORA_TIMEZONE", "UTC"),
		VAPIDPublic:    os.Getenv("NORA_VAPID_PUBLIC"),
		VAPIDPrivate:   os.Getenv("NORA_VAPID_PRIVATE"),
		AdminEmail:     os.Getenv("NORA_ADMIN_EMAIL"),
		AdminPassword:  os.Getenv("NORA_ADMIN_PASSWORD"),
	}

	if cfg.Secret == "" {
		log.Fatal("NORA_SECRET is required")
	}

	return cfg
}

// IsDebug returns true when NORA_LOG_LEVEL=debug.
func (c *Config) IsDebug() bool {
	return c.LogLevel == "debug"
}

func getEnvStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

