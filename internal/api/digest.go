package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/digitalcheffe/nora/internal/jobs"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
)

const digestScheduleKey = "digest_schedule"

// DigestHandler handles digest schedule configuration and manual triggers.
type DigestHandler struct {
	store     *repo.Store
	digestJob *jobs.DigestJob
}

// NewDigestHandler creates a DigestHandler.
func NewDigestHandler(store *repo.Store, digestJob *jobs.DigestJob) *DigestHandler {
	return &DigestHandler{store: store, digestJob: digestJob}
}

// Routes registers the digest endpoints.
func (h *DigestHandler) Routes(r chi.Router) {
	r.Get("/digest/schedule", h.GetSchedule)
	r.Put("/digest/schedule", h.PutSchedule)
	r.Post("/digest/send-now", h.SendNow)
	r.Get("/digest/report", h.GetReport)
}

// GetSchedule returns the current digest schedule: GET /api/v1/digest/schedule
func (h *DigestHandler) GetSchedule(w http.ResponseWriter, r *http.Request) {
	var sched models.DigestSchedule
	err := h.store.Settings.GetJSON(r.Context(), digestScheduleKey, &sched)
	if errors.Is(err, repo.ErrNotFound) {
		sched = models.DigestSchedule{Frequency: "monthly", DayOfWeek: 1, DayOfMonth: 1}
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sched)
}

// PutSchedule stores an updated digest schedule: PUT /api/v1/digest/schedule
func (h *DigestHandler) PutSchedule(w http.ResponseWriter, r *http.Request) {
	var req models.DigestSchedule
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate.
	switch req.Frequency {
	case "daily", "weekly", "monthly":
	default:
		writeError(w, http.StatusBadRequest, "frequency must be one of: daily, weekly, monthly")
		return
	}
	if req.DayOfWeek < 0 || req.DayOfWeek > 6 {
		writeError(w, http.StatusBadRequest, "day_of_week must be 0–6")
		return
	}
	if req.DayOfMonth < 1 || req.DayOfMonth > 28 {
		writeError(w, http.StatusBadRequest, "day_of_month must be 1–28")
		return
	}
	if req.SendHour != nil && (*req.SendHour < 0 || *req.SendHour > 23) {
		writeError(w, http.StatusBadRequest, "send_hour must be 0–23")
		return
	}

	if !h.digestJob.SMTPConfigured(r.Context()) {
		writeError(w, http.StatusUnprocessableEntity, "SMTP is not configured — set up SMTP before enabling the digest schedule")
		return
	}

	if err := h.store.Settings.SetJSON(r.Context(), digestScheduleKey, req); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, req)
}

// SendNow triggers an immediate digest for the current period: POST /api/v1/digest/send-now
func (h *DigestHandler) SendNow(w http.ResponseWriter, r *http.Request) {
	if !h.digestJob.SMTPConfigured(r.Context()) {
		writeError(w, http.StatusUnprocessableEntity, "SMTP is not configured — set up SMTP before sending a digest")
		return
	}

	var sched models.DigestSchedule
	err := h.store.Settings.GetJSON(r.Context(), digestScheduleKey, &sched)
	if errors.Is(err, repo.ErrNotFound) {
		sched = models.DigestSchedule{Frequency: "monthly", DayOfWeek: 1, DayOfMonth: 1}
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	period := periodLabelFromSchedule(sched.Frequency, time.Now())

	// Run in background — send-now is fire-and-forget from the HTTP perspective.
	go func() {
		if err := h.digestJob.Send(r.Context(), period); err != nil {
			_ = err
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status": "queued",
		"period": period,
	})
}

// GetReport renders a print-friendly HTML digest report: GET /api/v1/digest/report
// Optional query param: ?period=2026-03 (defaults to the current period).
func (h *DigestHandler) GetReport(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	if period == "" {
		var sched models.DigestSchedule
		err := h.store.Settings.GetJSON(r.Context(), digestScheduleKey, &sched)
		if errors.Is(err, repo.ErrNotFound) {
			sched = models.DigestSchedule{Frequency: "monthly"}
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		period = periodLabelFromSchedule(sched.Frequency, time.Now())
	}

	html, err := h.digestJob.GenerateReportHTML(r.Context(), period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, html)
}

// periodLabelFromSchedule returns the period label for the current date given a frequency.
func periodLabelFromSchedule(frequency string, t time.Time) string {
	switch frequency {
	case "daily":
		return t.Format("2006-01-02")
	case "weekly":
		_, week := t.ISOWeek()
		return t.Format("2006") + "-W" + fmt.Sprintf("%02d", week)
	default:
		return t.Format("2006-01")
	}
}
