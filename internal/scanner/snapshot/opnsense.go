package snapshot

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

// OPNsenseSnapshotScanner captures the installed firmware version and available
// update state for an OPNsense firewall every SnapshotInterval.
type OPNsenseSnapshotScanner struct {
	store *repo.Store
}

// NewOPNsenseSnapshotScanner returns an OPNsenseSnapshotScanner backed by store.
func NewOPNsenseSnapshotScanner(store *repo.Store) *OPNsenseSnapshotScanner {
	return &OPNsenseSnapshotScanner{store: store}
}

// opnsenseCreds mirrors the credentials JSON shape used by the metrics scanner.
type opnsenseCreds struct {
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
	APISecret string `json:"api_secret"`
	VerifyTLS bool   `json:"verify_tls"`
}

// opnsenseFirmwareInfo is the JSON shape returned by GET /api/core/firmware/info.
type opnsenseFirmwareInfo struct {
	Product struct {
		ProductVersion string `json:"product_version"`
		ProductLatest  string `json:"product_latest"`
	} `json:"product"`
}

// TakeSnapshot fetches OPNsense firmware version and update state, then writes
// snapshot rows. Events fire on condition changes only.
func (s *OPNsenseSnapshotScanner) TakeSnapshot(ctx context.Context, entityID string, entityType string) (*scanner.SnapshotResult, error) {
	c, err := s.store.InfraComponents.Get(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("get component %s: %w", entityID, err)
	}
	if c.Credentials == nil || *c.Credentials == "" {
		return nil, fmt.Errorf("no credentials configured for %s", c.Name)
	}

	var creds opnsenseCreds
	if err := json.Unmarshal([]byte(*c.Credentials), &creds); err != nil {
		return nil, fmt.Errorf("parse opnsense credentials: %w", err)
	}

	transport := &http.Transport{}
	if !creds.VerifyTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second}

	info, err := s.fetchFirmwareInfo(ctx, client, creds)
	if err != nil {
		return nil, fmt.Errorf("fetch firmware info: %w", err)
	}

	now := time.Now().UTC()
	changed := false

	// ── Installed version ──────────────────────────────────────────────────────
	if info.Product.ProductVersion != "" {
		_, ch := captureSnapshot(ctx, s.store, "physical_host", entityID,
			"opnsense_version", info.Product.ProductVersion, now)
		if ch {
			changed = true
			writeSnapshotEvent(ctx, s.store, entityID, c.Name, "physical_host", "info",
				fmt.Sprintf("[snapshot] OPNsense updated — %s: %s",
					c.Name, info.Product.ProductVersion))
		}
	}

	// ── Update availability (product_latest != product_version) ───────────────
	updateAvail := "false"
	if info.Product.ProductLatest != "" &&
		info.Product.ProductLatest != info.Product.ProductVersion {
		updateAvail = info.Product.ProductLatest
	}
	prevUpd, ch := captureSnapshot(ctx, s.store, "physical_host", entityID,
		"update_available", updateAvail, now)
	if ch {
		changed = true
		if updateAvail != "false" && prevUpd == "false" {
			writeSnapshotEvent(ctx, s.store, entityID, c.Name, "physical_host", "info",
				fmt.Sprintf("[snapshot] OPNsense update available — %s: %s",
					c.Name, updateAvail))
		} else if updateAvail == "false" && prevUpd != "false" {
			writeSnapshotEvent(ctx, s.store, entityID, c.Name, "physical_host", "info",
				fmt.Sprintf("[snapshot] OPNsense update applied — %s", c.Name))
		}
	}

	writeDebugEvent(ctx, s.store, entityID, c.Name, "physical_host")
	log.Printf("opnsense snapshot: %s: done (changed=%v)", c.Name, changed)

	return &scanner.SnapshotResult{
		EntityID:   entityID,
		EntityType: entityType,
		Changed:    changed,
	}, nil
}

func (s *OPNsenseSnapshotScanner) fetchFirmwareInfo(
	ctx context.Context,
	client *http.Client,
	creds opnsenseCreds,
) (*opnsenseFirmwareInfo, error) {
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

	var info opnsenseFirmwareInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decode firmware info: %w", err)
	}
	return &info, nil
}

// compile-time interface check.
var _ scanner.SnapshotScanner = (*OPNsenseSnapshotScanner)(nil)
