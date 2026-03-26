package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/digitalcheffe/nora/internal/ingest"
	"github.com/digitalcheffe/nora/internal/profile"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
)

const maxPayloadBytes = 1 << 20 // 1 MB

// HandleIngest handles POST /api/v1/ingest/{token}.
// This handler must be mounted outside the session auth middleware group —
// the token in the URL path is the only credential.
func HandleIngest(store *repo.Store, profiler profile.Loader, limiter *ingest.RateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := chi.URLParam(r, "token")

		// Enforce 1 MB body limit.
		r.Body = http.MaxBytesReader(w, r.Body, maxPayloadBytes)
		rawBody, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusRequestEntityTooLarge, "payload too large")
			return
		}

		// Payload must be valid JSON.
		if len(rawBody) == 0 || !json.Valid(rawBody) {
			writeError(w, http.StatusBadRequest, "payload must be valid JSON")
			return
		}

		result, err := ingest.Process(r.Context(), store, profiler, limiter, token, rawBody)
		if err != nil {
			var invalidToken ingest.ErrInvalidToken
			var rateLimited ingest.ErrRateLimited
			switch {
			case errors.As(err, &invalidToken):
				writeError(w, http.StatusUnauthorized, "invalid token")
			case errors.As(err, &rateLimited):
				writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			default:
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}

		writeJSON(w, http.StatusAccepted, map[string]string{
			"status": "accepted",
			"id":     result.EventID,
		})
	}
}
