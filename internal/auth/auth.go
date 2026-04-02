package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const tokenExpiry = 24 * time.Hour
const mfaTokenExpiry = 15 * time.Minute

// Claims holds the authenticated user identity embedded in a JWT.
type Claims struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// GenerateToken creates a signed JWT for the given user. The token expires in 24 hours.
func GenerateToken(userID string, role string, secret string) (string, error) {
	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(tokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// MFAClaims holds a user identity for the pending MFA verification step.
// It is short-lived and distinct from a full session token.
type MFAClaims struct {
	UserID  string `json:"user_id"`
	Pending bool   `json:"mfa_pending"`
	jwt.RegisteredClaims
}

// GenerateMFAToken creates a 15-minute JWT used for the TOTP second-step flow.
func GenerateMFAToken(userID string, secret string) (string, error) {
	claims := MFAClaims{
		UserID:  userID,
		Pending: true,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(mfaTokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidateMFAToken parses and validates a pending-MFA JWT.
func ValidateMFAToken(tokenStr string, secret string) (*MFAClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &MFAClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*MFAClaims)
	if !ok || !token.Valid || !claims.Pending {
		return nil, errors.New("invalid MFA token")
	}
	return claims, nil
}

// ValidateToken parses and validates a JWT, returning the embedded Claims on success.
func ValidateToken(tokenStr string, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
