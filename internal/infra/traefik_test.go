package infra

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ── TraefikClient tests ───────────────────────────────────────────────────────

func TestTraefikClient_Ping_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/overview" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewTraefikClient(srv.URL, "")
	if err := client.Ping(context.Background()); err != nil {
		t.Errorf("unexpected ping error: %v", err)
	}
}

func TestTraefikClient_Ping_ConnectionRefused(t *testing.T) {
	client := NewTraefikClient("http://127.0.0.1:1", "") // nothing listening
	if err := client.Ping(context.Background()); err == nil {
		t.Error("expected error for unreachable server, got nil")
	}
}

// ── ParseHostFromRule tests ───────────────────────────────────────────────────

func TestParseHostFromRule_BacktickSyntax(t *testing.T) {
	cases := []struct {
		rule string
		want string
	}{
		{`Host(` + "`" + `sonarr.itegasus.com` + "`" + `)`, "sonarr.itegasus.com"},
		{`Host(` + "`" + `radarr.home` + "`" + `) && PathPrefix(` + "`" + `/api` + "`" + `)`, "radarr.home"},
		{`Host("quoted.example.com")`, "quoted.example.com"},
		{`PathPrefix(` + "`" + `/metrics` + "`" + `)`, ""},
		{`HostRegexp(` + "`" + `{subdomain:[a-z]+}.example.com` + "`" + `)`, ""},
		{"", ""},
	}
	for _, tc := range cases {
		got := ParseHostFromRule(tc.rule)
		if tc.want == "" {
			if got != nil {
				t.Errorf("rule %q: expected nil, got %q", tc.rule, *got)
			}
		} else {
			if got == nil {
				t.Errorf("rule %q: expected %q, got nil", tc.rule, tc.want)
			} else if *got != tc.want {
				t.Errorf("rule %q: expected %q, got %q", tc.rule, tc.want, *got)
			}
		}
	}
}

// ── FetchOverview tests ───────────────────────────────────────────────────────

func TestTraefikClient_FetchOverview_ParsesCorrectly(t *testing.T) {
	payload := []byte(`{
		"version":"3.1.0",
		"http":{
			"routers":{"total":10,"warnings":1,"errors":2},
			"services":{"total":8,"errors":0},
			"middlewares":{"total":4}
		}
	}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/overview" {
			w.Header().Set("Content-Type", "application/json")
			w.Write(payload) //nolint:errcheck
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewTraefikClient(srv.URL, "")
	ov, err := client.FetchOverview(context.Background())
	if err != nil {
		t.Fatalf("FetchOverview: %v", err)
	}
	if ov.Version != "3.1.0" {
		t.Errorf("version: got %q, want 3.1.0", ov.Version)
	}
	if ov.HTTP.Routers.Total != 10 {
		t.Errorf("routers.total: got %d, want 10", ov.HTTP.Routers.Total)
	}
	if ov.HTTP.Routers.Errors != 2 {
		t.Errorf("routers.errors: got %d, want 2", ov.HTTP.Routers.Errors)
	}
	if ov.HTTP.Middlewares.Total != 4 {
		t.Errorf("middlewares.total: got %d, want 4", ov.HTTP.Middlewares.Total)
	}
}

// ── FetchRouters tests ────────────────────────────────────────────────────────

func TestTraefikClient_FetchRouters(t *testing.T) {
	payload := []map[string]interface{}{
		{"name": "router1", "rule": `Host(` + "`" + `a.com` + "`" + `)`, "service": "svc1@docker", "status": "enabled", "provider": "docker", "entryPoints": []string{"websecure"}},
		{"name": "router2", "rule": `Host(` + "`" + `b.com` + "`" + `)`, "service": "svc2@docker", "status": "disabled", "provider": "docker", "entryPoints": []string{"web"}},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/http/routers" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload) //nolint:errcheck
	}))
	defer srv.Close()

	client := NewTraefikClient(srv.URL, "")
	routers, err := client.FetchRouters(context.Background())
	if err != nil {
		t.Fatalf("FetchRouters: %v", err)
	}
	if len(routers) != 2 {
		t.Fatalf("expected 2 routers, got %d", len(routers))
	}
	if routers[1].Name != "router2" {
		t.Errorf("expected router2, got %q", routers[1].Name)
	}
	if routers[1].Status != "disabled" {
		t.Errorf("expected disabled, got %q", routers[1].Status)
	}
}

// ── FetchServices tests ───────────────────────────────────────────────────────

func TestTraefikClient_FetchServices_ServerStatus(t *testing.T) {
	payload := []map[string]interface{}{
		{
			"name":   "sonarr@docker",
			"type":   "loadbalancer",
			"status": "enabled",
			"serverStatus": map[string]string{
				"http://192.168.1.10:8989": "UP",
				"http://192.168.1.11:8989": "DOWN",
			},
		},
		{
			"name":   "api@internal",
			"type":   "loadbalancer",
			"status": "enabled",
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload) //nolint:errcheck
	}))
	defer srv.Close()

	client := NewTraefikClient(srv.URL, "")
	svcs, err := client.FetchServices(context.Background())
	if err != nil {
		t.Fatalf("FetchServices: %v", err)
	}
	if len(svcs) != 2 {
		t.Fatalf("expected 2 services, got %d", len(svcs))
	}
	var sonarr *TraefikServiceStatus
	for i := range svcs {
		if svcs[i].Name == "sonarr@docker" {
			sonarr = &svcs[i]
		}
	}
	if sonarr == nil {
		t.Fatal("sonarr service not found")
	}
	if sonarr.ServerStatus["http://192.168.1.10:8989"] != "UP" {
		t.Errorf("expected 192.168.1.10 UP, got %q", sonarr.ServerStatus["http://192.168.1.10:8989"])
	}
	if sonarr.ServerStatus["http://192.168.1.11:8989"] != "DOWN" {
		t.Errorf("expected 192.168.1.11 DOWN, got %q", sonarr.ServerStatus["http://192.168.1.11:8989"])
	}
}

