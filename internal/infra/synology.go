package infra

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/google/uuid"
)

// SynologyCredentials is the JSON shape stored in infrastructure_components.credentials.
type SynologyCredentials struct {
	BaseURL   string `json:"base_url"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	VerifyTLS bool   `json:"verify_tls"`
}

// SynologyPoller polls a single Synology DSM instance, writing resource_readings
// and generating events for degraded disk health. The session ID is reused across
// poll cycles; a re-login occurs automatically on session expiry (error code 119).
type SynologyPoller struct {
	componentID string
	creds       SynologyCredentials
	client      *http.Client

	mu  sync.Mutex
	sid string // empty means not yet authenticated
}

// NewSynologyPoller creates a SynologyPoller from a component ID and credentials JSON.
func NewSynologyPoller(componentID, credJSON string) (*SynologyPoller, error) {
	var creds SynologyCredentials
	if err := json.Unmarshal([]byte(credJSON), &creds); err != nil {
		return nil, fmt.Errorf("parse synology credentials: %w", err)
	}

	transport := &http.Transport{}
	if !creds.VerifyTLS {
		log.Printf("synology poller %s: TLS verification disabled (verify_tls=false)", componentID)
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}

	return &SynologyPoller{
		componentID: componentID,
		creds:       creds,
		client: &http.Client{
			Transport: transport,
			Timeout:   15 * time.Second,
		},
	}, nil
}

// ── API response shapes ───────────────────────────────────────────────────────

type synoEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *synoAPIError   `json:"error,omitempty"`
}

type synoAPIError struct {
	Code int `json:"code"`
}

type synoAuthData struct {
	SID string `json:"sid"`
}

type synoSystemInfo struct {
	CPUUserLoad float64 `json:"cpu_user_load"`
	RAMTotal    float64 `json:"ram_total"`
	RAMUsed     float64 `json:"ram_used"`
}

type synoStorageData struct {
	Volumes []synoVolume `json:"volumes"`
}

// synoVolume holds per-volume storage info. DSM returns byte counts as decimal
// strings (e.g. "1073741824000") to avoid JSON integer-overflow on large arrays.
type synoVolume struct {
	VolPath       string `json:"vol_path"`
	SizeTotalByte string `json:"size_total_byte"`
	SizeUsedByte  string `json:"size_used_byte"`
}

type synoDiskHealthData struct {
	Disks []synoDisk `json:"disks"`
}

type synoDisk struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// ── Authentication ────────────────────────────────────────────────────────────

// login authenticates with DSM and stores the returned session ID.
func (p *SynologyPoller) login(ctx context.Context) error {
	u, err := url.Parse(p.creds.BaseURL + "/webapi/auth.cgi")
	if err != nil {
		return fmt.Errorf("parse base URL: %w", err)
	}
	q := url.Values{}
	q.Set("api", "SYNO.API.Auth")
	q.Set("version", "3")
	q.Set("method", "login")
	q.Set("account", p.creds.Username)
	q.Set("passwd", p.creds.Password)
	q.Set("session", "NORA")
	q.Set("format", "cookie")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("build login request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("login request: %w", err)
	}
	defer resp.Body.Close()

	var env synoEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return fmt.Errorf("decode login response: %w", err)
	}
	if !env.Success {
		code := 0
		if env.Error != nil {
			code = env.Error.Code
		}
		return fmt.Errorf("login failed (error code %d)", code)
	}

	var authData synoAuthData
	if err := json.Unmarshal(env.Data, &authData); err != nil {
		return fmt.Errorf("decode auth data: %w", err)
	}
	if authData.SID == "" {
		return fmt.Errorf("login returned empty session ID")
	}

	p.mu.Lock()
	p.sid = authData.SID
	p.mu.Unlock()
	return nil
}

// Shutdown logs out of the active DSM session. Call this when stopping the poller.
func (p *SynologyPoller) Shutdown(ctx context.Context) {
	p.mu.Lock()
	sid := p.sid
	p.mu.Unlock()
	if sid == "" {
		return
	}

	u, err := url.Parse(p.creds.BaseURL + "/webapi/auth.cgi")
	if err != nil {
		log.Printf("synology poller %s: logout: parse URL: %v", p.componentID, err)
		return
	}
	q := url.Values{}
	q.Set("api", "SYNO.API.Auth")
	q.Set("version", "3")
	q.Set("method", "logout")
	q.Set("session", "NORA")
	q.Set("_sid", sid)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		log.Printf("synology poller %s: logout: build request: %v", p.componentID, err)
		return
	}
	resp, err := p.client.Do(req)
	if err != nil {
		log.Printf("synology poller %s: logout: %v", p.componentID, err)
		return
	}
	resp.Body.Close()
	log.Printf("synology poller %s: logged out", p.componentID)
}

// ── HTTP helper ───────────────────────────────────────────────────────────────

// get performs an authenticated GET to entry.cgi and decodes the response data
// into out. It automatically re-authenticates once on session expiry (code 119).
func (p *SynologyPoller) get(ctx context.Context, params url.Values, out interface{}) error {
	p.mu.Lock()
	sid := p.sid
	p.mu.Unlock()

	if sid == "" {
		if err := p.login(ctx); err != nil {
			return fmt.Errorf("authenticate: %w", err)
		}
		p.mu.Lock()
		sid = p.sid
		p.mu.Unlock()
	}

	env, err := p.doGet(ctx, sid, params)
	if err != nil {
		return err
	}

	// Re-authenticate on session expiry and retry once.
	if !env.Success && env.Error != nil && env.Error.Code == 119 {
		log.Printf("synology poller %s: session expired, re-authenticating", p.componentID)
		if authErr := p.login(ctx); authErr != nil {
			return fmt.Errorf("re-authenticate: %w", authErr)
		}
		p.mu.Lock()
		sid = p.sid
		p.mu.Unlock()

		env, err = p.doGet(ctx, sid, params)
		if err != nil {
			return err
		}
	}

	if !env.Success {
		code := 0
		if env.Error != nil {
			code = env.Error.Code
		}
		return fmt.Errorf("API call failed (error code %d)", code)
	}

	return json.Unmarshal(env.Data, out)
}

func (p *SynologyPoller) doGet(ctx context.Context, sid string, params url.Values) (*synoEnvelope, error) {
	u, err := url.Parse(p.creds.BaseURL + "/webapi/entry.cgi")
	if err != nil {
		return nil, fmt.Errorf("parse entry URL: %w", err)
	}
	params.Set("_sid", sid)
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET entry.cgi: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var env synoEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &env, nil
}

// ── Poll ──────────────────────────────────────────────────────────────────────

// Poll runs one full cycle: system resources, volume usage, and disk health.
// Returns an error if the system-resources call fails (indicating the host is
// unreachable). Partial failures on subsequent calls set status="degraded".
func (p *SynologyPoller) Poll(ctx context.Context, store *repo.Store) error {
	now := time.Now().UTC()

	// System resources is the primary reachability test.
	if err := p.pollSystemResources(ctx, store, now); err != nil {
		return fmt.Errorf("system resources: %w", err)
	}

	degraded := false

	if err := p.pollVolumeUsage(ctx, store, now); err != nil {
		log.Printf("synology poller %s: volume usage: %v", p.componentID, err)
		degraded = true
	}

	diskDegraded, err := p.pollDiskHealth(ctx, store, now)
	if err != nil {
		log.Printf("synology poller %s: disk health: %v", p.componentID, err)
		degraded = true
	}

	status := "online"
	if degraded || diskDegraded {
		status = "degraded"
	}
	polledAt := now.Format(time.RFC3339Nano)
	if err := store.InfraComponents.UpdateStatus(ctx, p.componentID, status, polledAt); err != nil {
		log.Printf("synology poller %s: update status: %v", p.componentID, err)
	}
	return nil
}

func (p *SynologyPoller) pollSystemResources(ctx context.Context, store *repo.Store, now time.Time) error {
	params := url.Values{}
	params.Set("api", "SYNO.Core.System")
	params.Set("version", "1")
	params.Set("method", "info")

	var info synoSystemInfo
	if err := p.get(ctx, params, &info); err != nil {
		return err
	}

	var memPercent float64
	if info.RAMTotal > 0 {
		memPercent = (info.RAMUsed / info.RAMTotal) * 100
	}

	for _, m := range []struct {
		metric string
		value  float64
	}{
		{"cpu_percent", info.CPUUserLoad},
		{"mem_percent", memPercent},
	} {
		reading := &models.ResourceReading{
			ID:         uuid.New().String(),
			SourceID:   p.componentID,
			SourceType: "synology",
			Metric:     m.metric,
			Value:      m.value,
			RecordedAt: now,
		}
		if err := store.Resources.Create(ctx, reading); err != nil {
			log.Printf("synology poller %s: write %s: %v", p.componentID, m.metric, err)
		}
	}
	return nil
}

func (p *SynologyPoller) pollVolumeUsage(ctx context.Context, store *repo.Store, now time.Time) error {
	params := url.Values{}
	params.Set("api", "SYNO.Storage.CGI.Storage")
	params.Set("version", "1")
	params.Set("method", "load_info")

	var storageData synoStorageData
	if err := p.get(ctx, params, &storageData); err != nil {
		return err
	}

	for _, vol := range storageData.Volumes {
		total, err := strconv.ParseInt(vol.SizeTotalByte, 10, 64)
		if err != nil || total == 0 {
			continue
		}
		used, err := strconv.ParseInt(vol.SizeUsedByte, 10, 64)
		if err != nil {
			continue
		}

		diskPercent := (float64(used) / float64(total)) * 100
		// Sanitise vol_path to a metric-safe key: "/volume1" → "volume1".
		volKey := strings.TrimLeft(vol.VolPath, "/")
		if volKey == "" {
			volKey = "unknown"
		}

		reading := &models.ResourceReading{
			ID:         uuid.New().String(),
			SourceID:   p.componentID,
			SourceType: "synology",
			Metric:     "disk_percent_" + volKey,
			Value:      diskPercent,
			RecordedAt: now,
		}
		if err := store.Resources.Create(ctx, reading); err != nil {
			log.Printf("synology poller %s: write disk_percent_%s: %v", p.componentID, volKey, err)
		}
	}
	return nil
}

// pollDiskHealth checks disk status and fires events for non-normal disks.
// Returns (degraded, error) — degraded is true if any disk is not "normal".
func (p *SynologyPoller) pollDiskHealth(ctx context.Context, store *repo.Store, now time.Time) (bool, error) {
	params := url.Values{}
	params.Set("api", "SYNO.Storage.CGI.DiskHealth")
	params.Set("version", "1")
	params.Set("method", "load_info")

	var healthData synoDiskHealthData
	if err := p.get(ctx, params, &healthData); err != nil {
		return false, err
	}

	degraded := false
	for _, disk := range healthData.Disks {
		if disk.Status == "normal" {
			continue
		}
		degraded = true

		severity := "warn"
		if disk.Status == "critical" || disk.Status == "failing" {
			severity = "error"
		}

		rawPayload, _ := json.Marshal(disk)
		event := &models.Event{
			ID:          uuid.New().String(),
			AppID:       "",
			ReceivedAt:  now,
			Severity:    severity,
			DisplayText: fmt.Sprintf("Synology disk %s status: %s", disk.ID, disk.Status),
			RawPayload:  string(rawPayload),
			Fields:      fmt.Sprintf(`{"source":"synology","disk_id":%q,"status":%q}`, disk.ID, disk.Status),
		}
		if err := store.Events.Create(ctx, event); err != nil {
			log.Printf("synology poller %s: create disk health event for %s: %v",
				p.componentID, disk.ID, err)
		}
	}
	return degraded, nil
}
