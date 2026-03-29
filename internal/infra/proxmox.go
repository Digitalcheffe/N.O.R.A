package infra

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/google/uuid"
)

// proxmoxChildNS is the fixed UUID v5 namespace used to derive stable child
// component IDs from a (parentComponentID, vmid) pair.
var proxmoxChildNS = uuid.MustParse("b5a4c3d2-1e0f-4d8c-9b7a-2c1d0e3f8a9b")

// proxmoxChildID returns a deterministic UUID for a VM or LXC container that
// belongs to a specific Proxmox component.  The same (parentID, vmid) pair
// always produces the same UUID, making upserts idempotent across polls.
func proxmoxChildID(parentID string, vmid int) string {
	return uuid.NewSHA1(proxmoxChildNS, []byte(fmt.Sprintf("%s/%d", parentID, vmid))).String()
}

// ProxmoxCredentials is the JSON shape stored in infrastructure_components.credentials.
type ProxmoxCredentials struct {
	BaseURL     string `json:"base_url"`
	TokenID     string `json:"token_id"`
	TokenSecret string `json:"token_secret"`
	VerifyTLS   bool   `json:"verify_tls"`
}

// ProxmoxPoller polls a single Proxmox VE instance, writing resource_readings
// and generating events for VM/LXC state changes.
type ProxmoxPoller struct {
	componentID string
	creds       ProxmoxCredentials
	client      *http.Client

	mu          sync.Mutex
	lastVMState map[string]string // key: "node/vmid", value: last known status
}

// NewProxmoxPoller creates a ProxmoxPoller from a component ID and credentials JSON.
// Logs a warning when verify_tls is false.
func NewProxmoxPoller(componentID, credJSON string) (*ProxmoxPoller, error) {
	var creds ProxmoxCredentials
	if err := json.Unmarshal([]byte(credJSON), &creds); err != nil {
		return nil, fmt.Errorf("parse proxmox credentials: %w", err)
	}

	transport := &http.Transport{}
	if !creds.VerifyTLS {
		log.Printf("proxmox poller %s: TLS verification disabled (verify_tls=false)", componentID)
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}

	return &ProxmoxPoller{
		componentID: componentID,
		creds:       creds,
		client: &http.Client{
			Transport: transport,
			Timeout:   15 * time.Second,
		},
		lastVMState: make(map[string]string),
	}, nil
}

// ── API response shapes ───────────────────────────────────────────────────────

type proxmoxEnvelope struct {
	Data json.RawMessage `json:"data"`
}

type proxmoxNode struct {
	Node   string `json:"node"`
	Status string `json:"status"`
}

type proxmoxNodeStatus struct {
	CPU    float64 `json:"cpu"`
	Memory struct {
		Used  float64 `json:"used"`
		Total float64 `json:"total"`
	} `json:"memory"`
}

type proxmoxStorage struct {
	Storage string  `json:"storage"`
	Used    float64 `json:"used"`
	Total   float64 `json:"total"`
	Active  int     `json:"active"`
}

