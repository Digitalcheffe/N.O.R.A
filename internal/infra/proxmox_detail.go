package infra

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
)

// upidObjectID extracts the object/VM ID from a Proxmox UPID string.
// UPID format: UPID:<node>:<pid>:<pstart>:<starttime>:<type>:<id>:<user>:
func upidObjectID(upid string) string {
	parts := strings.Split(upid, ":")
	if len(parts) >= 7 {
		return parts[6]
	}
	return ""
}

// ── Raw Proxmox API shapes ────────────────────────────────────────────────────

type proxmoxStorageRaw struct {
	Storage string `json:"storage"`
	Type    string `json:"type"`
	Used    int64  `json:"used"`
	Total   int64  `json:"total"`
	Active  int    `json:"active"`
	Enabled int    `json:"enabled"`
}

type proxmoxGuestRaw struct {
	VMID    int    `json:"vmid"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	CPUs    int    `json:"cpus"`
	MaxMem  int64  `json:"maxmem"`
	MaxDisk int64  `json:"maxdisk"`
	Uptime  int64  `json:"uptime"`
	Tags    string `json:"tags"`
}

type proxmoxNodeStatusDetail struct {
	CPU    float64 `json:"cpu"`
	CPUInfo struct {
		CPUs int `json:"cpus"`
	} `json:"cpuinfo"`
	Memory struct {
		Used  int64 `json:"used"`
		Total int64 `json:"total"`
	} `json:"memory"`
	Uptime     int64  `json:"uptime"`
	PVEVersion string `json:"pveversion"`
}

type proxmoxAptPackage struct {
	Package string `json:"Package"`
}

type proxmoxTaskRaw struct {
	UPID       string `json:"upid"`
	ID         string `json:"id"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	ExitStatus string `json:"exitstatus"`
	StartTime  int64  `json:"starttime"`
	EndTime    int64  `json:"endtime"`
	User       string `json:"user"`
	Node       string `json:"node"`
}

type proxmoxStorageContentRaw struct {
	VolID   string `json:"volid"`
	Content string `json:"content"`
	VMID    int    `json:"vmid"`
	CTime   int64  `json:"ctime"`
	Size    int64  `json:"size"`
	Format  string `json:"format"`
	Notes   string `json:"notes"`
}

// ── Result types returned to the API handler ─────────────────────────────────

// ProxmoxStoragePool is one storage pool entry for the detail page.
type ProxmoxStoragePool struct {
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	UsedBytes   int64   `json:"used_bytes"`
	TotalBytes  int64   `json:"total_bytes"`
	UsedPercent float64 `json:"used_percent"`
	Active      bool    `json:"active"`
	Node        string  `json:"node"`
}

