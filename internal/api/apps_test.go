package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/digitalcheffe/nora/internal/api"
	"github.com/digitalcheffe/nora/internal/config"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/migrations"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// newTestDB opens an in-memory SQLite database with all migrations applied.
func newTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	cfg := &config.Config{DBPath: ":memory:", DevMode: true}
	db, err := repo.Open(cfg, migrations.Files)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// newTestRouter wires an AppsHandler onto a chi router.
func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	db := newTestDB(t)
	appRepo := repo.NewAppRepo(db)
	h := api.NewAppsHandler(appRepo)
	r := chi.NewRouter()
	h.Routes(r)
	return r
}

// createApp is a helper that POSTs a create-app request and returns the decoded App.
func createApp(t *testing.T, router http.Handler, name string) models.App {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"name": name, "profile_id": "sonarr", "rate_limit": 100})
	req := httptest.NewRequest(http.MethodPost, "/apps", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("createApp: expected 201 got %d: %s", rr.Code, rr.Body.String())
	}
	var app models.App
	if err := json.NewDecoder(rr.Body).Decode(&app); err != nil {
		t.Fatalf("createApp decode: %v", err)
	}
	return app
}

// --- List ---

func TestListApps_Empty(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/apps", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}
	var resp struct {
		Data  []models.App `json:"data"`
		Total int          `json:"total"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 0 {
		t.Errorf("expected total=0 got %d", resp.Total)
	}
}

func TestListApps_ReturnsAll(t *testing.T) {
	router := newTestRouter(t)
	createApp(t, router, "Sonarr")
	createApp(t, router, "Radarr")

	req := httptest.NewRequest(http.MethodGet, "/apps", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	var resp struct {
		Data  []models.App `json:"data"`
		Total int          `json:"total"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Total != 2 {
		t.Errorf("expected total=2 got %d", resp.Total)
	}
}

// --- Create ---

func TestCreateApp_HappyPath(t *testing.T) {
	router := newTestRouter(t)
	app := createApp(t, router, "Sonarr")

	if app.ID == "" {
		t.Error("expected non-empty ID")
	}
	if app.Token == "" {
		t.Error("expected non-empty token")
	}
	if app.Name != "Sonarr" {
		t.Errorf("expected name=Sonarr got %q", app.Name)
	}
	// Token should be base64url, 43 chars for 32 bytes with no padding.
	if len(app.Token) != 43 {
		t.Errorf("expected token length 43 got %d", len(app.Token))
	}
}

func TestCreateApp_MissingName(t *testing.T) {
	router := newTestRouter(t)
	body := bytes.NewBufferString(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/apps", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d", rr.Code)
	}
}

// --- Get ---

func TestGetApp_HappyPath(t *testing.T) {
	router := newTestRouter(t)
	created := createApp(t, router, "Sonarr")

	req := httptest.NewRequest(http.MethodGet, "/apps/"+created.ID, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}
	var app models.App
	json.NewDecoder(rr.Body).Decode(&app)
	if app.ID != created.ID {
		t.Errorf("expected id=%s got %s", created.ID, app.ID)
	}
}

func TestGetApp_NotFound(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/apps/does-not-exist", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 got %d", rr.Code)
	}
}

// --- Update ---

func TestUpdateApp_HappyPath(t *testing.T) {
	router := newTestRouter(t)
	created := createApp(t, router, "Sonarr")

	body, _ := json.Marshal(map[string]any{"name": "Sonarr Updated"})
	req := httptest.NewRequest(http.MethodPut, "/apps/"+created.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rr.Code, rr.Body.String())
	}
	var app models.App
	json.NewDecoder(rr.Body).Decode(&app)
	if app.Name != "Sonarr Updated" {
		t.Errorf("expected updated name got %q", app.Name)
	}
}

func TestUpdateApp_NotFound(t *testing.T) {
	router := newTestRouter(t)
	body, _ := json.Marshal(map[string]any{"name": "X"})
	req := httptest.NewRequest(http.MethodPut, "/apps/does-not-exist", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 got %d", rr.Code)
	}
}

// --- Delete ---

func TestDeleteApp_HappyPath(t *testing.T) {
	router := newTestRouter(t)
	created := createApp(t, router, "Sonarr")

	req := httptest.NewRequest(http.MethodDelete, "/apps/"+created.ID, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204 got %d", rr.Code)
	}
}

func TestDeleteApp_NotFound(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodDelete, "/apps/does-not-exist", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 got %d", rr.Code)
	}
}

// --- RegenerateToken ---

func TestRegenerateToken_HappyPath(t *testing.T) {
	router := newTestRouter(t)
	created := createApp(t, router, "Sonarr")
	originalToken := created.Token

	req := httptest.NewRequest(http.MethodPost, "/apps/"+created.ID+"/token/regenerate", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Token == "" {
		t.Error("expected non-empty new token")
	}
	if resp.Token == originalToken {
		t.Error("expected token to change after regeneration")
	}
}

func TestRegenerateToken_NotFound(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/apps/does-not-exist/token/regenerate", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 got %d", rr.Code)
	}
}
