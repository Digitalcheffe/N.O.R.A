package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/digitalcheffe/nora/internal/api"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
)

// newChecksRouter wires a ChecksHandler onto a chi router backed by an in-memory DB.
func newChecksRouter(t *testing.T) http.Handler {
	t.Helper()
	db := newTestDB(t)
	checkRepo := repo.NewCheckRepo(db)
	eventRepo := repo.NewEventRepo(db)
	h := api.NewChecksHandler(checkRepo, eventRepo)
	r := chi.NewRouter()
	h.Routes(r)
	return r
}

// createCheck POSTs a create-check request and returns the decoded MonitorCheck.
func createCheck(t *testing.T, router http.Handler, name, checkType, target string, interval int) models.MonitorCheck {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"name":          name,
		"type":          checkType,
		"target":        target,
		"interval_secs": interval,
	})
	req := httptest.NewRequest(http.MethodPost, "/checks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("createCheck: expected 201 got %d: %s", rr.Code, rr.Body.String())
	}
	var check models.MonitorCheck
	if err := json.NewDecoder(rr.Body).Decode(&check); err != nil {
		t.Fatalf("createCheck decode: %v", err)
	}
	return check
}

// --- List ---

func TestListChecks_Empty(t *testing.T) {
	router := newChecksRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/checks", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}
	var resp struct {
		Data  []models.MonitorCheck `json:"data"`
		Total int                   `json:"total"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Total != 0 {
		t.Errorf("expected total=0 got %d", resp.Total)
	}
}

func TestListChecks_ReturnsAll(t *testing.T) {
	router := newChecksRouter(t)
	createCheck(t, router, "OPNsense", "ping", "192.168.1.1", 60)
	createCheck(t, router, "n8n", "url", "http://localhost:5678", 120)

	req := httptest.NewRequest(http.MethodGet, "/checks", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	var resp struct {
		Data  []models.MonitorCheck `json:"data"`
		Total int                   `json:"total"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Total != 2 {
		t.Errorf("expected total=2 got %d", resp.Total)
	}
}

// --- Create ---

func TestCreateCheck_Ping(t *testing.T) {
	router := newChecksRouter(t)
	check := createCheck(t, router, "OPNsense", "ping", "192.168.1.1", 60)

	if check.ID == "" {
		t.Error("expected non-empty ID")
	}
	if check.Name != "OPNsense" {
		t.Errorf("expected name=OPNsense got %q", check.Name)
	}
	if check.Type != "ping" {
		t.Errorf("expected type=ping got %q", check.Type)
	}
	if !check.Enabled {
		t.Error("expected enabled=true by default")
	}
}

func TestCreateCheck_URL(t *testing.T) {
	router := newChecksRouter(t)
	body, _ := json.Marshal(map[string]any{
		"name":            "n8n",
		"type":            "url",
		"target":          "https://n8n.example.com",
		"interval_secs":   60,
		"expected_status": 200,
	})
	req := httptest.NewRequest(http.MethodPost, "/checks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 got %d: %s", rr.Code, rr.Body.String())
	}
	var check models.MonitorCheck
	json.NewDecoder(rr.Body).Decode(&check)
	if check.ExpectedStatus != 200 {
		t.Errorf("expected expected_status=200 got %d", check.ExpectedStatus)
	}
}

func TestCreateCheck_SSL_DefaultThresholds(t *testing.T) {
	router := newChecksRouter(t)
	body, _ := json.Marshal(map[string]any{
		"name":          "My Domain",
		"type":          "ssl",
		"target":        "https://example.com",
		"interval_secs": 3600,
	})
	req := httptest.NewRequest(http.MethodPost, "/checks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 got %d: %s", rr.Code, rr.Body.String())
	}
	var check models.MonitorCheck
	json.NewDecoder(rr.Body).Decode(&check)
	if check.SSLWarnDays != 30 {
		t.Errorf("expected ssl_warn_days=30 got %d", check.SSLWarnDays)
	}
	if check.SSLCritDays != 7 {
		t.Errorf("expected ssl_crit_days=7 got %d", check.SSLCritDays)
	}
}

func TestCreateCheck_MissingName(t *testing.T) {
	router := newChecksRouter(t)
	body, _ := json.Marshal(map[string]any{
		"type":          "ping",
		"target":        "192.168.1.1",
		"interval_secs": 60,
	})
	req := httptest.NewRequest(http.MethodPost, "/checks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d", rr.Code)
	}
}

func TestCreateCheck_InvalidType(t *testing.T) {
	router := newChecksRouter(t)
	body, _ := json.Marshal(map[string]any{
		"name":          "Bad",
		"type":          "snmp",
		"target":        "192.168.1.1",
		"interval_secs": 60,
	})
	req := httptest.NewRequest(http.MethodPost, "/checks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d", rr.Code)
	}
}

func TestCreateCheck_IntervalTooShort(t *testing.T) {
	router := newChecksRouter(t)
	body, _ := json.Marshal(map[string]any{
		"name":          "Fast",
		"type":          "ping",
		"target":        "192.168.1.1",
		"interval_secs": 10,
	})
	req := httptest.NewRequest(http.MethodPost, "/checks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d", rr.Code)
	}
}