// ProxmoxGuestInfo is one VM or LXC entry with extended detail.
type ProxmoxGuestInfo struct {
	VMID           int      `json:"vmid"`
	Name           string   `json:"name"`
	GuestType      string   `json:"guest_type"` // "vm" or "lxc"
	Status         string   `json:"status"`     // "running", "stopped", "paused"
	CPUs           int      `json:"cpus"`
	MaxMemBytes    int64    `json:"max_mem_bytes"`
	MaxDiskBytes   int64    `json:"max_disk_bytes"`
	IP             string   `json:"ip,omitempty"`
	OSType         string   `json:"os_type,omitempty"`
	NetworkBridges []string `json:"network_bridges,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	Onboot         bool     `json:"onboot"`
	Uptime         int64    `json:"uptime"`
	Node           string   `json:"node"`
}

// ProxmoxNodeStatusDetail is extended node status for the detail page header.
type ProxmoxNodeStatusDetail struct {
	Node             string `json:"node"`
	CPUCount         int    `json:"cpu_count"`
	TotalMemBytes    int64  `json:"total_mem_bytes"`
	Uptime           int64  `json:"uptime"`
	PVEVersion       string `json:"pve_version,omitempty"`
	UpdatesAvailable int    `json:"updates_available"`
}

// ProxmoxTaskFailure is one failed Proxmox task entry.
type ProxmoxTaskFailure struct {
	UPID       string `json:"upid"`
	Type       string `json:"type"`
	ObjectID   string `json:"object_id,omitempty"`
	ExitStatus string `json:"exit_status"`
	StartTime  int64  `json:"start_time"`
	EndTime    int64  `json:"end_time,omitempty"`
	User       string `json:"user"`
	Node       string `json:"node"`
}

// ProxmoxBackupJob is one vzdump backup task entry (success or failure).
type ProxmoxBackupJob struct {
	UPID       string `json:"upid"`
	ObjectID   string `json:"object_id,omitempty"`
	ExitStatus string `json:"exit_status"`
	StartTime  int64  `json:"start_time"`
	EndTime    int64  `json:"end_time,omitempty"`
	Node       string `json:"node"`
}

// ProxmoxBackupFile is one backup file entry from storage content.
type ProxmoxBackupFile struct {
	VolID  string `json:"volid"`
	VMID   int    `json:"vmid"`
	CTime  int64  `json:"ctime"`
	Size   int64  `json:"size"`
	Format string `json:"format"`
	Node   string `json:"node"`
	Store  string `json:"store"`
}

// ── Methods on ProxmoxPoller ──────────────────────────────────────────────────

// FetchStoragePools returns all storage pools across all nodes.
func (p *ProxmoxPoller) FetchStoragePools(ctx context.Context) ([]ProxmoxStoragePool, error) {
	var nodes []proxmoxNode
	if err := p.get(ctx, "/api2/json/nodes", &nodes); err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	var pools []ProxmoxStoragePool
	for _, node := range nodes {
		var storages []proxmoxStorageRaw
		if err := p.get(ctx, fmt.Sprintf("/api2/json/nodes/%s/storage", node.Node), &storages); err != nil {
			log.Printf("proxmox detail: storage node %s: %v", node.Node, err)
			continue
		}
		for _, s := range storages {
			pool := ProxmoxStoragePool{
				Name:       s.Storage,
				Type:       s.Type,
				UsedBytes:  s.Used,
				TotalBytes: s.Total,
				Active:     s.Active == 1,
				Node:       node.Node,
			}
			if s.Total > 0 {
				pool.UsedPercent = float64(s.Used) / float64(s.Total) * 100
			}
			pools = append(pools, pool)
		}
	}
	return pools, nil
}

// FetchGuests returns all VMs with extended config details.
// Config is fetched concurrently per guest to get bridges, ostype, and onboot.
// LXC containers are no longer tracked.
func (p *ProxmoxPoller) FetchGuests(ctx context.Context) ([]ProxmoxGuestInfo, error) {
	var nodes []proxmoxNode
	if err := p.get(ctx, "/api2/json/nodes", &nodes); err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	type enrichJob struct {
		raw       proxmoxGuestRaw
		guestType string
		node      string
	}

	var jobs []enrichJob
	for _, node := range nodes {
		var vms []proxmoxGuestRaw
		if err := p.get(ctx, fmt.Sprintf("/api2/json/nodes/%s/qemu", node.Node), &vms); err != nil {
			log.Printf("proxmox detail: qemu node %s: %v", node.Node, err)
		} else {
			for _, vm := range vms {
				jobs = append(jobs, enrichJob{vm, "vm", node.Node})
			}
		}
	}

	results := make([]ProxmoxGuestInfo, len(jobs))
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Add(1)
		go func(idx int, job enrichJob) {
			defer wg.Done()
			results[idx] = p.enrichGuest(ctx, job.raw, job.guestType, job.node)
		}(i, j)
	}
	wg.Wait()

	return results, nil
}

// enrichGuest fetches per-guest config and returns a ProxmoxGuestInfo.
func (p *ProxmoxPoller) enrichGuest(ctx context.Context, g proxmoxGuestRaw, guestType, node string) ProxmoxGuestInfo {
	info := ProxmoxGuestInfo{
		VMID:         g.VMID,
		Name:         g.Name,
		GuestType:    guestType,
		Status:       g.Status,
		CPUs:         g.CPUs,
		MaxMemBytes:  g.MaxMem,
		MaxDiskBytes: g.MaxDisk,
		Uptime:       g.Uptime,
		Node:         node,
	}

	// Parse tags (semicolon or comma separated in Proxmox API).
	if g.Tags != "" {
		for _, t := range strings.FieldsFunc(g.Tags, func(r rune) bool {
			return r == ',' || r == ';'
		}) {
			if t = strings.TrimSpace(t); t != "" {
				info.Tags = append(info.Tags, t)
			}
		}
	}

	// Fetch config for bridges, ostype, and onboot.
	configPath := fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/config", node, g.VMID)

	var config map[string]interface{}
	if err := p.get(ctx, configPath, &config); err != nil {
		log.Printf("proxmox detail: config vm %d: %v", g.VMID, err)
		return info
	}

	if ostype, ok := config["ostype"].(string); ok {
		info.OSType = ostype
	}
	if onboot, ok := config["onboot"].(float64); ok {
		info.Onboot = onboot == 1
	}

	// Extract net0..net9 bridges, deduplicating.
	bridgeSet := make(map[string]struct{})
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("net%d", i)
		netConfig, ok := config[key].(string)
		if !ok || netConfig == "" {
			continue
		}
		if bridge := parseNetBridge(netConfig); bridge != "" {
			bridgeSet[bridge] = struct{}{}
		}
	}
	for bridge := range bridgeSet {
		info.NetworkBridges = append(info.NetworkBridges, bridge)
	}
	sort.Strings(info.NetworkBridges)

	// Query the guest agent for the primary IPv4 (best-effort).
	if g.Status == "running" {
		info.IP = p.fetchVMIP(ctx, node, g.VMID)
	}

	return info
}

// parseNetBridge extracts the bridge name from a Proxmox net config string
// like "virtio=xx:xx:xx:xx:xx:xx,bridge=vmbr0,firewall=1".
func parseNetBridge(netConfig string) string {
	for _, part := range strings.Split(netConfig, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 && strings.TrimSpace(kv[0]) == "bridge" {
			return strings.TrimSpace(kv[1])
		}
	}
	return ""
}

// FetchNodeStatus returns extended status for all nodes including updates available.
func (p *ProxmoxPoller) FetchNodeStatus(ctx context.Context) ([]ProxmoxNodeStatusDetail, error) {
	var nodes []proxmoxNode
	if err := p.get(ctx, "/api2/json/nodes", &nodes); err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	var statuses []ProxmoxNodeStatusDetail
	for _, node := range nodes {
		var st proxmoxNodeStatusDetail
		if err := p.get(ctx, fmt.Sprintf("/api2/json/nodes/%s/status", node.Node), &st); err != nil {
			log.Printf("proxmox detail: status node %s: %v", node.Node, err)
			continue
		}

		ns := ProxmoxNodeStatusDetail{
			Node:          node.Node,
			CPUCount:      st.CPUInfo.CPUs,
			TotalMemBytes: st.Memory.Total,
			Uptime:        st.Uptime,
			PVEVersion:    st.PVEVersion,
		}

		// Fetch available package updates — may fail if token lacks Sys.Modify.
		var pkgs []proxmoxAptPackage
		if err := p.get(ctx, fmt.Sprintf("/api2/json/nodes/%s/apt/update", node.Node), &pkgs); err != nil {
			log.Printf("proxmox detail: apt node %s: %v (token may lack Sys.Modify)", node.Node, err)
		} else {
			ns.UpdatesAvailable = len(pkgs)
		}

		statuses = append(statuses, ns)
	}
	return statuses, nil
}

// FetchBackupJobs returns recent vzdump backup tasks (all statuses) across all nodes.
func (p *ProxmoxPoller) FetchBackupJobs(ctx context.Context) ([]ProxmoxBackupJob, error) {
	var nodes []proxmoxNode
	if err := p.get(ctx, "/api2/json/nodes", &nodes); err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	var jobs []ProxmoxBackupJob
	for _, node := range nodes {
		var tasks []proxmoxTaskRaw
		path := fmt.Sprintf("/api2/json/nodes/%s/tasks?typefilter=vzdump&limit=50", node.Node)
		if err := p.get(ctx, path, &tasks); err != nil {
			log.Printf("proxmox detail: backup tasks node %s: %v", node.Node, err)
			continue
		}
		for _, t := range tasks {
			// Only include completed tasks (EndTime > 0)
			if t.EndTime == 0 {
				continue
			}
			// Proxmox puts the exit status in Status for completed tasks ("OK",
			// "error: ...", etc.); ExitStatus is only populated on some PVE versions.
			exitStatus := t.ExitStatus
			if exitStatus == "" {
				exitStatus = t.Status
			}
			// Object ID: prefer the id field, fall back to parsing the UPID.
			objectID := t.ID
			if objectID == "" {
				objectID = upidObjectID(t.UPID)
			}
			jobs = append(jobs, ProxmoxBackupJob{
				UPID:       t.UPID,
				ObjectID:   objectID,
				ExitStatus: exitStatus,
				StartTime:  t.StartTime,
				EndTime:    t.EndTime,
				Node:       t.Node,
			})
		}
	}
	return jobs, nil
}

// FetchTaskFailures returns failed tasks from the last 7 days across all nodes.
func (p *ProxmoxPoller) FetchTaskFailures(ctx context.Context) ([]ProxmoxTaskFailure, error) {
	var nodes []proxmoxNode
	if err := p.get(ctx, "/api2/json/nodes", &nodes); err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	var failures []ProxmoxTaskFailure
	for _, node := range nodes {
		// errors=1 asks Proxmox to filter to failed tasks only.
		var tasks []proxmoxTaskRaw
		path := fmt.Sprintf("/api2/json/nodes/%s/tasks?errors=1&limit=50", node.Node)
		if err := p.get(ctx, path, &tasks); err != nil {
			log.Printf("proxmox detail: tasks node %s: %v", node.Node, err)
			continue
		}

		for _, t := range tasks {
			// Skip tasks that ended with OK despite the errors=1 filter being unreliable.
			if t.ExitStatus == "" || t.ExitStatus == "OK" {
				continue
			}
			failures = append(failures, ProxmoxTaskFailure{
				UPID:       t.UPID,
				Type:       t.Type,
				ObjectID:   t.ID,
				ExitStatus: t.ExitStatus,
				StartTime:  t.StartTime,
				EndTime:    t.EndTime,
				User:       t.User,
				Node:       t.Node,
			})
		}
	}
	return failures, nil
}

// FetchBackupFiles returns backup files found in storage across all nodes.
// It queries each backup-capable storage's content endpoint to get per-VM file entries.
func (p *ProxmoxPoller) FetchBackupFiles(ctx context.Context) ([]ProxmoxBackupFile, error) {
	var nodes []proxmoxNode
	if err := p.get(ctx, "/api2/json/nodes", &nodes); err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	var files []ProxmoxBackupFile
	for _, node := range nodes {
		var storages []proxmoxStorageRaw
		if err := p.get(ctx, fmt.Sprintf("/api2/json/nodes/%s/storage", node.Node), &storages); err != nil {
			log.Printf("proxmox backup files: list storage node %s: %v", node.Node, err)
			continue
		}
		for _, s := range storages {
			if s.Active != 1 {
				continue
			}
			var content []proxmoxStorageContentRaw
			path := fmt.Sprintf("/api2/json/nodes/%s/storage/%s/content?content=backup", node.Node, s.Storage)
			if err := p.get(ctx, path, &content); err != nil {
				log.Printf("proxmox backup files: content %s/%s: %v", node.Node, s.Storage, err)
				continue
			}
			for _, c := range content {
				if c.VMID == 0 {
					continue
				}
				files = append(files, ProxmoxBackupFile{
					VolID:  c.VolID,
					VMID:   c.VMID,
					CTime:  c.CTime,
					Size:   c.Size,
					Format: c.Format,
					Node:   node.Node,
					Store:  s.Storage,
				})
			}
		}
	}
	return files, nil
}
