-- Add TOTP (time-based one-time password) fields to the users table.
-- totp_secret:  base32-encoded TOTP secret; NULL until setup is initiated.
-- totp_enabled: 1 once the user has confirmed their first TOTP code.
-- totp_grace:   1 when the user has a grace login available (global MFA on, not yet enrolled).
-- totp_exempt:  1 for the bootstrap admin; never subject to global MFA enforcement.
ALTER TABLE users ADD COLUMN totp_secret  TEXT;
ALTER TABLE users ADD COLUMN totp_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN totp_grace   INTEGER NOT NULL DEFAULT 1;
ALTER TABLE users ADD COLUMN totp_exempt  INTEGER NOT NULL DEFAULT 0;
