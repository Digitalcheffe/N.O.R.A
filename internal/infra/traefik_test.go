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
