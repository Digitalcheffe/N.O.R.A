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

// SynologyMeta is the rich snapshot stored in synology_meta and returned by the
// detail API endpoint. It is populated by the poller on every poll cycle.
type SynologyMeta struct {
	Model        string           `json:"model"`
	DSMVersion   string           `json:"dsm_version"`
	Hostname     string           `json:"hostname"`
	Uptime       string           `json:"uptime"`
	UptimeSecs   int64            `json:"uptime_secs"`
	TemperatureC int              `json:"temperature_c"`
	CPUPercent   float64          `json:"cpu_percent"`
	Memory       SynologyMemory   `json:"memory"`
	Volumes      []SynologyVolume `json:"volumes"`
	Disks        []SynologyDisk   `json:"disks"`
	Update       SynologyUpdate   `json:"update"`
	PolledAt     string           `json:"polled_at"`
}

// SynologyMemory is the memory sub-object within SynologyMeta.
type SynologyMemory struct {
	UsedBytes  int64   `json:"used_bytes"`
	TotalBytes int64   `json:"total_bytes"`
	Percent    float64 `json:"percent"`
}

// SynologyVolume is one entry in SynologyMeta.Volumes.
type SynologyVolume struct {
	Path       string  `json:"path"`
	Status     string  `json:"status"`
	UsedBytes  int64   `json:"used_bytes"`
	TotalBytes int64   `json:"total_bytes"`
	Percent    float64 `json:"percent"`
}

// SynologyDisk is one entry in SynologyMeta.Disks.
type SynologyDisk struct {
	Slot         int    `json:"slot"`
	Model        string `json:"model"`
	TemperatureC int    `json:"temperature_c"`
	Status       string `json:"status"`
}

// SynologyUpdate is the DSM update state in SynologyMeta.
type SynologyUpdate struct {
	Available bool   `json:"available"`
	Version   string `json:"version"`
}

// SynologyPoller polls a single Synology DSM instance, writing resource_readings
// and generating events for state changes. The session ID is cached and reused
// across poll cycles; re-authentication occurs on session expiry (>6 days) or
// when the API returns an auth error.
type SynologyPoller struct {
	componentID string
	creds       SynologyCredentials
	client      *http.Client

	mu              sync.Mutex
	sid             string
	authenticatedAt time.Time

	// State tracking for transition-based event generation.
	volumeStates    sync.Map // vol_path (string) → last known status (string)
	diskStates      sync.Map // slot key (string) → last known status (string)
	diskTempFlagged sync.Map // slot key (string) → bool (currently above 50°C threshold)
	lastUpdateVer   sync.Map // key "ver" → last seen available version string
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

// SYNO.Core.System method=info — system identity
type synoCoreSystemInfo struct {
	Model       string `json:"model"`
	FirmwareVer string `json:"firmware_ver"`
	HostName    string `json:"host_name"`
	UpTime      int64  `json:"up_time"`    // seconds
	Temperature int    `json:"temperature"` // Celsius
}

// SYNO.Core.System.Utilization method=get — CPU and memory utilization
type synoUtilization struct {
	CPU    synoUtilCPU    `json:"cpu"`
	Memory synoUtilMemory `json:"memory"`
}

type synoUtilCPU struct {
	UserLoad float64 `json:"user_load"` // 0–100
}

type synoUtilMemory struct {
	RealUsage float64 `json:"real_usage"` // percent 0–100
	RealTotal int64   `json:"real_total"` // KB
	AvailReal int64   `json:"avail_real"` // KB
}

// SYNO.Core.System method=info type=storage — volume health
type synoCoreSystemStorage struct {
	VolInfo []synoVolumeInfo `json:"vol_info"`
}

type synoVolumeInfo struct {
	VolPath   string `json:"vol_path"`
	Status    string `json:"status"`    // normal|degraded|crashed
	TotalSize string `json:"total_size"` // decimal byte count string
	UsedSize  string `json:"used_size"`  // decimal byte count string
}

// SYNO.Storage.CGI.Storage method=load_info — physical disk details
type synoStorageDiskData struct {
	Disks []synoStorageDisk `json:"disks"`
}

type synoStorageDisk struct {
	Slot   int    `json:"slot"`
	Model  string `json:"model"`
	Temp   int    `json:"temp"`   // Celsius
	Status string `json:"status"` // normal|warning|critical
}

// SYNO.Core.Upgrade method=check — DSM update availability
type synoUpgradeData struct {
	Available bool   `json:"update_available"`
	Version   string `json:"version"`
}

// ── Authentication ────────────────────────────────────────────────────────────

// login authenticates with DSM via entry.cgi and caches the returned SID.
func (p *SynologyPoller) login(ctx context.Context) error {
	u, err := url.Parse(p.creds.BaseURL + "/webapi/entry.cgi")
	if err != nil {
		return fmt.Errorf("parse base URL: %w", err)
	}
	q := url.Values{}
	q.Set("api", "SYNO.API.Auth")
	q.Set("version", "6")
	q.Set("method", "login")
	q.Set("account", p.creds.Username)
	q.Set("passwd", p.creds.Password)
	q.Set("format", "sid")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil)
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
	p.authenticatedAt = time.Now().UTC()
	p.mu.Unlock()
	log.Printf("synology poller %s: authenticated (new SID)", p.componentID)
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

	u, err := url.Parse(p.creds.BaseURL + "/webapi/entry.cgi")
	if err != nil {
		log.Printf("synology poller %s: logout: parse URL: %v", p.componentID, err)
		return
	}
	q := url.Values{}
	q.Set("api", "SYNO.API.Auth")
	q.Set("version", "6")
	q.Set("method", "logout")
	q.Set("_sid", sid)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil)
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

