package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/digitalcheffe/nora/internal/api"
	"github.com/digitalcheffe/nora/internal/jobs"
	"github.com/go-chi/chi/v5"
)

func newJobsRouter(t *testing.T, registry *jobs.Registry) http.Handler {
	t.Helper()
	h := api.NewJobsHandler(registry)
	r := chi.NewRouter()
	h.Routes(r)
	return r
}

func newTestRegistry() *jobs.Registry {
	reg := jobs.NewRegistry()
	reg.Register(&jobs.JobEntry{
		ID:          "ok_job",
		Name:        "OK Job",
		Description: "A job that always succeeds.",
		Category:    "system",
		RunFn:       func(_ context.Context) error { return nil },
	})
	reg.Register(&jobs.JobEntry{
		ID:          "fail_job",
		Name:        "Failing Job",
		Description: "A job that always fails.",
		Category:    "system",
		RunFn:       func(_ context.Context) error { return errors.New("always fails") },
	})
	return reg
}

func TestJobsList_HappyPath(t *testing.T) {
	router := newJobsRouter(t, newTestRegistry())
	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var body struct {
		Data []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Category string `json:"category"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(body.Data))
	}
	if body.Data[0].ID != "ok_job" {
		t.Errorf("expected first job id ok_job, got %s", body.Data[0].ID)
	}
	if body.Data[1].Category != "system" {
		t.Errorf("expected category system, got %s", body.Data[1].Category)
	}
}

func TestJobsRun_HappyPath(t *testing.T) {
	router := newJobsRouter(t, newTestRegistry())
	req := httptest.NewRequest(http.MethodPost, "/jobs/ok_job/run", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %v", body["status"])
	}
	if _, ok := body["duration_ms"]; !ok {
		t.Error("expected duration_ms in response")
	}
}

func TestJobsRun_UnknownID(t *testing.T) {
	router := newJobsRouter(t, newTestRegistry())
	req := httptest.NewRequest(http.MethodPost, "/jobs/no_such_job/run", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestJobsRun_ErrorPath(t *testing.T) {
	router := newJobsRouter(t, newTestRegistry())
	req := httptest.NewRequest(http.MethodPost, "/jobs/fail_job/run", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "error" {
		t.Errorf("expected status error, got %v", body["status"])
	}
	if body["error"] != "always fails" {
		t.Errorf("expected error message 'always fails', got %v", body["error"])
	}
}

func TestJobsList_LastRunPopulatedAfterRun(t *testing.T) {
	reg := newTestRegistry()
	router := newJobsRouter(t, reg)

	// Run the job first.
	runReq := httptest.NewRequest(http.MethodPost, "/jobs/ok_job/run", nil)
	httptest.NewRecorder() // discard
	router.ServeHTTP(httptest.NewRecorder(), runReq)

	// Now list — last_run_at and last_run_status should be populated.
	listReq := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, listReq)

	var body struct {
		Data []struct {
			ID            string  `json:"id"`
			LastRunAt     *string `json:"last_run_at"`
			LastRunStatus *string `json:"last_run_status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, j := range body.Data {
		if j.ID == "ok_job" {
			if j.LastRunAt == nil {
				t.Error("expected last_run_at to be set after run")
			}
			if j.LastRunStatus == nil || *j.LastRunStatus != "ok" {
				t.Errorf("expected last_run_status=ok, got %v", j.LastRunStatus)
			}
		}
	}
}
