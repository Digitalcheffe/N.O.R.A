package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/digitalcheffe/nora/internal/auth"
	"github.com/digitalcheffe/nora/internal/config"
	"github.com/digitalcheffe/nora/internal/push"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
)

// PushHandler serves Web Push subscription and notification endpoints.
type PushHandler struct {
	cfg    *config.Config
	store  *repo.Store
	sender *push.Sender
}

// NewPushHandler creates a PushHandler.
func NewPushHandler(cfg *config.Config, store *repo.Store, sender *push.Sender) *PushHandler {
	return &PushHandler{cfg: cfg, store: store, sender: sender}
}

// Routes registers push endpoints on r.
// The vapid-public-key endpoint is deliberately unauthenticated; the other
// routes are expected to be mounted inside an authenticated router group.
func (h *PushHandler) Routes(r chi.Router) {
	r.Get("/push/vapid-public-key", h.GetVAPIDPublicKey)
	r.Post("/push/subscribe", h.Subscribe)
	r.Delete("/push/subscribe", h.Unsubscribe)
	r.Post("/push/test", h.Test)
}

// RegisterPublicRoutes registers the unauthenticated vapid-public-key route on r.
func (h *PushHandler) RegisterPublicRoutes(r chi.Router) {
	r.Get("/api/v1/push/vapid-public-key", h.GetVAPIDPublicKey)
}

// --- request / response types ---

type subscribeRequest struct {
	Endpoint string          `json:"endpoint"`
	Keys     subscribeKeys   `json:"keys"`
}

type subscribeKeys struct {
	P256DH string `json:"p256dh"`
	Auth   string `json:"auth"`
}

// --- handlers ---

// GetVAPIDPublicKey returns the VAPID public key: GET /api/v1/push/vapid-public-key
// Unauthenticated — the service worker needs this before the user logs in.
func (h *PushHandler) GetVAPIDPublicKey(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"public_key": h.cfg.VAPIDPublic})
}

// Subscribe saves a push subscription: POST /api/v1/push/subscribe
func (h *PushHandler) Subscribe(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req subscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Endpoint == "" {
		writeError(w, http.StatusBadRequest, "endpoint is required")
		return
	}
	if req.Keys.P256DH == "" || req.Keys.Auth == "" {
		writeError(w, http.StatusBadRequest, "keys.p256dh and keys.auth are required")
		return
	}

	sub, err := h.store.WebPushSubscriptions.Save(r.Context(), userID, req.Endpoint, req.Keys.P256DH, req.Keys.Auth)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, sub)
}

// Unsubscribe removes a push subscription: DELETE /api/v1/push/subscribe
func (h *PushHandler) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req subscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Endpoint == "" {
		writeError(w, http.StatusBadRequest, "endpoint is required")
		return
	}

	err := h.store.WebPushSubscriptions.DeleteByUserAndEndpoint(r.Context(), userID, req.Endpoint)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusNotFound, "subscription not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Test sends a test notification to the current user: POST /api/v1/push/test
func (h *PushHandler) Test(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := h.sender.SendToUser(r.Context(), userID, "NORA Test", "Push notifications are working!"); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}
