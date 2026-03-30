package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/digitalcheffe/nora/internal/api"
	"github.com/digitalcheffe/nora/internal/config"
	"github.com/digitalcheffe/nora/internal/jobs"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
)

func newDigestRouter(t *testing.T) (http.Handler, *repo.Store) {
	t.Helper()
	db := newTestDB(t)
	store := repo.NewStore(
		repo.NewAppRepo(db),
		repo.NewEventRepo(db),
		repo.NewCheckRepo(db),
		repo.NewRollupRepo(db),
		repo.NewResourceReadingRepo(db),
		repo.NewResourceRollupRepo(db),
		repo.NewInfraComponentRepo(db),
		repo.NewDockerEngineRepo(db),
		repo.NewInfraRepo(db),
		repo.NewSettingsRepo(db),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	digestJob := jobs.NewDigestJob(store, &config.Config{})
	h := api.NewDigestHandler(store, digestJob)
	r := chi.NewRouter()
	h.Routes(r)
	return r, store
}

// newDigestRouterWithSMTP creates a digest router with SMTP seeded into settings
// so that SMTP-gated endpoints (PUT schedule, POST send-now) are accessible.
func newDigestRouterWithSMTP(t *testing.T) (http.Handler, *repo.Store) {
	t.Helper()
	cfg := &config.Config{
		SMTPHost: "smtp.example.com",
		SMTPPort: 587,
		SMTPFrom: "nora@example.com",
	}
	// Re-create the handler with SMTP available via config fallback.
	db := newTestDB(t)
	store2 := repo.NewStore(
		repo.NewAppRepo(db),
		repo.NewEventRepo(db),
		repo.NewCheckRepo(db),
		repo.NewRollupRepo(db),
		repo.NewResourceReadingRepo(db),
		repo.NewResourceRollupRepo(db),
		repo.NewInfraComponentRepo(db),
		repo.NewDockerEngineRepo(db),
		repo.NewInfraRepo(db),
		repo.NewSettingsRepo(db),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	digestJob2 := jobs.NewDigestJob(store2, cfg)
	h2 := api.NewDigestHandler(store2, digestJob2)
	r2 := chi.NewRouter()
	h2.Routes(r2)
	return r2, store2
}

// --- GET /digest/schedule ---

func TestGetDigestSchedule_Default(t *testing.T) {
	r, _ := newDigestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/digest/schedule", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rr.Code, rr.Body.String())
	}

	var sched models.DigestSchedule
	if err := json.NewDecoder(rr.Body).Decode(&sched); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if sched.Frequency != "monthly" {
		t.Errorf("expected default frequency=monthly, got %s", sched.Frequency)
	}
	if sched.DayOfMonth != 1 {
		t.Errorf("expected default day_of_month=1, got %d", sched.DayOfMonth)
	}
}

// --- PUT /digest/schedule ---

func TestPutDigestSchedule_NoSMTP(t *testing.T) {
	r, _ := newDigestRouter(t)
	body, _ := json.Marshal(models.DigestSchedule{Frequency: "daily", DayOfWeek: 1, DayOfMonth: 1})
	req := httptest.NewRequest(http.MethodPut, "/digest/schedule", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPutDigestSchedule_Happy(t *testing.T) {
	r, _ := newDigestRouterWithSMTP(t)

	body, _ := json.Marshal(models.DigestSchedule{
		Frequency:  "weekly",
		DayOfWeek:  2,
		DayOfMonth: 1,
	})
	req := httptest.NewRequest(http.MethodPut, "/digest/schedule", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rr.Code, rr.Body.String())
	}

	var got models.DigestSchedule
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Frequency != "weekly" || got.DayOfWeek != 2 {
		t.Errorf("unexpected response: %+v", got)
	}
}

func TestPutDigestSchedule_InvalidFrequency(t *testing.T) {
	r, _ := newDigestRouterWithSMTP(t)

	body, _ := json.Marshal(models.DigestSchedule{Frequency: "hourly"})
	req := httptest.NewRequest(http.MethodPut, "/digest/schedule", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", rr.Code)
	}
}

func TestPutDigestSchedule_InvalidDayOfWeek(t *testing.T) {
	r, _ := newDigestRouterWithSMTP(t)

	body, _ := json.Marshal(map[string]any{
		"frequency":    "weekly",
		"day_of_week":  7,
		"day_of_month": 1,
	})
	req := httptest.NewRequest(http.MethodPut, "/digest/schedule", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPutDigestSchedule_InvalidDayOfMonth(t *testing.T) {
	r, _ := newDigestRouterWithSMTP(t)

	tests := []struct {
		day  int
		want int
	}{
		{0, http.StatusBadRequest},
		{29, http.StatusBadRequest},
		{1, http.StatusOK},
		{28, http.StatusOK},
	}
	for _, tc := range tests {
		body, _ := json.Marshal(map[string]any{
			"frequency":    "monthly",
			"day_of_week":  1,
			"day_of_month": tc.day,
		})
		req := httptest.NewRequest(http.MethodPut, "/digest/schedule", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != tc.want {
			t.Errorf("day_of_month=%d: expected %d got %d: %s", tc.day, tc.want, rr.Code, rr.Body.String())
		}
	}
}

func TestPutDigestSchedule_InvalidBody(t *testing.T) {
	r, _ := newDigestRouterWithSMTP(t)

	req := httptest.NewRequest(http.MethodPut, "/digest/schedule", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", rr.Code)
	}
}

func TestPutDigestSchedule_InvalidSendHour(t *testing.T) {
	r, _ := newDigestRouterWithSMTP(t)

	tests := []struct {
		hour int
		want int
	}{
		{-1, http.StatusBadRequest},
		{24, http.StatusBadRequest},
		{0, http.StatusOK},
		{8, http.StatusOK},
		{23, http.StatusOK},
	}
	for _, tc := range tests {
		h := tc.hour
		body, _ := json.Marshal(map[string]any{
			"frequency":    "daily",
			"day_of_week":  1,
			"day_of_month": 1,
			"send_hour":    h,
		})
		req := httptest.NewRequest(http.MethodPut, "/digest/schedule", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != tc.want {
			t.Errorf("send_hour=%d: expected %d got %d: %s", tc.hour, tc.want, rr.Code, rr.Body.String())
		}
	}
}

// --- POST /digest/send-now ---

func TestSendNow_NoSMTP(t *testing.T) {
	r, _ := newDigestRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/digest/send-now", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSendNow_Accepted(t *testing.T) {
	r, _ := newDigestRouterWithSMTP(t)

	req := httptest.NewRequest(http.MethodPost, "/digest/send-now", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Should return 202 immediately (goroutine handles actual send)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202 got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "queued" {
		t.Errorf("expected status=queued, got %s", resp["status"])
	}
	if resp["period"] == "" {
		t.Errorf("expected non-empty period in response")
	}
}

// --- GET /digest/report ---

func TestGetReport(t *testing.T) {
	r, _ := newDigestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/digest/report", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rr.Code, rr.Body.String())
	}
	ct := rr.Header().Get("Content-Type")
	if ct == "" || len(ct) < 9 || ct[:9] != "text/html" {
		t.Errorf("expected text/html content-type, got %s", ct)
	}
	body := rr.Body.String()
	if len(body) == 0 {
		t.Error("expected non-empty HTML body")
	}
	// Must contain the print button.
	if !stringContains(body, "Print / Save as PDF") {
		t.Error("report HTML missing print button")
	}
}

func TestGetReport_WithPeriod(t *testing.T) {
	r, _ := newDigestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/digest/report?period=2026-03", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rr.Code, rr.Body.String())
	}
}

func stringContains(s, sub string) bool {
	return len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}()
}

// --- GET then PUT roundtrip ---

func TestDigestScheduleRoundtrip(t *testing.T) {
	r, _ := newDigestRouterWithSMTP(t)

	// Set a daily schedule.
	body, _ := json.Marshal(models.DigestSchedule{
		Frequency:  "daily",
		DayOfWeek:  0,
		DayOfMonth: 1,
	})
	putReq := httptest.NewRequest(http.MethodPut, "/digest/schedule", bytes.NewReader(body))
	putReq.Header.Set("Content-Type", "application/json")
	putRR := httptest.NewRecorder()
	r.ServeHTTP(putRR, putReq)
	if putRR.Code != http.StatusOK {
		t.Fatalf("PUT: expected 200 got %d", putRR.Code)
	}

	// Read it back.
	getReq := httptest.NewRequest(http.MethodGet, "/digest/schedule", nil)
	getRR := httptest.NewRecorder()
	r.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET: expected 200 got %d", getRR.Code)
	}

	var got models.DigestSchedule
	if err := json.NewDecoder(getRR.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Frequency != "daily" {
		t.Errorf("expected frequency=daily, got %s", got.Frequency)
	}
}
