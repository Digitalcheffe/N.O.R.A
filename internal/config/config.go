package config

import (
	"log"
	"os"
)

// NORA ships with a single persistent data directory — /data. Everything NORA
// writes (SQLite, app template copies, cached icons, VAPID keys) lives under
// that root. Operators bind-mount the host path they want; we don't expose
// the sub-paths as configuration since splitting them has no deploy benefit
// and adds surface area that can drift out of sync.
const (
	DefaultDBPath        = "/data/nora.db"
	DefaultTemplatesPath = "/data/templates"
	DefaultIconsPath     = "/data/icons"
)

type Config struct {
	Secret   string
	DBPath   string // kept as a field for tests that run against :memory: or a temp file
	Port     string
	LogLevel string // "debug" enables verbose request logging; default is minimal
	Timezone string // IANA timezone used by the digest scheduler (e.g. "America/New_York")
	VAPIDPublic  string
	VAPIDPrivate string
	VAPIDSubject string
	// Bootstrap admin credentials — used only when the users table is empty.
	AdminEmail    string
	AdminPassword string
}

func Load() *Config {
	if os.Getenv("NORA_DEV_MODE") != "" {
		log.Println("warning: NORA_DEV_MODE is no longer supported — auth is always required")
	}

	cfg := &Config{
		Secret:        os.Getenv("NORA_SECRET"),
		DBPath:        DefaultDBPath,
		Port:          getEnvStr("NORA_PORT", getEnvStr("PORT", "8081")),
		LogLevel:      getEnvStr("NORA_LOG_LEVEL", "info"),
		Timezone:      getEnvStr("NORA_TIMEZONE", "UTC"),
		VAPIDPublic:   os.Getenv("NORA_VAPID_PUBLIC"),
		VAPIDPrivate:  os.Getenv("NORA_VAPID_PRIVATE"),
		VAPIDSubject:  getEnvStr("NORA_VAPID_SUBJECT", "mailto:admin@localhost"),
		AdminEmail:    os.Getenv("NORA_ADMIN_EMAIL"),
		AdminPassword: os.Getenv("NORA_ADMIN_PASSWORD"),
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

