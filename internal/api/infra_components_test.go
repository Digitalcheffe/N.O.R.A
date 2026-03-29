package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/digitalcheffe/nora/internal/api"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
)

// newInfraComponentRouter wires an InfraComponentHandler onto a chi router.
func newInfraComponentRouter(t *testing.T) http.Handler {
	t.Helper()
	db := newTestDB(t)
	ic := repo.NewInfraComponentRepo(db)
	rollups := repo.NewResourceRollupRepo(db)
	checks := repo.NewCheckRepo(db)
	tc := repo.NewTraefikComponentRepo(db)
	h := api.NewInfraComponentHandler(ic, rollups, checks, tc)
	r := chi.NewRouter()
	h.Routes(r)
	return r
}

// infraComponentResponse mirrors the JSON shape returned by the handler.
type infraComponentResponse struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	IP               string  `json:"ip"`
	Type             string  `json:"type"`
	CollectionMethod string  `json:"collection_method"`
	ParentID         *string `json:"parent_id"`
	Notes            string  `json:"notes"`
	Enabled          bool    `json:"enabled"`
	LastStatus       string  `json:"last_status"`
	CreatedAt        string  `json:"created_at"`
	Credentials      *string `json:"credentials"` // must never be populated
}

func createInfraComponent(t *testing.T, router http.Handler, name, ip, compType, parentID string) infraComponentResponse {
	t.Helper()
	payload := map[string]any{
		"name": name, "ip": ip, "type": compType,
	}
	if parentID != "" {
		payload["parent_id"] = parentID
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/infrastructure", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("createInfraComponent: expected 201 got %d: %s", rr.Code, rr.Body.String())
	}
	var c infraComponentResponse
	if err := json.NewDecoder(rr.Body).Decode(&c); err != nil {
		t.Fatalf("createInfraComponent decode: %v", err)
	}
	return c
}

// ---- List -------------------------------------------------------------------

