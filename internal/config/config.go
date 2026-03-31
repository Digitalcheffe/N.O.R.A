package config

import (
	"log"
	"os"
	"strconv"
)

type Config struct {
	Secret         string
	DBPath         string
	Port           string
	SMTPHost       string
	SMTPPort       int
	SMTPUser       string
	SMTPPass       string
	SMTPFrom       string
	DigestSchedule string
	VAPIDPublic    string
	VAPIDPrivate   string
	TemplatesPath string
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
		Port:           getEnvStr("NORA_PORT", "8081"),
		SMTPHost:       os.Getenv("NORA_SMTP_HOST"),
		SMTPPort:       getEnvInt("NORA_SMTP_PORT", 587),
		SMTPUser:       os.Getenv("NORA_SMTP_USER"),
		SMTPPass:       os.Getenv("NORA_SMTP_PASS"),
		SMTPFrom:       os.Getenv("NORA_SMTP_FROM"),
		DigestSchedule: getEnvStr("NORA_DIGEST_SCHEDULE", "0 8 1 * *"),
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

func getEnvStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}
