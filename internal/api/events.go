package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
)

// EventsHandler holds dependencies for the events resource handlers.
type EventsHandler struct {
	events repo.EventRepo
}

// NewEventsHandler creates an EventsHandler with the given repository.
func NewEventsHandler(events repo.EventRepo) *EventsHandler {
	return &EventsHandler{events: events}
}

// Routes registers all event endpoints on r.
func (h *EventsHandler) Routes(r chi.Router) {
	r.Get("/events", h.List)
	r.Get("/events/timeseries", h.Timeseries) // must be before /events/{id}
	r.Get("/events/{id}", h.Get)
	r.Get("/apps/{id}/events", h.ListByApp)
}

// --- response types ---

// eventItem is the list-response shape: payload is excluded.
type eventItem struct {
	ID         string    `json:"id"`
	Level      string    `json:"level"`
	SourceName string    `json:"source_name"`
	SourceType string    `json:"source_type"`
	SourceID   string    `json:"source_id"`
	Title      string    `json:"title"`
	CreatedAt  time.Time `json:"created_at"`
}

// eventDetail is the single-event shape: includes payload as a JSON object.
type eventDetail struct {
	ID         string          `json:"id"`
	Level      string          `json:"level"`
	SourceName string          `json:"source_name"`
	SourceType string          `json:"source_type"`
	SourceID   string          `json:"source_id"`
	Title      string          `json:"title"`
	Payload    json.RawMessage `json:"payload"`
	CreatedAt  time.Time       `json:"created_at"`
}

