package auth

import (
	"context"
	"net/http"
)

type contextKey string

const userIDKey contextKey = "userID"

// RequireAuth is an HTTP middleware that enforces authentication.
// When NORA_DEV_MODE=true a hardcoded admin session is injected — no login required.
// TODO(T-30): remove the dev-mode bypass before production release.
func RequireAuth(devMode bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if devMode {
				// Dev bypass: inject a hardcoded admin identity so every request
				// is treated as authenticated. Never ship this in a production image.
				ctx := context.WithValue(r.Context(), userIDKey, "dev-admin")
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// TODO(T-10): validate JWT/session cookie and populate context.
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		})
	}
}

// UserID returns the authenticated user ID stored in the request context.
func UserID(ctx context.Context) string {
	v, _ := ctx.Value(userIDKey).(string)
	return v
}
