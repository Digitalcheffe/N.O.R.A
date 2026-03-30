package infra

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ── cert helpers ─────────────────────────────────────────────────────────────

// selfSignedCertPEM generates a self-signed cert for domain and returns it as
// a PEM-encoded []byte together with the parsed *x509.Certificate.
func selfSignedCertPEM(t *testing.T, domain string, notAfter time.Time) ([]byte, *x509.Certificate) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: domain, Organization: []string{"Test CA"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
		DNSNames:     []string{domain, "www." + domain},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}

	pemBlock := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return pemBlock, parsed
}

// traefikCertPayload returns a JSON body matching Traefik's /api/tls/certificates shape.
func traefikCertPayload(t *testing.T, domain string, notAfter time.Time) []byte {
	t.Helper()
	pemBytes, _ := selfSignedCertPEM(t, domain, notAfter)
	entry := traefikRawCert{}
	entry.Domain.Main = domain
	entry.Domain.SANs = []string{"www." + domain}
	entry.Certificate = base64.StdEncoding.EncodeToString(pemBytes)
	payload, _ := json.Marshal([]traefikRawCert{entry})
	return payload
}

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

func TestTraefikClient_FetchCerts_ParsesCorrectly(t *testing.T) {
	notAfter := time.Now().Add(45 * 24 * time.Hour).UTC()
	payload := traefikCertPayload(t, "example.com", notAfter)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tls/certificates" {
			w.Header().Set("Content-Type", "application/json")
			w.Write(payload) //nolint:errcheck
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewTraefikClient(srv.URL, "")
	certs, err := client.FetchCerts(context.Background())
	if err != nil {
		t.Fatalf("FetchCerts: %v", err)
	}

	if len(certs) != 1 {
		t.Fatalf("expected 1 cert, got %d", len(certs))
	}
	c := certs[0]
	if c.Domain != "example.com" {
		t.Errorf("domain: got %q, want %q", c.Domain, "example.com")
	}
	if c.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be set")
	}
	// Days should be around 45.
	days := int(time.Until(*c.ExpiresAt).Hours() / 24)
	if days < 44 || days > 46 {
		t.Errorf("expected ~45 days remaining, got %d", days)
	}
	// SANs should include www.example.com
	found := false
	for _, s := range c.SANs {
		if s == "www.example.com" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected www.example.com in SANs, got %v", c.SANs)
	}
}

func TestTraefikClient_FetchCerts_EmptyList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]")) //nolint:errcheck
	}))
	defer srv.Close()

	client := NewTraefikClient(srv.URL, "")
	certs, err := client.FetchCerts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(certs) != 0 {
		t.Errorf("expected 0 certs, got %d", len(certs))
	}
}

func TestTraefikClient_FetchCerts_SkipsMalformedEntry(t *testing.T) {
	// One good cert + one entry with garbage certificate data.
	goodPayload := traefikCertPayload(t, "good.com", time.Now().Add(30*24*time.Hour))

	var entries []traefikRawCert
	_ = json.Unmarshal(goodPayload, &entries)
	entries = append(entries, traefikRawCert{Certificate: "not-base64-or-pem!!!!"})
	payload, _ := json.Marshal(entries)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(payload) //nolint:errcheck
	}))
	defer srv.Close()

	client := NewTraefikClient(srv.URL, "")
	certs, err := client.FetchCerts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only the good cert should be returned.
	if len(certs) != 1 {
		t.Errorf("expected 1 cert (malformed entry skipped), got %d", len(certs))
	}
}

func TestTraefikClient_APIKey_SentAsAuthHeader(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]")) //nolint:errcheck
	}))
	defer srv.Close()

	client := NewTraefikClient(srv.URL, "secret-key")
	_, _ = client.FetchCerts(context.Background())

	if gotHeader != "secret-key" {
		t.Errorf("expected Authorization=secret-key, got %q", gotHeader)
	}
}

// ── parseCert unit test ───────────────────────────────────────────────────────

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

