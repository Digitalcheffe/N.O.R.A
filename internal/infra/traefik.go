// Package infra provides clients for infrastructure integrations (Traefik, etc.).
package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
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

// FetchRouters calls GET /api/http/routers and returns all routers.
func (c *TraefikClient) FetchRouters(ctx context.Context) ([]TraefikRouter, error) {
	req, err := c.newRequest(ctx, "GET", "/api/http/routers")
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("traefik fetch routers: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("traefik fetch routers: unexpected status %d", resp.StatusCode)
	}
	var raw []traefikRouterRaw
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("traefik fetch routers: decode: %w", err)
	}
	out := make([]TraefikRouter, 0, len(raw))
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
		out = append(out, tr)
	}
	return out, nil
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

// FetchServices calls GET /api/http/services and returns all services.
func (c *TraefikClient) FetchServices(ctx context.Context) ([]TraefikServiceStatus, error) {
	req, err := c.newRequest(ctx, "GET", "/api/http/services")
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("traefik fetch services: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("traefik fetch services: unexpected status %d", resp.StatusCode)
	}
	var raw []traefikServiceRaw
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("traefik fetch services: decode: %w", err)
	}
	out := make([]TraefikServiceStatus, 0, len(raw))
	for _, r := range raw {
		ss := map[string]string{}
		if r.ServerStatus != nil {
			ss = r.ServerStatus
		}
		out = append(out, TraefikServiceStatus{
			Name:         r.Name,
			Type:         r.Type,
			Status:       r.Status,
			Provider:     r.Provider,
			ServerStatus: ss,
		})
	}
	return out, nil
}

// ResolveTraefikCreds extracts the Traefik API URL and optional API key from a
// component's stored credentials JSON.  If the credentials are absent or
// malformed, the URL falls back to http://{ip}:8080 with an empty key.
func ResolveTraefikCreds(ip string, credJSON *string) (apiURL, apiKey string) {
	if credJSON != nil && *credJSON != "" {
		var creds struct {
			APIURL string `json:"api_url"`
			APIKey string `json:"api_key"`
		}
		if err := json.Unmarshal([]byte(*credJSON), &creds); err == nil && creds.APIURL != "" {
			return creds.APIURL, creds.APIKey
		}
	}
	return "http://" + ip + ":8080", ""
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