// sidExpired returns true if re-authentication is needed (no SID or SID > 6 days old).
func (p *SynologyPoller) sidExpired() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sid == "" {
		return true
	}
	return time.Since(p.authenticatedAt) > 6*24*time.Hour
}

// get performs an authenticated GET to entry.cgi and decodes the response data
// into out. It re-authenticates proactively if the SID is > 6 days old, and
// retries once on session error codes (105, 106, 119).
func (p *SynologyPoller) get(ctx context.Context, params url.Values, out interface{}) error {
	if p.sidExpired() {
		if err := p.login(ctx); err != nil {
			return fmt.Errorf("authenticate: %w", err)
		}
	}

	p.mu.Lock()
	sid := p.sid
	p.mu.Unlock()

	env, err := p.doGet(ctx, sid, params)
	if err != nil {
		return err
	}

	// Re-authenticate on session errors and retry once.
	if !env.Success && env.Error != nil &&
		(env.Error.Code == 119 || env.Error.Code == 105 || env.Error.Code == 106) {
		log.Printf("synology poller %s: session error (code %d), re-authenticating", p.componentID, env.Error.Code)
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

// Poll runs one full cycle: system info, CPU/memory, volumes, disks, and updates.
// Returns an error only if system info fails (host unreachable). Partial failures
// on subsequent calls set status="degraded" but do not abort the cycle.
func (p *SynologyPoller) Poll(ctx context.Context, store *repo.Store) error {
	now := time.Now().UTC()
	meta := &SynologyMeta{
		PolledAt: now.Format(time.RFC3339Nano),
		Volumes:  []SynologyVolume{},
		Disks:    []SynologyDisk{},
	}

	// System info is the primary reachability test.
	if err := p.fetchSystemInfo(ctx, meta); err != nil {
		return fmt.Errorf("system info: %w", err)
	}

	degraded := false

	if err := p.fetchUtilization(ctx, store, meta, now); err != nil {
		log.Printf("synology poller %s: utilization: %v", p.componentID, err)
		degraded = true
	}

	if err := p.fetchVolumes(ctx, store, meta, now); err != nil {
		log.Printf("synology poller %s: volumes: %v", p.componentID, err)
		degraded = true
	}

	diskDegraded, err := p.fetchDisks(ctx, store, meta, now)
	if err != nil {
		log.Printf("synology poller %s: disks: %v", p.componentID, err)
		degraded = true
	}

	if err := p.fetchUpdates(ctx, store, meta, now); err != nil {
		// Non-fatal — some DSM versions or permission sets restrict this API.
		log.Printf("synology poller %s: updates: %v (non-fatal)", p.componentID, err)
	}

	// Persist snapshot to synology_meta column.
	metaJSON, jsonErr := json.Marshal(meta)
	if jsonErr == nil {
		if storeErr := store.InfraComponents.UpdateSynologyMeta(ctx, p.componentID, string(metaJSON)); storeErr != nil {
			log.Printf("synology poller %s: store meta: %v", p.componentID, storeErr)
		}
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

// formatSynoUptime converts seconds to a human-readable string like "21d 4h 11m".
func formatSynoUptime(secs int64) string {
	if secs <= 0 {
		return "—"
	}
	d := secs / 86400
	h := (secs % 86400) / 3600
	m := (secs % 3600) / 60
	if d > 0 {
		return fmt.Sprintf("%dd %dh %dm", d, h, m)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

func (p *SynologyPoller) fetchSystemInfo(ctx context.Context, meta *SynologyMeta) error {
	params := url.Values{}
	params.Set("api", "SYNO.Core.System")
	params.Set("version", "1")
	params.Set("method", "info")

	var info synoCoreSystemInfo
	if err := p.get(ctx, params, &info); err != nil {
		return err
	}

	meta.Model = info.Model
	meta.DSMVersion = info.FirmwareVer
	meta.Hostname = info.HostName
	meta.UptimeSecs = info.UpTime
	meta.Uptime = formatSynoUptime(info.UpTime)
	meta.TemperatureC = info.Temperature
	return nil
}

func (p *SynologyPoller) fetchUtilization(ctx context.Context, store *repo.Store, meta *SynologyMeta, now time.Time) error {
	params := url.Values{}
	params.Set("api", "SYNO.Core.System.Utilization")
	params.Set("version", "1")
	params.Set("method", "get")

	var util synoUtilization
	if err := p.get(ctx, params, &util); err != nil {
		return err
	}

	meta.CPUPercent = util.CPU.UserLoad

	// RealTotal and AvailReal are in KB; convert to bytes.
	totalBytes := util.Memory.RealTotal * 1024
	availBytes := util.Memory.AvailReal * 1024
	usedBytes := totalBytes - availBytes
	if usedBytes < 0 {
		usedBytes = 0
	}
	meta.Memory = SynologyMemory{
		UsedBytes:  usedBytes,
		TotalBytes: totalBytes,
		Percent:    util.Memory.RealUsage,
	}

	for _, m := range []struct {
		metric string
		value  float64
	}{
		{"cpu_percent", util.CPU.UserLoad},
		{"mem_percent", util.Memory.RealUsage},
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

func (p *SynologyPoller) fetchVolumes(ctx context.Context, store *repo.Store, meta *SynologyMeta, now time.Time) error {
	params := url.Values{}
	params.Set("api", "SYNO.Core.System")
	params.Set("version", "1")
	params.Set("method", "info")
	params.Set("type", "storage")

	var storageData synoCoreSystemStorage
	if err := p.get(ctx, params, &storageData); err != nil {
		return err
	}

	for _, vol := range storageData.VolInfo {
		total, err := strconv.ParseInt(vol.TotalSize, 10, 64)
		if err != nil || total == 0 {
			continue
		}
		used, err := strconv.ParseInt(vol.UsedSize, 10, 64)
		if err != nil {
			continue
		}

		diskPct := (float64(used) / float64(total)) * 100
		// Sanitise vol_path to a metric-safe key: "/volume1" → "volume1".
		volKey := strings.TrimLeft(vol.VolPath, "/")
		if volKey == "" {
			volKey = "unknown"
		}

		meta.Volumes = append(meta.Volumes, SynologyVolume{
			Path:       vol.VolPath,
			Status:     vol.Status,
			UsedBytes:  used,
			TotalBytes: total,
			Percent:    diskPct,
		})

		reading := &models.ResourceReading{
			ID:         uuid.New().String(),
			SourceID:   p.componentID,
			SourceType: "synology",
			Metric:     "disk_percent_" + volKey,
			Value:      diskPct,
			RecordedAt: now,
		}
		if err := store.Resources.Create(ctx, reading); err != nil {
			log.Printf("synology poller %s: write disk_percent_%s: %v", p.componentID, volKey, err)
		}

		// Emit volume status change events (transitions only).
		lastStatusI, _ := p.volumeStates.Load(vol.VolPath)
		lastStatus, _ := lastStatusI.(string)
		if vol.Status != lastStatus {
			p.volumeStates.Store(vol.VolPath, vol.Status)
			if vol.Status == "degraded" || vol.Status == "crashed" {
				severity := "warn"
				displayText := fmt.Sprintf("Volume %s degraded", vol.VolPath)
				if vol.Status == "crashed" {
					severity = "error"
					displayText = fmt.Sprintf("Volume %s crashed", vol.VolPath)
				}
				rawPayload, _ := json.Marshal(vol)
				event := &models.Event{
					ID:          uuid.New().String(),
					ReceivedAt:  now,
					Severity:    severity,
					DisplayText: displayText,
					RawPayload:  string(rawPayload),
					Fields:      fmt.Sprintf(`{"source":"synology","component_id":%q,"vol_path":%q,"status":%q}`, p.componentID, vol.VolPath, vol.Status),
				}
				if err := store.Events.Create(ctx, event); err != nil {
					log.Printf("synology poller %s: create volume event for %s: %v", p.componentID, vol.VolPath, err)
				}
			}
		}
	}
	return nil
}

// fetchDisks fetches physical disk details and returns (diskDegraded, error).
func (p *SynologyPoller) fetchDisks(ctx context.Context, store *repo.Store, meta *SynologyMeta, now time.Time) (bool, error) {
	params := url.Values{}
	params.Set("api", "SYNO.Storage.CGI.Storage")
	params.Set("version", "1")
	params.Set("method", "load_info")

	var diskData synoStorageDiskData
	if err := p.get(ctx, params, &diskData); err != nil {
		return false, err
	}

	degraded := false
	for _, disk := range diskData.Disks {
		meta.Disks = append(meta.Disks, SynologyDisk{
			Slot:         disk.Slot,
			Model:        disk.Model,
			TemperatureC: disk.Temp,
			Status:       disk.Status,
		})

		if disk.Status != "normal" {
			degraded = true
		}

		slotKey := strconv.Itoa(disk.Slot)

		// Disk status transition events.
		lastStatusI, _ := p.diskStates.Load(slotKey)
		lastStatus, _ := lastStatusI.(string)
		if disk.Status != lastStatus {
			p.diskStates.Store(slotKey, disk.Status)
			if disk.Status == "warning" || disk.Status == "critical" {
				severity := "warn"
				displayText := fmt.Sprintf("Disk %d (%s) warning", disk.Slot, disk.Model)
				if disk.Status == "critical" {
					severity = "error"
					displayText = fmt.Sprintf("Disk %d (%s) critical", disk.Slot, disk.Model)
				}
				rawPayload, _ := json.Marshal(disk)
				event := &models.Event{
					ID:          uuid.New().String(),
					ReceivedAt:  now,
					Severity:    severity,
					DisplayText: displayText,
					RawPayload:  string(rawPayload),
					Fields:      fmt.Sprintf(`{"source":"synology","component_id":%q,"disk_slot":%d,"model":%q,"status":%q}`, p.componentID, disk.Slot, disk.Model, disk.Status),
				}
				if err := store.Events.Create(ctx, event); err != nil {
					log.Printf("synology poller %s: create disk status event for slot %d: %v", p.componentID, disk.Slot, err)
				}
			}
		}

		// Temperature threshold events: fire once when crossing >50°C, reset on cool-down.
		tempFlaggedI, _ := p.diskTempFlagged.Load(slotKey)
		wasFlagged, _ := tempFlaggedI.(bool)
		isFlagged := disk.Temp > 50
		if isFlagged && !wasFlagged {
			p.diskTempFlagged.Store(slotKey, true)
			rawPayload, _ := json.Marshal(disk)
			event := &models.Event{
				ID:          uuid.New().String(),
				ReceivedAt:  now,
				Severity:    "warn",
				DisplayText: fmt.Sprintf("Disk %d temp %d°C", disk.Slot, disk.Temp),
				RawPayload:  string(rawPayload),
				Fields:      fmt.Sprintf(`{"source":"synology","component_id":%q,"disk_slot":%d,"model":%q,"temp_c":%d}`, p.componentID, disk.Slot, disk.Model, disk.Temp),
			}
			if err := store.Events.Create(ctx, event); err != nil {
				log.Printf("synology poller %s: create disk temp event for slot %d: %v", p.componentID, disk.Slot, err)
			}
		} else if !isFlagged && wasFlagged {
			p.diskTempFlagged.Store(slotKey, false)
		}
	}
	return degraded, nil
}

func (p *SynologyPoller) fetchUpdates(ctx context.Context, store *repo.Store, meta *SynologyMeta, now time.Time) error {
	params := url.Values{}
	params.Set("api", "SYNO.Core.Upgrade")
	params.Set("version", "1")
	params.Set("method", "check")

	var upgrade synoUpgradeData
	if err := p.get(ctx, params, &upgrade); err != nil {
		return err
	}

	meta.Update = SynologyUpdate{
		Available: upgrade.Available,
		Version:   upgrade.Version,
	}

	// Fire one info event when a new available version is first seen.
	if upgrade.Available && upgrade.Version != "" {
		lastVerI, _ := p.lastUpdateVer.Load("ver")
		lastVer, _ := lastVerI.(string)
		if upgrade.Version != lastVer {
			p.lastUpdateVer.Store("ver", upgrade.Version)
			rawPayload, _ := json.Marshal(upgrade)
			event := &models.Event{
				ID:          uuid.New().String(),
				ReceivedAt:  now,
				Severity:    "info",
				DisplayText: fmt.Sprintf("DSM update available: %s", upgrade.Version),
				RawPayload:  string(rawPayload),
				Fields:      fmt.Sprintf(`{"source":"synology","component_id":%q,"update_version":%q}`, p.componentID, upgrade.Version),
			}
			if err := store.Events.Create(ctx, event); err != nil {
				log.Printf("synology poller %s: create update event: %v", p.componentID, err)
			}
		}
	} else if !upgrade.Available {
		// Reset so we fire again if a new update appears.
		p.lastUpdateVer.Store("ver", "")
	}
	return nil
}
