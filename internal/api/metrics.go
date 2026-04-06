package api

import (
	"net/http"
	"os"
	"time"

	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
)

// MetricsHandler serves instance-wide and per-app metrics.
type MetricsHandler struct {
	events  repo.EventRepo
	apps    repo.AppRepo
	metrics repo.MetricsRepo
	dbPath  string
	started time.Time
}

// NewMetricsHandler creates a MetricsHandler.
func NewMetricsHandler(events repo.EventRepo, apps repo.AppRepo, metrics repo.MetricsRepo, dbPath string, started time.Time) *MetricsHandler {
	return &MetricsHandler{
		events:  events,
		apps:    apps,
		metrics: metrics,
		dbPath:  dbPath,
		started: started,
	}
}

// Routes registers metrics endpoints on r.
func (h *MetricsHandler) Routes(r chi.Router) {
	r.Get("/metrics", h.GetInstance)
	// GET /apps/{id}/metrics is now handled by AppsHandler (returns api_polling snapshots).
}

// --- response types ---

type topAppItem struct {
	AppID         string `json:"app_id"`
	AppName       string `json:"app_name"`
	EventsPerHour int    `json:"events_per_hour"`
}

type appEventItem struct {
	AppID   string `json:"app_id"`
	AppName string `json:"app_name"`
	Count   int    `json:"count"`
}

type instanceMetricsResponse struct {
	DBSizeBytes   int64          `json:"db_size_bytes"`
	EventsLast24h int            `json:"events_last_24h"`
	UptimeSeconds int64          `json:"uptime_seconds"`
	TopApps       []topAppItem   `json:"top_apps"`
	AppEvents24h  []appEventItem `json:"app_events_24h"`
}

// GetInstance returns instance-wide metrics: GET /api/v1/metrics
func (h *MetricsHandler) GetInstance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// DB file size
	var dbSize int64
	if info, err := os.Stat(h.dbPath); err == nil {
		dbSize = info.Size()
	}

	// Total events in last 24 hours
	since24h := time.Now().Add(-24 * time.Hour)
	perApp, err := h.events.CountPerApp(ctx, since24h)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var total24h int
	appEvents := make([]appEventItem, 0, len(perApp))
	for _, row := range perApp {
		total24h += row.Count
		appEvents = append(appEvents, appEventItem{
			AppID:   row.AppID,
			AppName: row.AppName,
			Count:   row.Count,
		})
	}

	// Top apps by most-recent hourly rate
	tops, err := h.metrics.TopApps(ctx, 10)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	topItems := make([]topAppItem, 0, len(tops))
	for _, t := range tops {
		topItems = append(topItems, topAppItem{
			AppID:         t.AppID,
			AppName:       t.AppName,
			EventsPerHour: t.EventsPerHour,
		})
	}

	writeJSON(w, http.StatusOK, instanceMetricsResponse{
		DBSizeBytes:   dbSize,
		EventsLast24h: total24h,
		UptimeSeconds: int64(time.Since(h.started).Seconds()),
		TopApps:       topItems,
		AppEvents24h:  appEvents,
	})
}

// appMetricItem is the per-app metric trend point returned by GetByApp.
type appMetricItem struct {
	AppID           string `json:"app_id"`
	Period          string `json:"period"`
	EventsPerHour   int    `json:"events_per_hour"`
	AvgPayloadBytes int    `json:"avg_payload_bytes"`
	PeakPerMinute   int    `json:"peak_per_minute"`
}

// GetByApp returns per-app metrics trend (last 24 points): GET /api/v1/apps/{id}/metrics
func (h *MetricsHandler) GetByApp(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "id")
	ctx := r.Context()

	// Verify app exists.
	if _, err := h.apps.Get(ctx, appID); err != nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}

	rows, err := h.metrics.ListByApp(ctx, appID, 24)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	items := make([]appMetricItem, 0, len(rows))
	for _, m := range rows {
		items = append(items, appMetricItem{
			AppID:           m.AppID,
			Period:          m.Period.UTC().Format(time.RFC3339),
			EventsPerHour:   m.EventsPerHour,
			AvgPayloadBytes: m.AvgPayloadBytes,
			PeakPerMinute:   m.PeakPerMinute,
		})
	}

	writeJSON(w, http.StatusOK, struct {
		Data  []appMetricItem `json:"data"`
		Total int             `json:"total"`
	}{Data: items, Total: len(items)})
}
