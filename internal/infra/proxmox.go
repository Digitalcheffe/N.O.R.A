package infra

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
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

// ProxmoxChildID is the exported form of proxmoxChildID, for use by the
// discovery scanner package which lives outside the infra package.
func ProxmoxChildID(parentID string, vmid int) string {
	return proxmoxChildID(parentID, vmid)
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

// ── IP helpers ────────────────────────────────────────────────────────────────

// isUsableIPv4 returns true for IPv4 addresses that are not loopback or link-local.
func isUsableIPv4(addr string) bool {
	return !strings.HasPrefix(addr, "127.") && !strings.HasPrefix(addr, "169.254.")
}

// parseLXCNetIP extracts the primary IPv4 address from a Proxmox LXC net config
// string like "name=eth0,bridge=vmbr0,ip=192.168.1.55/24,gw=192.168.1.1".
// Returns "" if no usable address is found.
func parseLXCNetIP(netConfig string) string {
	for _, part := range strings.Split(netConfig, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 || strings.TrimSpace(kv[0]) != "ip" {
			continue
		}
		// Strip CIDR prefix; skip DHCP placeholder.
		ipPart := strings.SplitN(strings.TrimSpace(kv[1]), "/", 2)[0]
		if ipPart != "" && ipPart != "dhcp" && isUsableIPv4(ipPart) {
			return ipPart
		}
	}
	return ""
}

// proxmoxNetIPAddr is one IP address entry from the QEMU guest agent interface list.
type proxmoxNetIPAddr struct {
	IPAddress     string `json:"ip-address"`
	IPAddressType string `json:"ip-address-type"`
}

// proxmoxNetInterface is one network interface from the QEMU guest agent.
type proxmoxNetInterface struct {
	Name        string             `json:"name"`
	IPAddresses []proxmoxNetIPAddr `json:"ip-addresses"`
}

// fetchVMIP queries the QEMU guest agent for the VM's primary IPv4 address.
// Returns "" if the agent is not running or the request fails — never errors.
func (p *ProxmoxPoller) fetchVMIP(ctx context.Context, node string, vmid int) string {
	path := fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/agent/network-get-interfaces", node, vmid)
	var ifaces []proxmoxNetInterface
	if err := p.get(ctx, path, &ifaces); err != nil {
		return ""
	}
	for _, iface := range ifaces {
		for _, addr := range iface.IPAddresses {
			if addr.IPAddressType == "ipv4" && isUsableIPv4(addr.IPAddress) {
				return addr.IPAddress
			}
		}
	}
	return ""
}

// fetchLXCIP queries the LXC config for the container's primary IPv4 address.
// Returns "" if no usable address is found — never errors.
func (p *ProxmoxPoller) fetchLXCIP(ctx context.Context, node string, vmid int) string {
	path := fmt.Sprintf("/api2/json/nodes/%s/lxc/%d/config", node, vmid)
	var config map[string]interface{}
	if err := p.get(ctx, path, &config); err != nil {
		return ""
	}
	for i := 0; i < 10; i++ {
		if val, ok := config[fmt.Sprintf("net%d", i)].(string); ok && val != "" {
			if ip := parseLXCNetIP(val); ip != "" {
				return ip
			}
		}
	}
	return ""
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
	Uptime int64 `json:"uptime"` // seconds since last boot
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

// ── Metrics-only collection ───────────────────────────────────────────────────

// ProxmoxNodeMetrics holds the raw metric values collected for one Proxmox node.
// The caller (MetricsScanner) is responsible for writing these to resource_readings
// and applying threshold rules.
type ProxmoxNodeMetrics struct {
	Node        string
	CPUPercent  float64
	MemPercent  float64
	MemUsedGB   float64
	MemTotalGB  float64
	DiskPercent float64
	UptimeSecs  int64
}

// CollectNodeMetrics fetches CPU%, memory, and disk metrics for each node in the
// cluster and returns them as raw values without writing to the database.
// It returns the list of nodes found and any partial errors encountered.
func (p *ProxmoxPoller) CollectNodeMetrics(ctx context.Context) ([]ProxmoxNodeMetrics, error) {
	var nodes []proxmoxNode
	if err := p.get(ctx, "/api2/json/nodes", &nodes); err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	var results []ProxmoxNodeMetrics
	for _, node := range nodes {
		m, err := p.fetchNodeMetrics(ctx, node.Node)
		if err != nil {
			log.Printf("proxmox metrics %s: node %s: %v", p.componentID, node.Node, err)
			continue
		}
		results = append(results, m)
	}
	return results, nil
}

func (p *ProxmoxPoller) fetchNodeMetrics(ctx context.Context, node string) (ProxmoxNodeMetrics, error) {
	var status proxmoxNodeStatus
	if err := p.get(ctx, fmt.Sprintf("/api2/json/nodes/%s/status", node), &status); err != nil {
		return ProxmoxNodeMetrics{}, fmt.Errorf("node status: %w", err)
	}

	cpuPercent := status.CPU * 100
	var memPercent, memUsedGB, memTotalGB float64
	if status.Memory.Total > 0 {
		memPercent = (status.Memory.Used / status.Memory.Total) * 100
		memUsedGB = status.Memory.Used / (1024 * 1024 * 1024)
		memTotalGB = status.Memory.Total / (1024 * 1024 * 1024)
	}

	var storages []proxmoxStorage
	var diskPercent float64
	if err := p.get(ctx, fmt.Sprintf("/api2/json/nodes/%s/storage", node), &storages); err == nil {
		var usedTotal, sizeTotal float64
		for _, s := range storages {
			if s.Active == 0 {
				continue
			}
			usedTotal += s.Used
			sizeTotal += s.Total
		}
		if sizeTotal > 0 {
			diskPercent = (usedTotal / sizeTotal) * 100
		}
	}

	return ProxmoxNodeMetrics{
		Node:        node,
		CPUPercent:  cpuPercent,
		MemPercent:  memPercent,
		MemUsedGB:   memUsedGB,
		MemTotalGB:  memTotalGB,
		DiskPercent: diskPercent,
		UptimeSecs:  status.Uptime,
	}, nil
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

	// Auto-populate the parent node's IP from the base_url credential so it
	// appears on the detail page even if the user left the IP field blank.
	if u, err := url.Parse(p.creds.BaseURL); err == nil {
		if host := u.Hostname(); host != "" {
			if ipErr := store.InfraComponents.UpdateIP(ctx, p.componentID, host); ipErr != nil {
				log.Printf("proxmox poller %s: update parent ip: %v", p.componentID, ipErr)
			}
		}
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
	// LXC containers are no longer tracked as infrastructure components.
	var vms []proxmoxVM
	if err := p.get(ctx, fmt.Sprintf("/api2/json/nodes/%s/qemu", node), &vms); err != nil {
		log.Printf("proxmox poller %s: list qemu on %s: %v", p.componentID, node, err)
	} else {
		p.checkStateChanges(ctx, store, node, "VM", vms)
		p.upsertChildren(ctx, store, vms, node, now)
	}

	return nil
}

// upsertChildren creates or updates an InfrastructureComponent child record for
// each VM in the list.  Child IDs are derived deterministically from
// (componentID, vmid) so re-runs are fully idempotent.  New VMs default to
// type "vm_other"; the discovery scanner sets the precise type from ostype.
func (p *ProxmoxPoller) upsertChildren(ctx context.Context, store *repo.Store, vms []proxmoxVM, node string, now time.Time) {
	polledAt := now.Format(time.RFC3339Nano)

	for _, vm := range vms {
		id := proxmoxChildID(p.componentID, vm.VMID)

		status := "offline"
		if vm.Status == "running" {
			status = "online"
		}

		// Best-effort IP resolution via guest agent.
		var ip string
		if vm.Status == "running" {
			ip = p.fetchVMIP(ctx, node, vm.VMID)
		}

		// Try an update-in-place first (fast path for subsequent polls).
		err := store.InfraComponents.UpdateStatus(ctx, id, status, polledAt)
		if err == nil {
			if ip != "" {
				if ipErr := store.InfraComponents.UpdateIP(ctx, id, ip); ipErr != nil {
					log.Printf("proxmox poller %s: update ip for vm %q: %v", p.componentID, vm.Name, ipErr)
				}
			}
			continue
		}
		if !errors.Is(err, repo.ErrNotFound) {
			log.Printf("proxmox poller %s: update child vm %q: %v", p.componentID, vm.Name, err)
			continue
		}

		// First time seeing this VM — create a child component.
		// Type defaults to vm_other; the discovery scanner upgrades it to
		// vm_linux or vm_windows once it fetches the Proxmox config ostype.
		c := &models.InfrastructureComponent{
			ID:               id,
			Name:             vm.Name,
			IP:               ip,
			Type:             "vm_other",
			CollectionMethod: "none",
			Enabled:          true,
			LastStatus:       status,
			CreatedAt:        polledAt,
		}
		if err := store.InfraComponents.Create(ctx, c); err != nil {
			log.Printf("proxmox poller %s: create child vm %q: %v", p.componentID, vm.Name, err)
			continue
		}
		if err := store.ComponentLinks.SetParent(ctx, "proxmox_node", p.componentID, "vm_other", id); err != nil {
			log.Printf("proxmox poller %s: set parent link for vm %q: %v", p.componentID, vm.Name, err)
		}
		log.Printf("proxmox poller %s: discovered vm %q (vmid=%d, status=%s, ip=%s)",
			p.componentID, vm.Name, vm.VMID, vm.Status, ip)
	}
}

// checkStateChanges compares current VM statuses against the last known
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
			ID:         uuid.New().String(),
			Level:      severity,
			SourceName: vm.Name,
			SourceType: "vm_other",
			SourceID:   p.componentID,
			Title:      fmt.Sprintf("%s %s is now %s on %s", kind, vm.Name, vm.Status, node),
			Payload:    string(rawPayload),
			CreatedAt:  time.Now().UTC(),
		}
		if err := store.Events.Create(ctx, event); err != nil {
			log.Printf("proxmox poller %s: create state-change event for %s %s: %v",
				p.componentID, kind, vm.Name, err)
		}
	}
}
