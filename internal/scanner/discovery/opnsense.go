package discovery

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/internal/scanner"
)

// OPNsenseCredentials is the JSON shape stored in infrastructure_components.credentials
// for opnsense components.
type OPNsenseCredentials struct {
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
	APISecret string `json:"api_secret"`
	VerifyTLS bool   `json:"verify_tls"`
}

// OPNsenseDiscoveryScanner discovers configured interfaces, firewall rule
// summary, and installed plugins for an OPNsense component.
type OPNsenseDiscoveryScanner struct {
	store *repo.Store
}

// NewOPNsenseDiscoveryScanner returns an OPNsenseDiscoveryScanner backed by store.
func NewOPNsenseDiscoveryScanner(store *repo.Store) *OPNsenseDiscoveryScanner {
	return &OPNsenseDiscoveryScanner{store: store}
}

// Discover fetches interfaces and installed plugins from the OPNsense REST API
// and writes discovery events.
func (s *OPNsenseDiscoveryScanner) Discover(ctx context.Context, entityID string, entityType string) (*scanner.DiscoveryResult, error) {
	c, err := s.store.InfraComponents.Get(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("get component %s: %w", entityID, err)
	}
	if c.Credentials == nil || *c.Credentials == "" {
		return nil, fmt.Errorf("no credentials configured for %s", c.Name)
	}

	var creds OPNsenseCredentials
	if err := json.Unmarshal([]byte(*c.Credentials), &creds); err != nil {
		return nil, fmt.Errorf("parse opnsense credentials: %w", err)
	}

	transport := &http.Transport{}
	if !creds.VerifyTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
	}

	interfaceNames, err := s.fetchInterfaceNames(ctx, client, creds)
	if err != nil {
		return nil, fmt.Errorf("fetch interfaces: %w", err)
	}

	plugins, err := s.fetchInstalledPlugins(ctx, client, creds)
	if err != nil {
		// Non-fatal — older OPNsense versions may not expose this endpoint.
		log.Printf("opnsense discovery: fetch plugins %s: %v (non-fatal)", c.Name, err)
	}

	found := len(interfaceNames) + len(plugins)

	if found == 0 {
		writeDiscoveryEvent(ctx, s.store, entityID, c.Name, "physical_host", "debug",
			fmt.Sprintf("[discovery] %s discovery completed — no changes", c.Name))
	} else {
		writeDiscoveryEvent(ctx, s.store, entityID, c.Name, "physical_host", "info",
			fmt.Sprintf("[discovery] %s: %d interface(s), %d plugin(s) discovered",
				c.Name, len(interfaceNames), len(plugins)))
	}

	return &scanner.DiscoveryResult{
		EntityID:    entityID,
		EntityType:  entityType,
		Found:       found,
		Disappeared: 0,
	}, nil
}

// fetchInterfaceNames calls GET /api/diagnostics/interface/getInterfaceNames
// and returns the interface identifiers.
func (s *OPNsenseDiscoveryScanner) fetchInterfaceNames(ctx context.Context, client *http.Client, creds OPNsenseCredentials) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		creds.BaseURL+"/api/diagnostics/interface/getInterfaceNames", nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(creds.APIKey, creds.APISecret)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET /api/diagnostics/interface/getInterfaceNames: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	// Response is a JSON object: { "em0": "WAN", "em1": "LAN", ... }
	var ifaces map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&ifaces); err != nil {
		return nil, fmt.Errorf("decode interface names: %w", err)
	}

	names := make([]string, 0, len(ifaces))
	for k := range ifaces {
		names = append(names, k)
	}
	return names, nil
}

// fetchInstalledPlugins calls GET /api/core/firmware/info and returns the
// installed package count.
func (s *OPNsenseDiscoveryScanner) fetchInstalledPlugins(ctx context.Context, client *http.Client, creds OPNsenseCredentials) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		creds.BaseURL+"/api/core/firmware/info", nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(creds.APIKey, creds.APISecret)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET /api/core/firmware/info: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	var info struct {
		Product struct {
			Plugins []struct {
				Name string `json:"name"`
			} `json:"plugins"`
		} `json:"product"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decode firmware info: %w", err)
	}

	plugins := make([]string, 0, len(info.Product.Plugins))
	for _, p := range info.Product.Plugins {
		plugins = append(plugins, p.Name)
	}
	return plugins, nil
}

// compile-time check.
var _ scanner.DiscoveryScanner = (*OPNsenseDiscoveryScanner)(nil)