func TestListInfraComponents_Empty(t *testing.T) {
	router := newInfraComponentRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/infrastructure", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}
	var resp struct {
		Data  []any `json:"data"`
		Total int   `json:"total"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Total != 0 {
		t.Errorf("expected total=0 got %d", resp.Total)
	}
}

// ---- Create -----------------------------------------------------------------

func TestCreateInfraComponent_HappyPath(t *testing.T) {
	router := newInfraComponentRouter(t)
	c := createInfraComponent(t, router, "proxmox-node1", "192.168.1.10", "proxmox_node", "")
	if c.ID == "" {
		t.Error("expected non-empty ID")
	}
	if c.Name != "proxmox-node1" {
		t.Errorf("expected name=proxmox-node1 got %q", c.Name)
	}
	if c.Type != "proxmox_node" {
		t.Errorf("expected type=proxmox_node got %q", c.Type)
	}
	if c.CollectionMethod != "none" {
		t.Errorf("expected collection_method=none got %q", c.CollectionMethod)
	}
	if c.LastStatus != "unknown" {
		t.Errorf("expected last_status=unknown got %q", c.LastStatus)
	}
}

func TestCreateInfraComponent_InvalidType(t *testing.T) {
	router := newInfraComponentRouter(t)
	body, _ := json.Marshal(map[string]any{"name": "test", "ip": "1.2.3.4", "type": "raspberry_pi"})
	req := httptest.NewRequest(http.MethodPost, "/infrastructure", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d", rr.Code)
	}
}

func TestCreateInfraComponent_MissingName(t *testing.T) {
	router := newInfraComponentRouter(t)
	body := bytes.NewBufferString(`{"ip":"1.2.3.4","type":"bare_metal"}`)
	req := httptest.NewRequest(http.MethodPost, "/infrastructure", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d", rr.Code)
	}
}

func TestCreateInfraComponent_InvalidParentID(t *testing.T) {
	router := newInfraComponentRouter(t)
	body, _ := json.Marshal(map[string]any{
		"name": "vm01", "ip": "1.2.3.4", "type": "vm",
		"parent_id": "does-not-exist",
	})
	req := httptest.NewRequest(http.MethodPost, "/infrastructure", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 got %d", rr.Code)
	}
}

func TestCreateInfraComponent_CredentialsNotInResponse(t *testing.T) {
	router := newInfraComponentRouter(t)
	body, _ := json.Marshal(map[string]any{
		"name": "proxmox", "type": "proxmox_node",
		"credentials": `{"user":"root","pass":"secret"}`,
	})
	req := httptest.NewRequest(http.MethodPost, "/infrastructure", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 got %d: %s", rr.Code, rr.Body.String())
	}
	var c infraComponentResponse
	json.NewDecoder(rr.Body).Decode(&c)
	if c.Credentials != nil {
		t.Error("credentials must not appear in API response")
	}
	// Raw body must not contain the secret value either.
	if bytes.Contains(rr.Body.Bytes(), []byte("secret")) {
		t.Error("credentials value leaked in response body")
	}
}

// ---- Get --------------------------------------------------------------------

func TestGetInfraComponent_HappyPath(t *testing.T) {
	router := newInfraComponentRouter(t)
	created := createInfraComponent(t, router, "host1", "10.0.0.1", "bare_metal", "")
	req := httptest.NewRequest(http.MethodGet, "/infrastructure/"+created.ID, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}
}

func TestGetInfraComponent_NotFound(t *testing.T) {
	router := newInfraComponentRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/infrastructure/does-not-exist", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 got %d", rr.Code)
	}
}

// ---- Update -----------------------------------------------------------------

func TestUpdateInfraComponent_HappyPath(t *testing.T) {
	router := newInfraComponentRouter(t)
	created := createInfraComponent(t, router, "host1", "10.0.0.1", "bare_metal", "")
	body, _ := json.Marshal(map[string]any{"name": "host1-renamed"})
	req := httptest.NewRequest(http.MethodPut, "/infrastructure/"+created.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rr.Code, rr.Body.String())
	}
	var updated infraComponentResponse
	json.NewDecoder(rr.Body).Decode(&updated)
	if updated.Name != "host1-renamed" {
		t.Errorf("expected updated name got %q", updated.Name)
	}
}

func TestUpdateInfraComponent_NotFound(t *testing.T) {
	router := newInfraComponentRouter(t)
	body, _ := json.Marshal(map[string]any{"name": "x"})
	req := httptest.NewRequest(http.MethodPut, "/infrastructure/does-not-exist", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 got %d", rr.Code)
	}
}

// ---- Delete -----------------------------------------------------------------

func TestDeleteInfraComponent_HappyPath(t *testing.T) {
	router := newInfraComponentRouter(t)
	created := createInfraComponent(t, router, "host1", "10.0.0.1", "bare_metal", "")
	req := httptest.NewRequest(http.MethodDelete, "/infrastructure/"+created.ID, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204 got %d", rr.Code)
	}
}

func TestDeleteInfraComponent_NotFound(t *testing.T) {
	router := newInfraComponentRouter(t)
	req := httptest.NewRequest(http.MethodDelete, "/infrastructure/does-not-exist", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 got %d", rr.Code)
	}
}

// Deleting a parent component must SET NULL child parent_id (not cascade-delete).
func TestDeleteInfraComponent_DoesNotCascadeChildren(t *testing.T) {
	router := newInfraComponentRouter(t)
	parent := createInfraComponent(t, router, "proxmox", "10.0.0.1", "proxmox_node", "")
	child := createInfraComponent(t, router, "vm01", "10.0.0.2", "vm", parent.ID)

	req := httptest.NewRequest(http.MethodDelete, "/infrastructure/"+parent.ID, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204 got %d", rr.Code)
	}

	// Child must still exist.
	req2 := httptest.NewRequest(http.MethodGet, "/infrastructure/"+child.ID, nil)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("child component should still exist after parent deleted, got %d", rr2.Code)
	}
}

// ---- Resources --------------------------------------------------------------

func TestGetInfraComponentResources_HappyPath(t *testing.T) {
	router := newInfraComponentRouter(t)
	c := createInfraComponent(t, router, "host1", "10.0.0.1", "bare_metal", "")
	req := httptest.NewRequest(http.MethodGet, "/infrastructure/"+c.ID+"/resources", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		CPU  float64 `json:"cpu"`
		Mem  float64 `json:"mem"`
		Disk float64 `json:"disk"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode resources: %v", err)
	}
	if resp.CPU != 0 || resp.Mem != 0 || resp.Disk != 0 {
		t.Errorf("expected zeroes when no rollup data, got cpu=%.2f mem=%.2f disk=%.2f", resp.CPU, resp.Mem, resp.Disk)
	}
}

func TestGetInfraComponentResources_NotFound(t *testing.T) {
	router := newInfraComponentRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/infrastructure/does-not-exist/resources", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 got %d", rr.Code)
	}
}

func TestGetInfraComponentResources_InvalidPeriod(t *testing.T) {
	router := newInfraComponentRouter(t)
	c := createInfraComponent(t, router, "host1", "10.0.0.1", "bare_metal", "")
	req := httptest.NewRequest(http.MethodGet, "/infrastructure/"+c.ID+"/resources?period=week", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d", rr.Code)
	}
}

// ---- List includes parent_id ------------------------------------------------

func TestListInfraComponents_IncludesParentID(t *testing.T) {
	router := newInfraComponentRouter(t)
	parent := createInfraComponent(t, router, "proxmox", "10.0.0.1", "proxmox_node", "")
	createInfraComponent(t, router, "vm01", "10.0.0.2", "vm", parent.ID)

	req := httptest.NewRequest(http.MethodGet, "/infrastructure", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	var resp struct {
		Data  []infraComponentResponse `json:"data"`
		Total int                      `json:"total"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Total != 2 {
		t.Fatalf("expected total=2 got %d", resp.Total)
	}
	// Find the child.
	var child *infraComponentResponse
	for i := range resp.Data {
		if resp.Data[i].Name == "vm01" {
			child = &resp.Data[i]
		}
	}
	if child == nil {
		t.Fatal("vm01 not in list response")
	}
	if child.ParentID == nil || *child.ParentID != parent.ID {
		t.Errorf("expected parent_id=%s got %v", parent.ID, child.ParentID)
	}
}
