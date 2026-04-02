package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"
)

// GenerateTOTPSecret generates a cryptographically random 160-bit base32-encoded TOTP secret.
func GenerateTOTPSecret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw), nil
}

// TOTPUri builds an otpauth:// URI suitable for QR code display.
func TOTPUri(secret, account, issuer string) string {
	return fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s&algorithm=SHA1&digits=6&period=30",
		url.PathEscape(issuer),
		url.PathEscape(account),
		secret,
		url.QueryEscape(issuer),
	)
}

// ValidateTOTP returns true if code is a valid 6-digit TOTP for the given base32 secret.
// Accepts the previous, current, and next 30-second window (90-second clock-skew leniency).
func ValidateTOTP(secret, code string) bool {
	if len(code) != 6 {
		return false
	}
	normalized := strings.ToUpper(strings.ReplaceAll(secret, " ", ""))
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(normalized)
	if err != nil {
		return false
	}
	counter := time.Now().Unix() / 30
	for _, delta := range []int64{-1, 0, 1} {
		if totpCode(key, counter+delta) == code {
			return true
		}
	}
	return false
}

func totpCode(key []byte, counter int64) string {
	msg := make([]byte, 8)
	binary.BigEndian.PutUint64(msg, uint64(counter))
	h := hmac.New(sha1.New, key)
	h.Write(msg)
	sum := h.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", int(code)%int(math.Pow10(6)))
}
