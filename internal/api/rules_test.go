package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/digitalcheffe/nora/internal/api"
	"github.com/digitalcheffe/nora/internal/config"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/internal/rules"
	"github.com/go-chi/chi/v5"
)

func newRulesRouter(t *testing.T) http.Handler {
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
		repo.NewSettingsRepo(db),
		repo.NewMetricsRepo(db),
		repo.NewUserRepo(db),
		repo.NewDiscoveredContainerRepo(db),
		repo.NewDiscoveredRouteRepo(db),
		repo.NewWebPushSubscriptionRepo(db),
		repo.NewSnapshotRepo(db),
		repo.NewRuleRepo(db),
		nil,
		nil,
		nil,
	)
	cfg := &config.Config{Secret: "test"}
	engine := rules.NewEngine(store, nil, cfg)
	h := api.NewRulesHandler(store, engine)
	r := chi.NewRouter()
	h.Routes(r)
	return r
}

func TestRulesCRUDRoundTrip(t *testing.T) {
	router := newRulesRouter(t)

	// ── CREATE ────────────────────────────────────────────────────────────────

	createBody := map[string]any{
		"name":             "Test Rule",
		"enabled":          true,
		"conditions":       []map[string]any{{"field": "severity", "op": "eq", "value": "error"}},
		"condition_logic":  "AND",
		"delivery_email":   false,
		"delivery_push":    false,
		"delivery_webhook": false,
		"notif_title":      "Alert: {severity}",
		"notif_body":       "{display_text}",
	}
	body, _ := json.Marshal(createBody)
	req := httptest.NewRequest(http.MethodPost, "/rules", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("create: expected 201 got %d: %s", rr.Code, rr.Body.String())
	}

	var created map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatalf("create decode: %v", err)
	}

	id, _ := created["id"].(string)
	if id == "" {
		t.Fatal("create: response missing id")
	}
	if created["name"] != "Test Rule" {
		t.Errorf("create: name = %v, want %q", created["name"], "Test Rule")
	}
	// conditions must round-trip as an array, not a raw string
	conds, ok := created["conditions"].([]any)
	if !ok {
		t.Fatalf("create: conditions is %T, want []any", created["conditions"])
	}
	if len(conds) != 1 {
		t.Errorf("create: len(conditions) = %d, want 1", len(conds))
	}

	// ── LIST — rule must appear immediately after create ──────────────────────

	req = httptest.NewRequest(http.MethodGet, "/rules", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("list: expected 200 got %d", rr.Code)
	}

	var listResp struct {
		Data  []map[string]any `json:"data"`
		Total int              `json:"total"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&listResp); err != nil {
		t.Fatalf("list decode: %v", err)
	}
	if listResp.Total != 1 {
		t.Errorf("list: total = %d, want 1", listResp.Total)
	}
	if len(listResp.Data) == 0 || listResp.Data[0]["id"] != id {
		t.Errorf("list: created rule not found in list response")
	}
	// conditions must be an array in the list response too
	if listConds, ok := listResp.Data[0]["conditions"].([]any); !ok || len(listConds) != 1 {
		t.Errorf("list: conditions not correctly deserialized: %v", listResp.Data[0]["conditions"])
	}

	// ── UPDATE ────────────────────────────────────────────────────────────────

	updateBody := map[string]any{
		"name":             "Updated Rule",
		"enabled":          false,
		"conditions":       []map[string]any{},
		"condition_logic":  "OR",
		"delivery_email":   false,
		"delivery_push":    false,
		"delivery_webhook": false,
		"notif_title":      "Updated",
		"notif_body":       "",
	}
	body, _ = json.Marshal(updateBody)
	req = httptest.NewRequest(http.MethodPut, "/rules/"+id, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("update: expected 200 got %d: %s", rr.Code, rr.Body.String())
	}

	var updated map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&updated); err != nil {
		t.Fatalf("update decode: %v", err)
	}
	if updated["name"] != "Updated Rule" {
		t.Errorf("update: name = %v, want %q", updated["name"], "Updated Rule")
	}
	if updated["enabled"] != false {
		t.Errorf("update: enabled = %v, want false", updated["enabled"])
	}

	// ── DELETE ────────────────────────────────────────────────────────────────

	req = httptest.NewRequest(http.MethodDelete, "/rules/"+id, nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204 got %d", rr.Code)
	}

	// List should now be empty.
	req = httptest.NewRequest(http.MethodGet, "/rules", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	json.NewDecoder(rr.Body).Decode(&listResp)
	if listResp.Total != 0 {
		t.Errorf("after delete: total = %d, want 0", listResp.Total)
	}
}

func TestRulesCreateRequiresName(t *testing.T) {
	router := newRulesRouter(t)

	body, _ := json.Marshal(map[string]any{"name": "", "conditions": []any{}, "condition_logic": "AND"})
	req := httptest.NewRequest(http.MethodPost, "/rules", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing name, got %d", rr.Code)
	}
}

func TestRulesConditionsRoundTrip(t *testing.T) {
	router := newRulesRouter(t)

	conditions := []map[string]any{
		{"field": "severity", "op": "eq", "value": "error"},
		{"field": "source_name", "op": "contains", "value": "nginx"},
	}
	body, _ := json.Marshal(map[string]any{
		"name":            "Multi-condition Rule",
		"enabled":         true,
		"conditions":      conditions,
		"condition_logic": "AND",
		"notif_title":     "t",
		"notif_body":      "b",
	})
	req := httptest.NewRequest(http.MethodPost, "/rules", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 got %d: %s", rr.Code, rr.Body.String())
	}

	var created map[string]any
	json.NewDecoder(rr.Body).Decode(&created)

	conds, ok := created["conditions"].([]any)
	if !ok {
		t.Fatalf("conditions is %T, want []any", created["conditions"])
	}
	if len(conds) != 2 {
		t.Errorf("len(conditions) = %d, want 2", len(conds))
	}

	// Verify via GET /rules/{id} as well.
	id := created["id"].(string)
	req = httptest.NewRequest(http.MethodGet, "/rules/"+id, nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get: expected 200 got %d", rr.Code)
	}
	var got map[string]any
	json.NewDecoder(rr.Body).Decode(&got)
	gotConds, _ := got["conditions"].([]any)
	if len(gotConds) != 2 {
		t.Errorf("get: len(conditions) = %d, want 2", len(gotConds))
	}
}
