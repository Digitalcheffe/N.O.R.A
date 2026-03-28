-- Add skip_tls_verify column to monitor_checks.
-- When true, URL checks skip TLS certificate validation.
-- Useful for self-signed certificates on internal services.

ALTER TABLE monitor_checks ADD COLUMN skip_tls_verify INTEGER NOT NULL DEFAULT 0;
