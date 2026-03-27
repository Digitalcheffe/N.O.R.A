package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/digitalcheffe/nora/internal/api"
	"github.com/digitalcheffe/nora/internal/apptemplate"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
	"testing/fstest"
)

// newAppTemplatesRouter builds a chi router with the app templates handler under /api/v1.
func newAppTemplatesRouter(t *testing.T) http.Handler {
	t.Helper()

	// Minimal in-memory YAML app template filesystem.
	fsys := fstest.MapFS{
		"sonarr.yaml": {Data: []byte(`meta:
  name: Sonarr
  category: Media
  logo: sonarr.png
  description: TV series management
  capability: full
webhook:
  display_template: "{event_type}"
  field_mappings:
    event_type: "$.eventType"
  severity_mapping:
    Download: info
`)},
	}

	registry, err := apptemplate.NewRegistry(fsys)
	if err != nil {
		t.Fatalf("build registry: %v", err)
	}

	db := newTestDB(t)
	customRepo := repo.NewCustomProfileRepo(db)

	h := api.NewProfilesHandler(registry, customRepo)
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		h.Routes(r)
	})
	return r
}

// --- GET /app-templates ---

func TestAppTemplatesList_OK(t *testing.T) {
	router := newAppTemplatesRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/app-templates", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["total"].(float64) != 1 {
		t.Errorf("expected 1 app template, got %v", resp["total"])
	}
}

func TestAppTemplatesGet_NotFound(t *testing.T) {
	router := newAppTemplatesRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/app-templates/does-not-exist", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 got %d", rr.Code)
	}
}

// --- POST /app-templates/validate ---

func TestAppTemplatesValidate_Valid(t *testing.T) {
	router := newAppTemplatesRouter(t)

	body, _ := json.Marshal(map[string]string{"yaml": `meta:
  name: MyApp
  category: Custom
  description: A custom app
  capability: webhook_only
`})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/app-templates/validate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["valid"] != true {
		t.Errorf("expected valid=true, got %v (errors: %v)", resp["valid"], resp["errors"])
	}
}

func TestAppTemplatesValidate_MissingRequired(t *testing.T) {
	router := newAppTemplatesRouter(t)

	// meta.name and meta.category are missing
	body, _ := json.Marshal(map[string]string{"yaml": `meta:
  description: Missing name and category
  capability: webhook_only
`})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/app-templates/validate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}
	var resp map[string]interface{}
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	if resp["valid"] != false {
		t.Errorf("expected valid=false")
	}
	errs, _ := resp["errors"].([]interface{})
	if len(errs) == 0 {
		t.Error("expected at least one error")
	}
}

func TestAppTemplatesValidate_InvalidYAML(t *testing.T) {
	router := newAppTemplatesRouter(t)

	body, _ := json.Marshal(map[string]string{"yaml": ":\t:invalid yaml{"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/app-templates/validate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}
	var resp map[string]interface{}
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	if resp["valid"] != false {
		t.Errorf("expected valid=false for bad YAML")
	}
}

// --- POST /app-templates/custom ---

func TestAppTemplatesCreateCustom_OK(t *testing.T) {
	router := newAppTemplatesRouter(t)

	body, _ := json.Marshal(map[string]string{"yaml": `meta:
  name: MyCustomApp
  category: Custom
  description: A test custom app template
  capability: webhook_only
`})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/app-templates/custom", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["id"] == "" || resp["id"] == nil {
		t.Error("expected non-empty id in response")
	}
	if resp["name"] != "MyCustomApp" {
		t.Errorf("expected name=MyCustomApp got %v", resp["name"])
	}
}

func TestAppTemplatesCreateCustom_InvalidYAML(t *testing.T) {
	router := newAppTemplatesRouter(t)

	body, _ := json.Marshal(map[string]string{"yaml": `meta:
  description: Missing required fields
`})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/app-templates/custom", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 got %d: %s", rr.Code, rr.Body.String())
	}
}
