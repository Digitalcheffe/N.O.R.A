// Package infra provides clients for infrastructure integrations (Traefik, etc.).
package infra

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
)

// TraefikClient calls the Traefik dashboard API to discover TLS certificates
// and HTTP routers. It carries no state between calls and is safe for
// concurrent use.
type TraefikClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// NewTraefikClient returns a client targeting the Traefik API at apiURL.
// If apiKey is non-empty it is sent as the Authorization header on every request.
func NewTraefikClient(apiURL, apiKey string) *TraefikClient {
	return &TraefikClient{
		baseURL: strings.TrimRight(apiURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Ping verifies connectivity by calling GET /api/overview.
func (c *TraefikClient) Ping(ctx context.Context) error {
	req, err := c.newRequest(ctx, "GET", "/api/overview")
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("traefik ping: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("traefik ping: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// traefikRawCert mirrors the JSON shape returned by GET /api/tls/certificates.
type traefikRawCert struct {
	Domain struct {
		Main string   `json:"main"`
		SANs []string `json:"sans"`
	} `json:"domain"`
	Certificate string `json:"certificate"` // base64-encoded PEM
}

// FetchCerts calls GET /api/tls/certificates, parses each entry through the
// x509 library, and returns a slice of TraefikCert ready for the cache.
func (c *TraefikClient) FetchCerts(ctx context.Context) ([]*models.TraefikCert, error) {
	req, err := c.newRequest(ctx, "GET", "/api/tls/certificates")
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("traefik fetch certs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("traefik fetch certs: unexpected status %d", resp.StatusCode)
	}

	var raw []traefikRawCert
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("traefik fetch certs: decode: %w", err)
	}

	out := make([]*models.TraefikCert, 0, len(raw))
	for _, r := range raw {
		cert, err := parseCert(r)
		if err != nil {
			// Skip malformed entries rather than failing the whole sync.
			continue
		}
		out = append(out, cert)
	}
	return out, nil
}

// traefikOverviewRaw mirrors the JSON shape returned by GET /api/overview.
type traefikOverviewRaw struct {
	Version string `json:"version"`
	HTTP    struct {
		Routers struct {
			Total    int `json:"total"`
			Warnings int `json:"warnings"`
			Errors   int `json:"errors"`
		} `json:"routers"`
		Services struct {
			Total  int `json:"total"`
			Errors int `json:"errors"`
		} `json:"services"`
		Middlewares struct {
			Total int `json:"total"`
		} `json:"middlewares"`
	} `json:"http"`
}

// FetchOverview calls GET /api/overview and returns the parsed result.
func (c *TraefikClient) FetchOverview(ctx context.Context) (*traefikOverviewRaw, error) {
	req, err := c.newRequest(ctx, "GET", "/api/overview")
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("traefik fetch overview: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("traefik fetch overview: unexpected status %d", resp.StatusCode)
	}
	var raw traefikOverviewRaw
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("traefik fetch overview: decode: %w", err)
	}
	return &raw, nil
}

// TraefikRouter is a view of a Traefik HTTP router entry.
type TraefikRouter struct {
	Name            string   `json:"name"`
	Rule            string   `json:"rule"`
	ServiceName     string   `json:"service"`
	Status          string   `json:"status"`
	Provider        string   `json:"provider"`
	EntryPoints     []string `json:"entryPoints"`
	TLSCertResolver string   `json:"tls_cert_resolver"` // flattened from tls.certResolver
}

// traefikRouterRaw is the full JSON shape from /api/http/routers before flattening.
type traefikRouterRaw struct {
	Name        string   `json:"name"`
	Rule        string   `json:"rule"`
	Service     string   `json:"service"`
	Status      string   `json:"status"`
	Provider    string   `json:"provider"`
	EntryPoints []string `json:"entryPoints"`
	TLS         *struct {
		CertResolver string `json:"certResolver"`
	} `json:"tls"`
}

// FetchRouters calls GET /api/http/routers, handling pagination, and returns all routers.
func (c *TraefikClient) FetchRouters(ctx context.Context) ([]TraefikRouter, error) {
	const perPage = 100
	var all []TraefikRouter
	page := 1
	for {
		path := fmt.Sprintf("/api/http/routers?page=%d&per_page=%d", page, perPage)
		req, err := c.newRequest(ctx, "GET", path)
		if err != nil {
			return nil, err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("traefik fetch routers page %d: %w", page, err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			// On page 2+, a non-OK response means no more data — return what we have.
			if page > 1 {
				break
			}
			return nil, fmt.Errorf("traefik fetch routers: unexpected status %d", resp.StatusCode)
		}
		var raw []traefikRouterRaw
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("traefik fetch routers page %d: decode: %w", page, err)
		}
		resp.Body.Close()

		for _, r := range raw {
			tr := TraefikRouter{
				Name:        r.Name,
				Rule:        r.Rule,
				ServiceName: r.Service,
				Status:      r.Status,
				Provider:    r.Provider,
				EntryPoints: r.EntryPoints,
			}
			if r.TLS != nil {
				tr.TLSCertResolver = r.TLS.CertResolver
			}
			all = append(all, tr)
		}
		// Only continue if the page was full — otherwise we have everything.
		if len(raw) < perPage {
			break
		}
		page++
	}
	return all, nil
}

// traefikServiceRaw is the JSON shape from /api/http/services.
type traefikServiceRaw struct {
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	Status       string            `json:"status"`
	Provider     string            `json:"provider"`
	ServerStatus map[string]string `json:"serverStatus"`
}

// TraefikServiceStatus is a parsed service entry with backend server health.
type TraefikServiceStatus struct {
	Name         string
	Type         string
	Status       string
	Provider     string
	ServerStatus map[string]string // server URL → "UP" | "DOWN"
}

// FetchServices calls GET /api/http/services, handling pagination, and returns all services.
func (c *TraefikClient) FetchServices(ctx context.Context) ([]TraefikServiceStatus, error) {
	const perPage = 100
	var all []TraefikServiceStatus
	page := 1
	for {
		path := fmt.Sprintf("/api/http/services?page=%d&per_page=%d", page, perPage)
		req, err := c.newRequest(ctx, "GET", path)
		if err != nil {
			return nil, err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("traefik fetch services page %d: %w", page, err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			// On page 2+, a non-OK response means no more data — return what we have.
			if page > 1 {
				break
			}
			return nil, fmt.Errorf("traefik fetch services: unexpected status %d", resp.StatusCode)
		}
		var raw []traefikServiceRaw
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("traefik fetch services page %d: decode: %w", page, err)
		}
		resp.Body.Close()

		for _, r := range raw {
			ss := map[string]string{}
			if r.ServerStatus != nil {
				ss = r.ServerStatus
			}
			all = append(all, TraefikServiceStatus{
				Name:         r.Name,
				Type:         r.Type,
				Status:       r.Status,
				Provider:     r.Provider,
				ServerStatus: ss,
			})
		}
		// Only continue if the page was full — otherwise we have everything.
		if len(raw) < perPage {
			break
		}
		page++
	}
	return all, nil
}

// newRequest builds an authenticated GET/POST request.
func (c *TraefikClient) newRequest(ctx context.Context, method, path string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", c.apiKey)
	}
	return req, nil
}

// parseCert extracts certificate metadata from a raw Traefik cert entry.
// The Certificate field is base64-encoded PEM (not DER).
func parseCert(r traefikRawCert) (*models.TraefikCert, error) {
	pemBytes, err := base64.StdEncoding.DecodeString(r.Certificate)
	if err != nil {
		// Some Traefik builds emit plain (non-base64) PEM.
		pemBytes = []byte(r.Certificate)
	}

	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("no PEM block in certificate data")
	}

	x509cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse x509: %w", err)
	}

	domain := r.Domain.Main
	if domain == "" {
		domain = x509cert.Subject.CommonName
	}

	sans := x509cert.DNSNames
	if sans == nil {
		sans = []string{}
	}

	issuer := strings.Join(x509cert.Issuer.Organization, ", ")
	if issuer == "" {
		issuer = x509cert.Issuer.CommonName
	}

	expiresAt := x509cert.NotAfter.UTC()

	cert := &models.TraefikCert{
		Domain:    domain,
		SANs:      sans,
		ExpiresAt: &expiresAt,
	}
	if issuer != "" {
		cert.Issuer = &issuer
	}
	return cert, nil
}