// ── FetchRouters pagination tests ─────────────────────────────────────────────

func TestTraefikClient_FetchRouters_Pagination(t *testing.T) {
	// Build a full page of 100 routers to trigger page 2 fetch.
	page1 := make([]map[string]interface{}, 100)
	for i := range page1 {
		page1[i] = map[string]interface{}{
			"name": fmt.Sprintf("router%d", i+1), "rule": fmt.Sprintf("Host(`r%d.com`)", i+1),
			"service": "svc@docker", "status": "enabled", "provider": "docker",
			"entryPoints": []string{"websecure"},
		}
	}
	page2 := []map[string]interface{}{
		{"name": "router-last", "rule": `Host(` + "`" + `last.com` + "`" + `)`, "service": "svc-last@docker", "status": "disabled", "provider": "docker", "entryPoints": []string{"websecure"}},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/http/routers" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		if page == "1" || page == "" {
			json.NewEncoder(w).Encode(page1) //nolint:errcheck
		} else {
			json.NewEncoder(w).Encode(page2) //nolint:errcheck
		}
	}))
	defer srv.Close()

	client := NewTraefikClient(srv.URL, "")
	routers, err := client.FetchRouters(context.Background())
	if err != nil {
		t.Fatalf("FetchRouters: %v", err)
	}
	if len(routers) != 101 {
		t.Errorf("expected 101 routers (paginated), got %d", len(routers))
	}
	if routers[100].Name != "router-last" {
		t.Errorf("expected last router to be router-last, got %q", routers[100].Name)
	}
	if routers[100].Status != "disabled" {
		t.Errorf("expected router-last status=disabled, got %q", routers[100].Status)
	}
}

// TestTraefikClient_FetchRouters_400OnPage2 verifies that a 400 on page 2
// (as Traefik returns when all items fit on page 1) is handled gracefully.
func TestTraefikClient_FetchRouters_400OnPage2(t *testing.T) {
	page1 := make([]map[string]interface{}, 100)
	for i := range page1 {
		page1[i] = map[string]interface{}{
			"name": fmt.Sprintf("router%d", i+1), "rule": fmt.Sprintf("Host(`r%d.com`)", i+1),
			"service": "svc@docker", "status": "enabled", "provider": "docker",
			"entryPoints": []string{"websecure"},
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/http/routers" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		page := r.URL.Query().Get("page")
		if page == "1" || page == "" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(page1) //nolint:errcheck
		} else {
			// Traefik returns 400 when requesting a non-existent page.
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	client := NewTraefikClient(srv.URL, "")
	routers, err := client.FetchRouters(context.Background())
	if err != nil {
		t.Fatalf("FetchRouters: unexpected error on 400 page 2: %v", err)
	}
	if len(routers) != 100 {
		t.Errorf("expected 100 routers, got %d", len(routers))
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

// ── parseCert unit test ───────────────────────────────────────────────────────

func TestParseCert_ExtractsDomain(t *testing.T) {
	notAfter := time.Now().Add(90 * 24 * time.Hour)
	pemBytes, x := selfSignedCertPEM(t, "myservice.home", notAfter)

	raw := traefikRawCert{}
	raw.Domain.Main = "myservice.home"
	raw.Certificate = base64.StdEncoding.EncodeToString(pemBytes)

	cert, err := parseCert(raw)
	if err != nil {
		t.Fatalf("parseCert: %v", err)
	}
	if cert.Domain != "myservice.home" {
		t.Errorf("domain: got %q", cert.Domain)
	}
	if cert.ExpiresAt == nil {
		t.Fatal("ExpiresAt is nil")
	}
	diff := cert.ExpiresAt.Sub(x.NotAfter)
	if diff < 0 {
		diff = -diff
	}
	if diff > time.Second {
		t.Errorf("ExpiresAt mismatch: cert=%v parsed=%v", x.NotAfter, cert.ExpiresAt)
	}
}