// listEventsResponse wraps a page of events with pagination metadata.
type listEventsResponse struct {
	Data   []eventItem `json:"data"`
	Total  int         `json:"total"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
}

// timeseriesResponse wraps a slice of timeseries buckets.
type timeseriesResponse struct {
	Data []repo.TimeseriesBucket `json:"data"`
}

// rawOrEmpty returns the string as a RawMessage, falling back to {} for empty/null.
func rawOrEmpty(s string) json.RawMessage {
	if s == "" || s == "null" {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(s)
}

func toEventItem(e models.Event) eventItem {
	return eventItem{
		ID:         e.ID,
		Level:      e.Level,
		SourceName: e.SourceName,
		SourceType: e.SourceType,
		SourceID:   e.SourceID,
		Title:      e.Title,
		CreatedAt:  e.CreatedAt,
	}
}

func toEventDetail(e *models.Event) eventDetail {
	return eventDetail{
		ID:         e.ID,
		Level:      e.Level,
		SourceName: e.SourceName,
		SourceType: e.SourceType,
		SourceID:   e.SourceID,
		Title:      e.Title,
		Payload:    rawOrEmpty(e.Payload),
		CreatedAt:  e.CreatedAt,
	}
}

// parseFilter reads event filter query params from the request.
// Returns an error string (400) if any param is malformed.
func parseFilter(r *http.Request) (repo.ListFilter, error) {
	q := r.URL.Query()

	f := repo.ListFilter{
		SourceID:   q.Get("source_id"),
		SourceName: q.Get("source_name"),
		Limit:      50,
		Offset:     0,
	}

	// Accept source_type directly.
	if st := q.Get("source_type"); st != "" {
		f.SourceType = st
	}

	if s := q.Get("search"); s != "" {
		f.Search = s
	}

	if lv := q.Get("level"); lv != "" {
		for _, l := range strings.Split(lv, ",") {
			if l = strings.TrimSpace(l); l != "" {
				f.Level = append(f.Level, l)
			}
		}
	}

	if s := q.Get("since"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return f, fmt.Errorf("invalid since: must be RFC3339")
		}
		f.Since = &t
	}

	if s := q.Get("until"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return f, fmt.Errorf("invalid until: must be RFC3339")
		}
		f.Until = &t
	}

	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			return f, fmt.Errorf("invalid limit: must be a positive integer")
		}
		f.Limit = n
	}

	if s := q.Get("offset"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return f, fmt.Errorf("invalid offset: must be a non-negative integer")
		}
		f.Offset = n
	}

	if s := q.Get("sort"); s != "" {
		switch s {
		case "newest", "oldest", "level_desc", "level_asc":
			f.Sort = s
		}
	}

	return f, nil
}

// Timeseries returns event counts grouped by time bucket: GET /api/v1/events/timeseries
func (h *EventsHandler) Timeseries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	granularity := q.Get("granularity")
	if granularity != "hour" && granularity != "day" {
		granularity = "day"
	}

	var since, until time.Time
	var err error
	if s := q.Get("since"); s != "" {
		since, err = time.Parse(time.RFC3339, s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid since: must be RFC3339")
			return
		}
	} else {
		since = time.Now().UTC().Add(-7 * 24 * time.Hour)
	}
	if s := q.Get("until"); s != "" {
		until, err = time.Parse(time.RFC3339, s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid until: must be RFC3339")
			return
		}
	} else {
		until = time.Now().UTC()
	}

	sourceID := q.Get("source_id")
	level := q.Get("level")

	buckets, err := h.events.Timeseries(r.Context(), since, until, granularity, sourceID, level)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, timeseriesResponse{Data: buckets})
}

// --- handlers ---

// List returns a filtered page of events: GET /api/v1/events
//
// @Summary      List events
// @Description  Returns a filtered, paginated list of events across all sources. payload is excluded.
// @Tags         events
// @Produce      json
// @Param        source_id    query  string  false  "Filter by source ID (app ID, check ID, component ID, etc.)"
// @Param        source_type  query  string  false  "Filter by source type: app, physical_host, virtual_host, docker_engine, monitor_check, system"
// @Param        source_name  query  string  false  "Partial match on source name (case-insensitive)"
// @Param        level        query  string  false  "Comma-separated level filter: debug,info,warn,error,critical"
// @Param        since        query  string  false  "ISO8601 lower bound (inclusive) e.g. 2026-01-01T00:00:00Z"
// @Param        until        query  string  false  "ISO8601 upper bound (inclusive) e.g. 2026-12-31T23:59:59Z"
// @Param        limit        query  int     false  "Page size (default 50, max 500)"
// @Param        offset       query  int     false  "Pagination offset (default 0)"
// @Success      200  {object}  listEventsResponse
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerToken
// @Router       /events [get]
func (h *EventsHandler) List(w http.ResponseWriter, r *http.Request) {
	f, err := parseFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	events, total, err := h.events.List(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	items := make([]eventItem, len(events))
	for i, e := range events {
		items[i] = toEventItem(e)
	}
	writeJSON(w, http.StatusOK, listEventsResponse{
		Data:   items,
		Total:  total,
		Limit:  f.Limit,
		Offset: f.Offset,
	})
}

// Get returns a single event with payload: GET /api/v1/events/{id}
//
// @Summary      Get an event
// @Description  Returns a single event by ID. Includes payload as a parsed JSON object.
// @Tags         events
// @Produce      json
// @Param        id   path      string  true  "Event ID"
// @Success      200  {object}  eventDetail
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerToken
// @Router       /events/{id} [get]
func (h *EventsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ev, err := h.events.Get(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "event not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toEventDetail(ev))
}

// ListByApp returns events scoped to a single app: GET /api/v1/apps/{id}/events
//
// @Summary      List events for an app
// @Description  Returns a filtered, paginated list of events for a specific app. Same query params as /events.
// @Tags         events
// @Produce      json
// @Param        id        path   string  true   "App ID"
// @Param        level     query  string  false  "Comma-separated level filter"
// @Param        since     query  string  false  "ISO8601 lower bound"
// @Param        until     query  string  false  "ISO8601 upper bound"
// @Param        limit     query  int     false  "Page size (default 50, max 500)"
// @Param        offset    query  int     false  "Pagination offset"
// @Success      200  {object}  listEventsResponse
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerToken
// @Router       /apps/{id}/events [get]
func (h *EventsHandler) ListByApp(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "id")
	f, err := parseFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	f.SourceID = appID   // override with path param
	f.SourceType = "app" // scope to app events only

	events, total, err := h.events.List(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	items := make([]eventItem, len(events))
	for i, e := range events {
		items[i] = toEventItem(e)
	}
	writeJSON(w, http.StatusOK, listEventsResponse{
		Data:   items,
		Total:  total,
		Limit:  f.Limit,
		Offset: f.Offset,
	})
}
