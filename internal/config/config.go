package config

import (
	"log"
	"os"
	"strconv"
)

type Config struct {
	DevMode        bool
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
}

func Load() *Config {
	cfg := &Config{
		DevMode:        getEnvBool("NORA_DEV_MODE", false),
		Secret:         os.Getenv("NORA_SECRET"),
		DBPath:         getEnvStr("NORA_DB_PATH", "/data/nora.db"),
		Port:           getEnvStr("NORA_PORT", "6000"),
		SMTPHost:       os.Getenv("NORA_SMTP_HOST"),
		SMTPPort:       getEnvInt("NORA_SMTP_PORT", 587),
		SMTPUser:       os.Getenv("NORA_SMTP_USER"),
		SMTPPass:       os.Getenv("NORA_SMTP_PASS"),
		SMTPFrom:       os.Getenv("NORA_SMTP_FROM"),
		DigestSchedule: getEnvStr("NORA_DIGEST_SCHEDULE", "0 8 1 * *"),
		VAPIDPublic:    os.Getenv("NORA_VAPID_PUBLIC"),
		VAPIDPrivate:   os.Getenv("NORA_VAPID_PRIVATE"),
	}

	if !cfg.DevMode && cfg.Secret == "" {
		log.Fatal("NORA_SECRET is required when NORA_DEV_MODE is false")
	}

	return cfg
}

func getEnvStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
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
