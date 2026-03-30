package metrics

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

// opnsenseCreds is the JSON shape stored in infrastructure_components.credentials
// for opnsense components (shared with the discovery scanner).
type opnsenseCreds struct {
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
	APISecret string `json:"api_secret"`
	VerifyTLS bool   `json:"verify_tls"`
}

// OPNsenseMetricsScanner collects CPU%, memory%, and interface statistics from
// an OPNsense firewall via its REST API every MetricsInterval.
type OPNsenseMetricsScanner struct {
	store   *repo.Store
	tracker ThresholdTracker
}

// NewOPNsenseMetricsScanner returns an OPNsenseMetricsScanner backed by store.
func NewOPNsenseMetricsScanner(store *repo.Store) *OPNsenseMetricsScanner {
	return &OPNsenseMetricsScanner{
		store:   store,
		tracker: newThresholdTracker(),
	}
}

// CollectMetrics fetches CPU%, memory%, and interface rx/tx from OPNsense and
// writes them to resource_readings. Fires threshold events on CPU > 90% and
// memory > 90%.
func (s *OPNsenseMetricsScanner) CollectMetrics(ctx context.Context, entityID string, entityType string) (*scanner.MetricsResult, error) {
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
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	now := time.Now().UTC()
	readings := 0

	// ── System resources (CPU load + memory) ──────────────────────────────────
	cpuPct, memPct, err := s.fetchSystemResources(ctx, client, creds)
	if err != nil {
		log.Printf("opnsense metrics %s: system resources: %v (non-fatal)", c.Name, err)
	} else {
		writeReading(ctx, s.store, entityID, "opnsense", "cpu_percent", cpuPct, now)
		writeReading(ctx, s.store, entityID, "opnsense", "mem_percent", memPct, now)
		readings += 2

		s.tracker.CheckAndFire(ctx, s.store, entityID, c.Name, "physical_host", "cpu_percent",
			cpuThreshold(cpuPct),
			func(l thresholdLevel) string {
				if l == levelNormal {
					return fmt.Sprintf("[metrics] CPU recovered — %s: %.1f%%", c.Name, cpuPct)
				}
				return fmt.Sprintf("[metrics] High CPU — %s: %.1f%%", c.Name, cpuPct)
			},
		)
		s.tracker.CheckAndFire(ctx, s.store, entityID, c.Name, "physical_host", "mem_percent",
			memThreshold(memPct),
			func(l thresholdLevel) string {
				if l == levelNormal {
					return fmt.Sprintf("[metrics] Memory recovered — %s: %.1f%%", c.Name, memPct)
				}
				return fmt.Sprintf("[metrics] High memory — %s: %.1f%%", c.Name, memPct)
			},
		)
	}

	// ── Interface statistics (rx/tx bytes) ────────────────────────────────────
	ifaceStats, err := s.fetchInterfaceStats(ctx, client, creds)
	if err != nil {
		log.Printf("opnsense metrics %s: interface stats: %v (non-fatal)", c.Name, err)
	} else {
		var totalRx, totalTx float64
		for _, iface := range ifaceStats {
			totalRx += iface.rxBytes
			totalTx += iface.txBytes
		}
		writeReading(ctx, s.store, entityID, "opnsense", "net_rx_bytes", totalRx, now)
		writeReading(ctx, s.store, entityID, "opnsense", "net_tx_bytes", totalTx, now)
		readings += 2
	}

	log.Printf("opnsense metrics: %s: %d readings", c.Name, readings)

	return &scanner.MetricsResult{
		EntityID:   entityID,
		EntityType: entityType,
		Readings:   readings,
	}, nil
}

// opnsenseSystemResources is the JSON shape from GET /api/diagnostics/system/systemResources.
// Field names are best-effort based on OPNsense 23.x+ API.
type opnsenseSystemResources struct {
	CPU struct {
		Usage float64 `json:"usage"` // 0–100 on some versions
	} `json:"cpu"`
	Memory struct {
		Used  float64 `json:"used"`  // bytes
		Total float64 `json:"total"` // bytes
	} `json:"memory"`
	// Older firmware returns load averages instead of a direct usage field.
	LoadAverages []string `json:"load_averages"`
}

func (s *OPNsenseMetricsScanner) fetchSystemResources(ctx context.Context, client *http.Client, creds opnsenseCreds) (cpuPct, memPct float64, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		creds.BaseURL+"/api/diagnostics/system/systemResources", nil)
	if err != nil {
		return 0, 0, err
	}
	req.SetBasicAuth(creds.APIKey, creds.APISecret)

	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("GET systemResources: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	var res opnsenseSystemResources
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return 0, 0, fmt.Errorf("decode systemResources: %w", err)
	}

	// CPU usage — use direct field if present, otherwise derive from 1-min load average.
	cpuPct = res.CPU.Usage
	if cpuPct == 0 && len(res.LoadAverages) > 0 {
		// Approximate: load avg / 1 CPU (conservative estimate; better than 0).
		var load float64
		fmt.Sscanf(res.LoadAverages[0], "%f", &load)
		cpuPct = load * 100
		if cpuPct > 100 {
			cpuPct = 100
		}
	}

	// Memory percent
	if res.Memory.Total > 0 {
		memPct = (res.Memory.Used / res.Memory.Total) * 100
	}

	return cpuPct, memPct, nil
}

// opnsenseIfaceStat holds rx/tx bytes for a single interface.
type opnsenseIfaceStat struct {
	name    string
	rxBytes float64
	txBytes float64
}

// opnsenseIfaceStatResponse is the JSON from GET /api/diagnostics/interface/getInterfaceStatistics.
type opnsenseIfaceStatResponse map[string]struct {
	BytesReceived  float64 `json:"bytes_received"`
	BytesSent      float64 `json:"bytes_sent"`
}

func (s *OPNsenseMetricsScanner) fetchInterfaceStats(ctx context.Context, client *http.Client, creds opnsenseCreds) ([]opnsenseIfaceStat, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		creds.BaseURL+"/api/diagnostics/interface/getInterfaceStatistics", nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(creds.APIKey, creds.APISecret)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET getInterfaceStatistics: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	var raw opnsenseIfaceStatResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode interface stats: %w", err)
	}

	var stats []opnsenseIfaceStat
	for name, s := range raw {
		stats = append(stats, opnsenseIfaceStat{
			name:    name,
			rxBytes: s.BytesReceived,
			txBytes: s.BytesSent,
		})
	}
	return stats, nil
}

// compile-time interface check.
var _ scanner.MetricsScanner = (*OPNsenseMetricsScanner)(nil)