type proxmoxVM struct {
	VMID   int    `json:"vmid"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

func (p *ProxmoxPoller) get(ctx context.Context, path string, out interface{}) error {
	url := p.creds.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request %s: %w", path, err)
	}
	req.Header.Set("Authorization",
		fmt.Sprintf("PVEAPIToken=%s=%s", p.creds.TokenID, p.creds.TokenSecret))

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: unexpected status %d", path, resp.StatusCode)
	}

	var env proxmoxEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return fmt.Errorf("decode response %s: %w", path, err)
	}
	return json.Unmarshal(env.Data, out)
}

// ── Poll ──────────────────────────────────────────────────────────────────────

// Poll runs one full poll cycle: fetches all nodes, records metrics, and fires
// VM/LXC state-change events. Returns an error only if the cluster is unreachable.
// Partial failures (single-node errors) result in status="degraded".
func (p *ProxmoxPoller) Poll(ctx context.Context, store *repo.Store) error {
	var nodes []proxmoxNode
	if err := p.get(ctx, "/api2/json/nodes", &nodes); err != nil {
		return fmt.Errorf("list nodes: %w", err)
	}

	partial := false
	now := time.Now().UTC()

	for _, node := range nodes {
		if err := p.pollNode(ctx, store, node.Node, now); err != nil {
			log.Printf("proxmox poller %s: node %s: %v", p.componentID, node.Node, err)
			partial = true
		}
	}

	status := "online"
	if partial {
		status = "degraded"
	}
	polledAt := now.Format(time.RFC3339Nano)
	if err := store.InfraComponents.UpdateStatus(ctx, p.componentID, status, polledAt); err != nil {
		log.Printf("proxmox poller %s: update status: %v", p.componentID, err)
	}

	return nil
}

func (p *ProxmoxPoller) pollNode(ctx context.Context, store *repo.Store, node string, now time.Time) error {
	// ── Node status (CPU + memory) ────────────────────────────────────────────
	var status proxmoxNodeStatus
	if err := p.get(ctx, fmt.Sprintf("/api2/json/nodes/%s/status", node), &status); err != nil {
		return fmt.Errorf("node status: %w", err)
	}

	cpuPercent := status.CPU * 100
	var memPercent float64
	if status.Memory.Total > 0 {
		memPercent = (status.Memory.Used / status.Memory.Total) * 100
	}

	for _, m := range []struct {
		metric string
		value  float64
	}{
		{"cpu_percent", cpuPercent},
		{"mem_percent", memPercent},
	} {
		reading := &models.ResourceReading{
			ID:         uuid.New().String(),
			SourceID:   p.componentID,
			SourceType: "proxmox_node",
			Metric:     m.metric,
			Value:      m.value,
			RecordedAt: now,
		}
		if err := store.Resources.Create(ctx, reading); err != nil {
			log.Printf("proxmox poller %s: write %s: %v", p.componentID, m.metric, err)
		}
	}

	// ── Storage (disk) ────────────────────────────────────────────────────────
	var storages []proxmoxStorage
	if err := p.get(ctx, fmt.Sprintf("/api2/json/nodes/%s/storage", node), &storages); err != nil {
		return fmt.Errorf("node storage: %w", err)
	}

	var usedTotal, sizeTotal float64
	for _, s := range storages {
		if s.Active == 0 {
			continue
		}
		usedTotal += s.Used
		sizeTotal += s.Total
	}
	var diskPercent float64
	if sizeTotal > 0 {
		diskPercent = (usedTotal / sizeTotal) * 100
	}
	diskReading := &models.ResourceReading{
		ID:         uuid.New().String(),
		SourceID:   p.componentID,
		SourceType: "proxmox_node",
		Metric:     "disk_percent",
		Value:      diskPercent,
		RecordedAt: now,
	}
	if err := store.Resources.Create(ctx, diskReading); err != nil {
		log.Printf("proxmox poller %s: write disk_percent: %v", p.componentID, err)
	}

	// ── VM inventory ──────────────────────────────────────────────────────────
	var vms []proxmoxVM
	if err := p.get(ctx, fmt.Sprintf("/api2/json/nodes/%s/qemu", node), &vms); err != nil {
		log.Printf("proxmox poller %s: list qemu on %s: %v", p.componentID, node, err)
	} else {
		p.checkStateChanges(ctx, store, node, "VM", vms)
		p.upsertChildren(ctx, store, vms, "vm", now)
	}

	var ctrs []proxmoxVM
	if err := p.get(ctx, fmt.Sprintf("/api2/json/nodes/%s/lxc", node), &ctrs); err != nil {
		log.Printf("proxmox poller %s: list lxc on %s: %v", p.componentID, node, err)
	} else {
		p.checkStateChanges(ctx, store, node, "LXC", ctrs)
		p.upsertChildren(ctx, store, ctrs, "lxc", now)
	}

	return nil
}

// upsertChildren creates or updates an InfrastructureComponent child record for
// each VM or LXC in the list.  Child IDs are derived deterministically from
// (componentID, vmid) so re-runs are fully idempotent.
func (p *ProxmoxPoller) upsertChildren(ctx context.Context, store *repo.Store, vms []proxmoxVM, kind string, now time.Time) {
	polledAt := now.Format(time.RFC3339Nano)

	for _, vm := range vms {
		id := proxmoxChildID(p.componentID, vm.VMID)

		status := "offline"
		if vm.Status == "running" {
			status = "online"
		}

		// Try an update-in-place first (fast path for subsequent polls).
		err := store.InfraComponents.UpdateStatus(ctx, id, status, polledAt)
		if err == nil {
			continue
		}
		if !errors.Is(err, repo.ErrNotFound) {
			log.Printf("proxmox poller %s: update child %s %q: %v", p.componentID, kind, vm.Name, err)
			continue
		}

		// First time seeing this VM/LXC — create a child component.
		parentID := p.componentID
		c := &models.InfrastructureComponent{
			ID:               id,
			Name:             vm.Name,
			IP:               "",
			Type:             kind,
			CollectionMethod: "none",
			ParentID:         &parentID,
			Enabled:          true,
			LastStatus:       status,
			CreatedAt:        polledAt,
		}
		if err := store.InfraComponents.Create(ctx, c); err != nil {
			log.Printf("proxmox poller %s: create child %s %q: %v", p.componentID, kind, vm.Name, err)
			continue
		}
		log.Printf("proxmox poller %s: discovered %s %q (vmid=%d, status=%s)",
			p.componentID, kind, vm.Name, vm.VMID, vm.Status)
	}
}

// checkStateChanges compares current VM/LXC statuses against the last known
// state and fires events for running↔stopped transitions.
func (p *ProxmoxPoller) checkStateChanges(ctx context.Context, store *repo.Store, node, kind string, vms []proxmoxVM) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, vm := range vms {
		key := fmt.Sprintf("%s/%d", node, vm.VMID)
		prev, seen := p.lastVMState[key]
		p.lastVMState[key] = vm.Status

		if !seen {
			continue
		}
		if prev == vm.Status {
			continue
		}

		// Fire events only on running↔stopped transitions.
		if !((prev == "running" && vm.Status == "stopped") ||
			(prev == "stopped" && vm.Status == "running")) {
			continue
		}

		severity := "info"
		if vm.Status == "stopped" {
			severity = "warn"
		}

		rawPayload, _ := json.Marshal(vm)

		event := &models.Event{
			ID:          uuid.New().String(),
			AppID:       "",
			ReceivedAt:  time.Now().UTC(),
			Severity:    severity,
			DisplayText: fmt.Sprintf("%s %s is now %s on %s", kind, vm.Name, vm.Status, node),
			RawPayload:  string(rawPayload),
			Fields:      fmt.Sprintf(`{"source":"proxmox","node":%q,"vmid":%d,"kind":%q}`, node, vm.VMID, kind),
		}
		if err := store.Events.Create(ctx, event); err != nil {
			log.Printf("proxmox poller %s: create state-change event for %s %s: %v",
				p.componentID, kind, vm.Name, err)
		}
	}
}