func TestCreateCheck_URLWithoutScheme(t *testing.T) {
	router := newChecksRouter(t)
	body, _ := json.Marshal(map[string]any{
		"name":          "Bad URL",
		"type":          "url",
		"target":        "example.com",
		"interval_secs": 60,
	})
	req := httptest.NewRequest(http.MethodPost, "/checks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d", rr.Code)
	}
}

func TestCreateCheck_SSLWithoutScheme(t *testing.T) {
	router := newChecksRouter(t)
	body, _ := json.Marshal(map[string]any{
		"name":          "Bad SSL",
		"type":          "ssl",
		"target":        "example.com",
		"interval_secs": 3600,
	})
	req := httptest.NewRequest(http.MethodPost, "/checks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d", rr.Code)
	}
}

// --- Get ---

func TestGetCheck_HappyPath(t *testing.T) {
	router := newChecksRouter(t)
	created := createCheck(t, router, "OPNsense", "ping", "192.168.1.1", 60)

	req := httptest.NewRequest(http.MethodGet, "/checks/"+created.ID, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}
	var check models.MonitorCheck
	json.NewDecoder(rr.Body).Decode(&check)
	if check.ID != created.ID {
		t.Errorf("expected id=%s got %s", created.ID, check.ID)
	}
}

func TestGetCheck_NotFound(t *testing.T) {
	router := newChecksRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/checks/does-not-exist", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 got %d", rr.Code)
	}
}

// --- Update ---

func TestUpdateCheck_HappyPath(t *testing.T) {
	router := newChecksRouter(t)
	created := createCheck(t, router, "OPNsense", "ping", "192.168.1.1", 60)

	body, _ := json.Marshal(map[string]any{"name": "OPNsense Updated", "interval_secs": 120})
	req := httptest.NewRequest(http.MethodPut, "/checks/"+created.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rr.Code, rr.Body.String())
	}
	var check models.MonitorCheck
	json.NewDecoder(rr.Body).Decode(&check)
	if check.Name != "OPNsense Updated" {
		t.Errorf("expected updated name got %q", check.Name)
	}
	if check.IntervalSecs != 120 {
		t.Errorf("expected interval_secs=120 got %d", check.IntervalSecs)
	}
}

func TestUpdateCheck_NotFound(t *testing.T) {
	router := newChecksRouter(t)
	body, _ := json.Marshal(map[string]any{"name": "X"})
	req := httptest.NewRequest(http.MethodPut, "/checks/does-not-exist", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 got %d", rr.Code)
	}
}

// --- Delete ---

func TestDeleteCheck_HappyPath(t *testing.T) {
	router := newChecksRouter(t)
	created := createCheck(t, router, "OPNsense", "ping", "192.168.1.1", 60)

	req := httptest.NewRequest(http.MethodDelete, "/checks/"+created.ID, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204 got %d", rr.Code)
	}

	// Confirm it's gone.
	req = httptest.NewRequest(http.MethodGet, "/checks/"+created.ID, nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete got %d", rr.Code)
	}
}

func TestDeleteCheck_NotFound(t *testing.T) {
	router := newChecksRouter(t)
	req := httptest.NewRequest(http.MethodDelete, "/checks/does-not-exist", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 got %d", rr.Code)
	}
}

// --- Manual Run ---

func TestRunCheck_URL_ReturnsResult(t *testing.T) {
	// Start a local HTTP server to check against.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	router := newChecksRouter(t)
	body, _ := json.Marshal(map[string]any{
		"name":            "Local",
		"type":            "url",
		"target":          ts.URL,
		"interval_secs":   60,
		"expected_status": 200,
	})
	req := httptest.NewRequest(http.MethodPost, "/checks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: expected 201 got %d", rr.Code)
	}
	var check models.MonitorCheck
	json.NewDecoder(rr.Body).Decode(&check)

	// Run it.
	req = httptest.NewRequest(http.MethodPost, "/checks/"+check.ID+"/run", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("run: expected 200 got %d: %s", rr.Code, rr.Body.String())
	}

	var result struct {
		Status    string          `json:"status"`
		Result    json.RawMessage `json:"result"`
		CheckedAt string          `json:"checked_at"`
	}
	json.NewDecoder(rr.Body).Decode(&result)

	if result.Status != "up" {
		t.Errorf("expected status=up got %q", result.Status)
	}
	if result.CheckedAt == "" {
		t.Error("expected non-empty checked_at")
	}
}

func TestRunCheck_URL_StatusMismatch_ReturnsDown(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	router := newChecksRouter(t)
	body, _ := json.Marshal(map[string]any{
		"name":            "Down Service",
		"type":            "url",
		"target":          ts.URL,
		"interval_secs":   60,
		"expected_status": 200,
	})
	req := httptest.NewRequest(http.MethodPost, "/checks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	var check models.MonitorCheck
	json.NewDecoder(rr.Body).Decode(&check)

	req = httptest.NewRequest(http.MethodPost, "/checks/"+check.ID+"/run", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}
	var result struct {
		Status string `json:"status"`
	}
	json.NewDecoder(rr.Body).Decode(&result)
	if result.Status != "down" {
		t.Errorf("expected status=down got %q", result.Status)
	}
}

func TestRunCheck_NotFound(t *testing.T) {
	router := newChecksRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/checks/does-not-exist/run", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 got %d", rr.Code)
	}
}
