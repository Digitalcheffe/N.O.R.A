package auth

import (
	"context"
	"net/http"
)

type contextKey string

const (
	UserIDKey contextKey = "userID"
	RoleKey   contextKey = "role"
)

// DEV MODE BYPASS — remove in T-30 when real auth is implemented
// When NORA_DEV_MODE=true this middleware injects a hardcoded admin session
// so the app is fully usable without logging in during development.
func DevModeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), UserIDKey, "dev-admin")
		ctx = context.WithValue(ctx, RoleKey, "admin")
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAuthMiddleware returns 401 when real auth is not yet implemented.
// Replaced by real JWT/session validation in T-30.
func RequireAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	})
}
