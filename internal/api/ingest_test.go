package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/digitalcheffe/nora/internal/api"
	"github.com/digitalcheffe/nora/internal/apptemplate"
	"github.com/digitalcheffe/nora/internal/ingest"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
)

// newIngestRouter wires up the ingest handler on a chi router with a real in-memory DB.
func newIngestRouter(t *testing.T) (http.Handler, *repo.Store) {
	t.Helper()
	db := newTestDB(t)
	appRepo := repo.NewAppRepo(db)
	eventRepo := repo.NewEventRepo(db)
	store := repo.NewStore(appRepo, eventRepo, repo.NewCheckRepo(db), repo.NewRollupRepo(db), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	limiter := ingest.NewRateLimiter()
	profiler := &apptemplate.NoopLoader{}

	r := chi.NewRouter()
	r.Post("/api/v1/ingest/{token}", api.HandleIngest(store, profiler, limiter))
	return r, store
}

func postIngest(t *testing.T, router http.Handler, token string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest/"+token, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func TestHandleIngest_HappyPath(t *testing.T) {
	// Build everything on a single in-memory DB so token lookup works.
	db := newTestDB(t)
	appRepo := repo.NewAppRepo(db)
	eventRepo := repo.NewEventRepo(db)
	s := repo.NewStore(appRepo, eventRepo, repo.NewCheckRepo(db), repo.NewRollupRepo(db), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	limiter := ingest.NewRateLimiter()

	r := chi.NewRouter()
	r.Post("/api/v1/ingest/{token}", api.HandleIngest(s, &apptemplate.NoopLoader{}, limiter))

	appsR := chi.NewRouter()
	api.NewAppsHandler(appRepo).Routes(appsR)

	// Create an app
	appBody, _ := json.Marshal(map[string]any{"name": "my-app", "rate_limit": 100})
	req := httptest.NewRequest(http.MethodPost, "/apps", bytes.NewReader(appBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	appsR.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create app: %d %s", rr.Code, rr.Body.String())
	}

	var createdApp map[string]any
	json.NewDecoder(rr.Body).Decode(&createdApp)
	token := createdApp["token"].(string)

	payload := []byte(`{"msg":"hello"}`)
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/ingest/"+token, bytes.NewReader(payload))
	req2.Header.Set("Content-Type", "application/json")
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusAccepted {
		t.Fatalf("expected 202 got %d: %s", rr2.Code, rr2.Body.String())
	}
	var resp map[string]string
	json.NewDecoder(rr2.Body).Decode(&resp)
	if resp["status"] != "accepted" {
		t.Errorf("expected status=accepted, got %v", resp)
	}
	if resp["id"] == "" {
		t.Error("expected non-empty id")
	}
}

func TestHandleIngest_InvalidToken(t *testing.T) {
	router, _ := newIngestRouter(t)
	rr := postIngest(t, router, "bad-token", []byte(`{"x":1}`))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleIngest_BadJSON(t *testing.T) {
	router, _ := newIngestRouter(t)
	rr := postIngest(t, router, "any-token", []byte(`not-json`))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleIngest_EmptyBody(t *testing.T) {
	router, _ := newIngestRouter(t)
	rr := postIngest(t, router, "any-token", []byte{})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleIngest_PayloadTooLarge(t *testing.T) {
	router, _ := newIngestRouter(t)

	// Build a payload just over 1 MB.
	large := strings.Repeat("x", 1<<20+1)
	body := []byte(`{"data":"` + large + `"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest/any-token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413 got %d", rr.Code)
	}
}

func TestHandleIngest_RateLimit(t *testing.T) {
	db := newTestDB(t)
	appRepo := repo.NewAppRepo(db)
	eventRepo := repo.NewEventRepo(db)
	s := repo.NewStore(appRepo, eventRepo, repo.NewCheckRepo(db), repo.NewRollupRepo(db), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	limiter := ingest.NewRateLimiter()

	r := chi.NewRouter()
	r.Post("/api/v1/ingest/{token}", api.HandleIngest(s, &apptemplate.NoopLoader{}, limiter))

	// Create app with limit of 1
	appsR := chi.NewRouter()
	api.NewAppsHandler(appRepo).Routes(appsR)
	appBody, _ := json.Marshal(map[string]any{"name": "rate-app", "rate_limit": 1})
	req := httptest.NewRequest(http.MethodPost, "/apps", bytes.NewReader(appBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	appsR.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create app: %d %s", rr.Code, rr.Body.String())
	}
	var createdApp map[string]any
	json.NewDecoder(rr.Body).Decode(&createdApp)
	token := createdApp["token"].(string)

	payload := []byte(`{"x":1}`)

	// First request: accepted
	rr1 := httptest.NewRecorder()
	r.ServeHTTP(rr1, httptest.NewRequest(http.MethodPost, "/api/v1/ingest/"+token, bytes.NewReader(payload)))
	if rr1.Code != http.StatusAccepted {
		t.Fatalf("first request: expected 202 got %d", rr1.Code)
	}

	// Second request: rate limited
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, httptest.NewRequest(http.MethodPost, "/api/v1/ingest/"+token, bytes.NewReader(payload)))
	if rr2.Code != http.StatusTooManyRequests {
		t.Errorf("second request: expected 429 got %d: %s", rr2.Code, rr2.Body.String())
	}
}
