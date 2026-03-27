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
		repo.NewPhysicalHostRepo(db),
		repo.NewVirtualHostRepo(db),
		repo.NewDockerEngineRepo(db),
		repo.NewInfraRepo(db),
		repo.NewSettingsRepo(db),
	)
	digestJob := jobs.NewDigestJob(store, &config.Config{})
	h := api.NewDigestHandler(store, digestJob)
	r := chi.NewRouter()
	h.Routes(r)
	return r, store
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

func TestPutDigestSchedule_Happy(t *testing.T) {
	r, _ := newDigestRouter(t)

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
	r, _ := newDigestRouter(t)

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
	r, _ := newDigestRouter(t)

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
	r, _ := newDigestRouter(t)

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
	r, _ := newDigestRouter(t)

	req := httptest.NewRequest(http.MethodPut, "/digest/schedule", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", rr.Code)
	}
}

// --- POST /digest/send-now ---

func TestSendNow_Accepted(t *testing.T) {
	r, _ := newDigestRouter(t)

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

// --- GET then PUT roundtrip ---

func TestDigestScheduleRoundtrip(t *testing.T) {
	r, _ := newDigestRouter(t)

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
