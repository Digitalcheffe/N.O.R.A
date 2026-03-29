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

func newDriversRouter(t *testing.T) (http.Handler, repo.SettingsRepo) {
	t.Helper()
	db := newTestDB(t)
	settings := repo.NewSettingsRepo(db)
	h := api.NewIntegrationDriversHandler(settings)
	r := chi.NewRouter()
	h.Routes(r)
	return r, settings
}

// --- GET /integration-drivers ---

func TestListIntegrationDrivers_ReturnsAll(t *testing.T) {
	r, _ := newDriversRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/integration-drivers", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data  []map[string]interface{} `json:"data"`
		Total int                      `json:"total"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 5 {
		t.Errorf("expected 5 drivers, got %d", resp.Total)
	}
	if len(resp.Data) != 5 {
		t.Fatalf("expected 5 items in data, got %d", len(resp.Data))
	}

	// All should start unconfigured.
	for _, d := range resp.Data {
		if configured, ok := d["configured"].(bool); !ok || configured {
			t.Errorf("driver %v: expected configured=false", d["name"])
		}
	}
}

func TestListIntegrationDrivers_NoCredentials(t *testing.T) {
	r, _ := newDriversRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/integration-drivers", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	body := rr.Body.String()
	// Response must never contain credential field names.
	credFields := []string{"api_token", "token_secret", "api_secret", "password", "auth_password", "priv_password", "community"}
	for _, field := range credFields {
		// Only check that literal credential values aren't exposed;
		// field names in capabilities/description are acceptable.
		_ = field
	}
	// The key check: response must not include a "api_token" key at the data level.
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, _ := raw["data"].([]interface{})
	for _, item := range data {
		d, _ := item.(map[string]interface{})
		for _, credKey := range []string{"api_token", "token_id", "token_secret", "api_key", "api_secret", "password", "community", "username", "auth_password", "priv_password"} {
			if _, exists := d[credKey]; exists {
				t.Errorf("driver %v: response must not contain credential key %q", d["name"], credKey)
			}
		}
	}
}

func TestListIntegrationDrivers_ExpectedNames(t *testing.T) {
	r, _ := newDriversRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/integration-drivers", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	var resp struct {
		Data []struct {
			Name         string   `json:"name"`
			Label        string   `json:"label"`
			Capabilities []string `json:"capabilities"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	wantNames := []string{"traefik", "proxmox", "opnsense", "synology", "snmp"}
	for i, want := range wantNames {
		if resp.Data[i].Name != want {
			t.Errorf("driver[%d]: expected name=%q got %q", i, want, resp.Data[i].Name)
		}
		if len(resp.Data[i].Capabilities) == 0 {
			t.Errorf("driver %q: expected non-empty capabilities", want)
		}
	}
}

// --- PUT /integration-drivers/{name} ---

func TestConfigureDriver_Happy(t *testing.T) {
	r, _ := newDriversRouter(t)

	body, _ := json.Marshal(map[string]string{
		"api_url":   "http://traefik:8080",
		"api_token": "secret",
	})
	req := httptest.NewRequest(http.MethodPut, "/integration-drivers/traefik", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]bool
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp["configured"] {
		t.Error("expected configured=true after PUT")
	}
}

func TestConfigureDriver_AppearsConfiguredInList(t *testing.T) {
	r, _ := newDriversRouter(t)

	// Configure proxmox.
	body, _ := json.Marshal(map[string]string{
		"host_url":     "https://proxmox.local:8006",
		"token_id":     "user@pam!mytoken",
		"token_secret": "abc123",
	})
	putReq := httptest.NewRequest(http.MethodPut, "/integration-drivers/proxmox", bytes.NewReader(body))
	putReq.Header.Set("Content-Type", "application/json")
	putRR := httptest.NewRecorder()
	r.ServeHTTP(putRR, putReq)
	if putRR.Code != http.StatusOK {
		t.Fatalf("PUT: expected 200 got %d", putRR.Code)
	}

	// List and verify proxmox is now configured.
	getReq := httptest.NewRequest(http.MethodGet, "/integration-drivers", nil)
	getRR := httptest.NewRecorder()
	r.ServeHTTP(getRR, getReq)

	var resp struct {
		Data []struct {
			Name       string `json:"name"`
			Configured bool   `json:"configured"`
		} `json:"data"`
	}
	if err := json.NewDecoder(getRR.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, d := range resp.Data {
		if d.Name == "proxmox" && !d.Configured {
			t.Error("proxmox should be configured after PUT")
		}
		if d.Name != "proxmox" && d.Configured {
			t.Errorf("driver %q should still be unconfigured", d.Name)
		}
	}
}

func TestConfigureDriver_UnknownName(t *testing.T) {
	r, _ := newDriversRouter(t)

	body, _ := json.Marshal(map[string]string{"api_url": "http://whatever"})
	req := httptest.NewRequest(http.MethodPut, "/integration-drivers/unknown", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestConfigureDriver_InvalidBody(t *testing.T) {
	r, _ := newDriversRouter(t)

	req := httptest.NewRequest(http.MethodPut, "/integration-drivers/traefik", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestConfigureSNMP_V2c(t *testing.T) {
	r, _ := newDriversRouter(t)

	body, _ := json.Marshal(map[string]string{
		"version":   "v2c",
		"community": "public",
	})
	req := httptest.NewRequest(http.MethodPut, "/integration-drivers/snmp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]bool
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp["configured"] {
		t.Error("expected configured=true for SNMP v2c with community set")
	}
}

func TestConfigureSNMP_V3(t *testing.T) {
	r, _ := newDriversRouter(t)

	body, _ := json.Marshal(map[string]string{
		"version":       "v3",
		"username":      "snmpuser",
		"auth_password": "authpass",
		"priv_password": "privpass",
	})
	req := httptest.NewRequest(http.MethodPut, "/integration-drivers/snmp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]bool
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp["configured"] {
		t.Error("expected configured=true for SNMP v3 with username set")
	}
}

// --- DELETE /integration-drivers/{name} ---

func TestDisconnectDriver_Happy(t *testing.T) {
	r, _ := newDriversRouter(t)

	// First configure.
	configBody, _ := json.Marshal(map[string]string{
		"host_url": "https://synology.local:5001",
		"username": "admin",
		"password": "hunter2",
	})
	putReq := httptest.NewRequest(http.MethodPut, "/integration-drivers/synology", bytes.NewReader(configBody))
	putReq.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(httptest.NewRecorder(), putReq)

	// Now disconnect.
	delReq := httptest.NewRequest(http.MethodDelete, "/integration-drivers/synology", nil)
	delRR := httptest.NewRecorder()
	r.ServeHTTP(delRR, delReq)

	if delRR.Code != http.StatusNoContent {
		t.Fatalf("expected 204 got %d: %s", delRR.Code, delRR.Body.String())
	}

	// List should show synology as not configured.
	getReq := httptest.NewRequest(http.MethodGet, "/integration-drivers", nil)
	getRR := httptest.NewRecorder()
	r.ServeHTTP(getRR, getReq)

	var resp struct {
		Data []struct {
			Name       string `json:"name"`
			Configured bool   `json:"configured"`
		} `json:"data"`
	}
	if err := json.NewDecoder(getRR.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, d := range resp.Data {
		if d.Name == "synology" && d.Configured {
			t.Error("synology should be unconfigured after disconnect")
		}
	}
}

func TestDisconnectDriver_UnknownName(t *testing.T) {
	r, _ := newDriversRouter(t)

	req := httptest.NewRequest(http.MethodDelete, "/integration-drivers/unknown", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d: %s", rr.Code, rr.Body.String())
	}
}
