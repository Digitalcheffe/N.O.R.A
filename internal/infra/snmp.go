// Package infra — SNMP poller for generic Linux and Windows hosts.
//
// Host-side setup (Linux, net-snmp):
//   apt install snmpd                              (Debian/Ubuntu)
//   yum install net-snmp                           (RHEL/Rocky)
//   Edit /etc/snmp/snmpd.conf — set community string and allowed source IP
//   systemctl enable --now snmpd
//
// Host-side setup (Windows):
//   Enable SNMP Service via Windows Features or Server Manager
//   In SNMP Service properties: set community string, add NORA host IP as permitted manager
package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/gosnmp/gosnmp"
	"github.com/google/uuid"
)

// SNMPConfig is the JSON shape stored in infrastructure_components.snmp_config.
// version is "2c" (default) or "3". v3 fields are only required when version is "3".
type SNMPConfig struct {
	Version        string `json:"version"`          // "2c" | "3"
	Community      string `json:"community"`        // v2c community / v3 security name
	Port           uint16 `json:"port"`             // default 161
	AuthProtocol   string `json:"auth_protocol"`    // v3: "MD5" | "SHA"
	AuthPassphrase string `json:"auth_passphrase"`  // v3
	PrivProtocol   string `json:"priv_protocol"`    // v3: "DES" | "AES"
	PrivPassphrase string `json:"priv_passphrase"`  // v3
	ContextName    string `json:"context_name"`     // v3
}

// SNMPMeta is the JSON shape stored in infrastructure_components.snmp_meta.
// It holds the latest system identity + resource snapshot written each poll cycle.
type SNMPMeta struct {
	OSDescription string     `json:"os_description"` // sysDescr OID
	Uptime        string     `json:"uptime"`         // sysUpTime converted from timeticks
	Hostname      string     `json:"hostname"`       // sysName OID
	CPUPercent    float64    `json:"cpu_percent"`
	Memory        SNMPMemory `json:"memory"`
	Disks         []SNMPDisk `json:"disks"`
}

// SNMPMemory holds the latest RAM reading from hrStorageRam.
type SNMPMemory struct {
	UsedBytes  int64   `json:"used_bytes"`
	TotalBytes int64   `json:"total_bytes"`
	Percent    float64 `json:"percent"`
}

// SNMPDisk holds one fixed-disk entry from hrStorageFixedDisk.
type SNMPDisk struct {
	Label      string  `json:"label"`       // original hrStorageDescr (e.g. "/", "C:")
	UsedBytes  int64   `json:"used_bytes"`
	TotalBytes int64   `json:"total_bytes"`
	Percent    float64 `json:"percent"`
}

// SNMPv2-MIB — system identity OIDs (RFC 3418).
const (
	oidSysDescr  = "1.3.6.1.2.1.1.1.0" // full OS description string
	oidSysUpTime = "1.3.6.1.2.1.1.3.0" // timeticks since last reboot (1/100 second)
	oidSysName   = "1.3.6.1.2.1.1.5.0" // hostname as reported by the device
)

// HOST-RESOURCES-MIB OIDs (RFC 2790) — work on Linux (net-snmp) and Windows.
const (
	oidHrProcessorLoad  = "1.3.6.1.2.1.25.3.3.1.2"  // hrProcessorLoad — one row per CPU core
	oidHrStorageEntry   = "1.3.6.1.2.1.25.2.3.1"     // hrStorageEntry table (all columns)
	oidHrStorageRam     = "1.3.6.1.2.1.25.2.1.2"     // hrStorageType value for RAM
	oidHrStorageFixDisk = "1.3.6.1.2.1.25.2.1.4"     // hrStorageType value for fixed disk
)

// hrStorageEntry column numbers within the hrStorageEntry table.
const (
	colHrStorageType            = "2"
	colHrStorageDescr           = "3"
	colHrStorageAllocationUnits = "4"
	colHrStorageSize            = "5"
	colHrStorageUsed            = "6"
)

// snmpClient is the interface used for SNMP operations, allowing test mocking.
type snmpClient interface {
	Connect() error
	Close() error
	BulkWalkAll(rootOid string) ([]gosnmp.SnmpPDU, error)
	Get(oids []string) (*gosnmp.SnmpPacket, error)
}

// SNMPPoller polls a single host via SNMP (SNMPv2-MIB + HOST-RESOURCES-MIB),
// writing cpu_percent, mem_percent, and per-disk disk_percent_{label} into
// resource_readings, and storing the full snapshot in snmp_meta.
type SNMPPoller struct {
	componentID string
	ip          string
	cfg         SNMPConfig
	newClient   func() snmpClient // injectable for tests
}

// NewSNMPPoller creates an SNMPPoller from a component ID, IP, and snmp_config JSON.
func NewSNMPPoller(componentID, ip, cfgJSON string) (*SNMPPoller, error) {
	var cfg SNMPConfig
	if err := json.Unmarshal([]byte(cfgJSON), &cfg); err != nil {
		return nil, fmt.Errorf("parse snmp config: %w", err)
	}
	if cfg.Port == 0 {
		cfg.Port = 161
	}
	if cfg.Version == "" {
		cfg.Version = "2c"
	}
	p := &SNMPPoller{
		componentID: componentID,
		ip:          ip,
		cfg:         cfg,
	}
	p.newClient = p.buildGoSNMPClient
	return p, nil
}

// buildGoSNMPClient constructs a production gosnmp.GoSNMP from the poller config.
func (p *SNMPPoller) buildGoSNMPClient() snmpClient {
	g := &gosnmp.GoSNMP{
		Target:  p.ip,
		Port:    p.cfg.Port,
		Timeout: 10 * time.Second,
		Retries: 1,
	}

	switch p.cfg.Version {
	case "3":
		g.Version = gosnmp.Version3
		g.ContextName = p.cfg.ContextName
		g.MsgFlags = snmpv3MsgFlags(p.cfg)
		g.SecurityModel = gosnmp.UserSecurityModel
		g.SecurityParameters = &gosnmp.UsmSecurityParameters{
			UserName:                 p.cfg.Community,
			AuthenticationProtocol:   snmpAuthProtocol(p.cfg.AuthProtocol),
			AuthenticationPassphrase: p.cfg.AuthPassphrase,
			PrivacyProtocol:          snmpPrivProtocol(p.cfg.PrivProtocol),
			PrivacyPassphrase:        p.cfg.PrivPassphrase,
		}
	default: // "2c"
		g.Version = gosnmp.Version2c
		g.Community = p.cfg.Community
	}

	return g
}

func snmpv3MsgFlags(cfg SNMPConfig) gosnmp.SnmpV3MsgFlags {
	hasAuth := cfg.AuthPassphrase != ""
	hasPriv := cfg.PrivPassphrase != ""
	switch {
	case hasAuth && hasPriv:
		return gosnmp.AuthPriv
	case hasAuth:
		return gosnmp.AuthNoPriv
	default:
		return gosnmp.NoAuthNoPriv
	}
}

func snmpAuthProtocol(s string) gosnmp.SnmpV3AuthProtocol {
	switch strings.ToUpper(s) {
	case "SHA":
		return gosnmp.SHA
	default:
		return gosnmp.MD5
	}
}

func snmpPrivProtocol(s string) gosnmp.SnmpV3PrivProtocol {
	switch strings.ToUpper(s) {
	case "AES":
		return gosnmp.AES
	default:
		return gosnmp.DES
	}
}

// ── Metrics-only collection ───────────────────────────────────────────────────

// SNMPDiskReading holds the utilisation for a single disk from HOST-RESOURCES-MIB.
type SNMPDiskReading struct {
	Label   string  // original hrStorageDescr (e.g. "/", "C:")
	Percent float64 // disk_percent value
}

// SNMPMetricsSnapshot holds the raw metric values collected in one SNMP pass.
// The MetricsScanner uses this to write resource_readings and apply thresholds.
type SNMPMetricsSnapshot struct {
	CPUPercent float64
	MemPercent float64
	MemUsedGB  float64
	MemTotalGB float64
	Disks      []SNMPDiskReading
	Uptime     string
}

// CollectMetrics opens an SNMP connection, reads CPU, memory, disk, and uptime,
// and returns raw values without any database writes or status updates.
func (p *SNMPPoller) CollectMetrics(ctx context.Context) (*SNMPMetricsSnapshot, error) {
	client := p.newClient()
	if err := client.Connect(); err != nil {
		return nil, fmt.Errorf("snmp connect %s: %w", p.ip, err)
	}
	defer client.Close() //nolint:errcheck

	snap := &SNMPMetricsSnapshot{}

	// System info — uptime
	if sysInfo, err := p.pollSystemInfo(client); err == nil {
		snap.Uptime = sysInfo.Uptime
	}

	// CPU
	if cpuPct, err := p.pollCPU(client); err != nil {
		log.Printf("snmp metrics %s: cpu: %v", p.componentID, err)
	} else {
		snap.CPUPercent = cpuPct
	}

	// Storage (memory + disks)
	if rows, err := p.walkStorageTable(client); err != nil {
		log.Printf("snmp metrics %s: storage: %v", p.componentID, err)
	} else {
		if mem, ok := computeMemResult(rows); ok {
			snap.MemPercent = mem.percent
			snap.MemUsedGB = float64(mem.usedBytes) / (1024 * 1024 * 1024)
			snap.MemTotalGB = float64(mem.totalBytes) / (1024 * 1024 * 1024)
		}
		for _, d := range computeDiskResults(rows) {
			snap.Disks = append(snap.Disks, SNMPDiskReading{
				Label:   d.label,
				Percent: d.percent,
			})
		}
	}

	return snap, nil
}

// ── Poll ──────────────────────────────────────────────────────────────────────

// Poll opens an SNMP connection, collects system identity + CPU / memory / disk
// metrics, writes resource_readings, stores snmp_meta, and updates last_status.
// A Connect() error is returned directly so the caller can mark the component
// offline. Metric-level errors are logged but do not abort the poll; partial
// success yields status="degraded".
func (p *SNMPPoller) Poll(ctx context.Context, store *repo.Store) error {
	client := p.newClient()

	if err := client.Connect(); err != nil {
		return fmt.Errorf("snmp connect %s: %w", p.ip, err)
	}
	defer client.Close() //nolint:errcheck

	now := time.Now().UTC()
	degraded := false
	meta := SNMPMeta{}

	// ── System identity (sysDescr, sysUpTime, sysName) ───────────────────────
	sysInfo, err := p.pollSystemInfo(client)
	if err != nil {
		log.Printf("snmp poller %s: system info: %v", p.componentID, err)
		// Non-fatal — system info is best-effort.
	} else {
		meta.OSDescription = sysInfo.OSDescription
		meta.Uptime = sysInfo.Uptime
		meta.Hostname = sysInfo.Hostname
	}

	// ── CPU ──────────────────────────────────────────────────────────────────
	cpuPct, err := p.pollCPU(client)
	if err != nil {
		log.Printf("snmp poller %s: cpu: %v", p.componentID, err)
		degraded = true
	} else {
		meta.CPUPercent = cpuPct
		p.writeReading(ctx, store, now, "cpu_percent", cpuPct)
	}

	// ── Storage (memory + disks) ──────────────────────────────────────────────
	storageEntries, err := p.walkStorageTable(client)
	if err != nil {
		log.Printf("snmp poller %s: storage table: %v", p.componentID, err)
		degraded = true
	} else {
		if mem, ok := computeMemResult(storageEntries); ok {
			meta.Memory = SNMPMemory{
				UsedBytes:  mem.usedBytes,
				TotalBytes: mem.totalBytes,
				Percent:    mem.percent,
			}
			p.writeReading(ctx, store, now, "mem_percent", mem.percent)
		} else {
			log.Printf("snmp poller %s: no RAM storage entry found", p.componentID)
			degraded = true
		}

		diskResults := computeDiskResults(storageEntries)
		meta.Disks = make([]SNMPDisk, len(diskResults))
		for i, d := range diskResults {
			meta.Disks[i] = SNMPDisk{
				Label:      d.label,
				UsedBytes:  d.usedBytes,
				TotalBytes: d.totalBytes,
				Percent:    d.percent,
			}
			p.writeReading(ctx, store, now, "disk_percent_"+sanitizeDiskLabel(d.label), d.percent)
		}
	}

	// ── Persist snmp_meta snapshot ────────────────────────────────────────────
	if metaJSON, jsonErr := json.Marshal(meta); jsonErr == nil {
		if updateErr := store.InfraComponents.UpdateSNMPMeta(ctx, p.componentID, string(metaJSON)); updateErr != nil {
			log.Printf("snmp poller %s: write snmp_meta: %v", p.componentID, updateErr)
		}
	}

	status := "online"
	if degraded {
		status = "degraded"
	}
	polledAt := now.Format(time.RFC3339Nano)
	if err := store.InfraComponents.UpdateStatus(ctx, p.componentID, status, polledAt); err != nil {
		log.Printf("snmp poller %s: update status: %v", p.componentID, err)
	}
	return nil
}

func (p *SNMPPoller) writeReading(ctx context.Context, store *repo.Store, now time.Time, metric string, value float64) {
	r := &models.ResourceReading{
		ID:         uuid.New().String(),
		SourceID:   p.componentID,
		SourceType: "snmp_host",
		Metric:     metric,
		Value:      value,
		RecordedAt: now,
	}
	if err := store.Resources.Create(ctx, r); err != nil {
		log.Printf("snmp poller %s: write %s: %v", p.componentID, metric, err)
	}
}

// ── System info collector ─────────────────────────────────────────────────────

// systemInfoResult holds the parsed SNMPv2-MIB system scalars.
type systemInfoResult struct {
	OSDescription string
	Uptime        string
	Hostname      string
}

// pollSystemInfo performs scalar GETs for sysDescr, sysUpTime, and sysName.
func (p *SNMPPoller) pollSystemInfo(client snmpClient) (systemInfoResult, error) {
	pkt, err := client.Get([]string{oidSysDescr, oidSysUpTime, oidSysName})
	if err != nil {
		return systemInfoResult{}, fmt.Errorf("get system OIDs: %w", err)
	}

	var result systemInfoResult
	for _, v := range pkt.Variables {
		clean := strings.TrimPrefix(v.Name, ".")
		switch clean {
		case oidSysDescr:
			result.OSDescription = strings.TrimSpace(snmpToString(v.Value))
		case oidSysUpTime:
			ticks := snmpToUint32(v.Value)
			result.Uptime = ticksToUptime(ticks)
		case oidSysName:
			result.Hostname = strings.TrimSpace(snmpToString(v.Value))
		}
	}
	return result, nil
}

// ticksToUptime converts SNMP timeticks (1/100 second units) to a human-readable
// uptime string such as "14d 3h 22m" or "2h 5m".
func ticksToUptime(ticks uint32) string {
	total := time.Duration(ticks) * 10 * time.Millisecond
	d := int(total.Hours() / 24)
	h := int(total.Hours()) % 24
	m := int(total.Minutes()) % 60
	switch {
	case d > 0:
		return fmt.Sprintf("%dd %dh %dm", d, h, m)
	case h > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	default:
		return fmt.Sprintf("%dm", m)
	}
}

// ── Metric collectors ─────────────────────────────────────────────────────────

// pollCPU walks hrProcessorLoad and returns the average load across all cores.
func (p *SNMPPoller) pollCPU(client snmpClient) (float64, error) {
	pdus, err := client.BulkWalkAll(oidHrProcessorLoad)
	if err != nil {
		return 0, fmt.Errorf("walk hrProcessorLoad: %w", err)
	}
	if len(pdus) == 0 {
		return 0, fmt.Errorf("no hrProcessorLoad entries (host may not expose CPU via SNMP)")
	}
	var sum float64
	for _, pdu := range pdus {
		sum += snmpToFloat64(pdu.Value)
	}
	return sum / float64(len(pdus)), nil
}

// storageRow holds one assembled row from the hrStorageEntry table.
type storageRow struct {
	storageType string
	descr       string
	allocUnits  int64
	size        int64
	used        int64
}

// walkStorageTable does a single BulkWalk of the hrStorageEntry subtree and
// assembles per-row structs keyed by table index.
func (p *SNMPPoller) walkStorageTable(client snmpClient) ([]storageRow, error) {
	pdus, err := client.BulkWalkAll(oidHrStorageEntry)
	if err != nil {
		return nil, fmt.Errorf("walk hrStorageEntry: %w", err)
	}

	rowMap := make(map[string]*storageRow)
	ensure := func(idx string) *storageRow {
		if r, ok := rowMap[idx]; ok {
			return r
		}
		r := &storageRow{}
		rowMap[idx] = r
		return r
	}

	for _, pdu := range pdus {
		// PDU name: .1.3.6.1.2.1.25.2.3.1.{col}.{idx}
		clean := strings.TrimPrefix(pdu.Name, ".")
		parts := strings.Split(clean, ".")
		if len(parts) < 2 {
			continue
		}
		col := parts[len(parts)-2]
		idx := parts[len(parts)-1]
		row := ensure(idx)

		switch col {
		case colHrStorageType:
			row.storageType = snmpOIDString(pdu.Value)
		case colHrStorageDescr:
			row.descr = snmpToString(pdu.Value)
		case colHrStorageAllocationUnits:
			row.allocUnits = snmpToInt64(pdu.Value)
		case colHrStorageSize:
			row.size = snmpToInt64(pdu.Value)
		case colHrStorageUsed:
			row.used = snmpToInt64(pdu.Value)
		}
	}

	rows := make([]storageRow, 0, len(rowMap))
	for _, r := range rowMap {
		rows = append(rows, *r)
	}
	return rows, nil
}

// memResult holds the computed memory metrics from the hrStorageRam entry.
type memResult struct {
	percent    float64
	usedBytes  int64
	totalBytes int64
}

// computeMemResult finds the hrStorageRam entry and returns the full result.
func computeMemResult(rows []storageRow) (memResult, bool) {
	for _, r := range rows {
		if !oidMatch(r.storageType, oidHrStorageRam) {
			continue
		}
		if r.size == 0 {
			return memResult{}, false
		}
		total := r.size * r.allocUnits
		used := r.used * r.allocUnits
		pct := float64(r.used) / float64(r.size) * 100
		return memResult{percent: pct, usedBytes: used, totalBytes: total}, true
	}
	return memResult{}, false
}

// computeMemPercent is kept for backward compatibility with existing tests.
func computeMemPercent(rows []storageRow) (float64, bool) {
	m, ok := computeMemResult(rows)
	return m.percent, ok
}

// diskResult holds the computed disk metrics for one fixed-disk entry.
type diskResult struct {
	label      string  // original hrStorageDescr
	percent    float64
	usedBytes  int64
	totalBytes int64
}

// computeDiskResults returns a slice of diskResult for all fixed-disk entries.
func computeDiskResults(rows []storageRow) []diskResult {
	var results []diskResult
	for _, r := range rows {
		if !oidMatch(r.storageType, oidHrStorageFixDisk) {
			continue
		}
		if r.size == 0 {
			continue
		}
		pct := float64(r.used) / float64(r.size) * 100
		total := r.size * r.allocUnits
		used := r.used * r.allocUnits
		results = append(results, diskResult{
			label:      r.descr,
			percent:    pct,
			usedBytes:  used,
			totalBytes: total,
		})
	}
	return results
}

// computeDiskPercents is kept for backward compatibility with existing tests.
func computeDiskPercents(rows []storageRow) map[string]float64 {
	result := make(map[string]float64)
	for _, d := range computeDiskResults(rows) {
		result[sanitizeDiskLabel(d.label)] = d.percent
	}
	return result
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// oidMatch compares two OID strings, tolerating a leading dot on either side.
func oidMatch(a, b string) bool {
	return strings.TrimPrefix(a, ".") == strings.TrimPrefix(b, ".")
}

// snmpToFloat64 converts common gosnmp integer value types to float64.
func snmpToFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case int:
		return float64(val)
	case int32:
		return float64(val)
	case int64:
		return float64(val)
	case uint:
		return float64(val)
	case uint32:
		return float64(val)
	case uint64:
		return float64(val)
	case float32:
		return float64(val)
	case float64:
		return val
	}
	return 0
}

func snmpToInt64(v interface{}) int64 { return int64(snmpToFloat64(v)) }

// snmpToUint32 extracts a uint32 timeticks value from a gosnmp PDU.
func snmpToUint32(v interface{}) uint32 {
	switch val := v.(type) {
	case uint32:
		return val
	case uint:
		return uint32(val)
	case int:
		if val >= 0 {
			return uint32(val)
		}
	}
	return uint32(snmpToFloat64(v))
}

// snmpOIDString converts a gosnmp ObjectIdentifier value to a dotted-decimal string.
// gosnmp returns OID values as dotted strings, potentially with a leading dot.
func snmpOIDString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	}
	return fmt.Sprintf("%v", v)
}

// snmpToString converts a gosnmp OctetString value to a Go string.
func snmpToString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	}
	return fmt.Sprintf("%v", v)
}

// sanitizeDiskLabel converts a storage description to a safe metric-name suffix.
// Examples: "/" → "root", "C:\\" → "c", "Label /dev/sda1" → "label__dev_sda1"
// SanitizeDiskLabel is the exported form of sanitizeDiskLabel, used by the
// metrics scanner package which lives outside the infra package.
func SanitizeDiskLabel(s string) string {
	return sanitizeDiskLabel(s)
}

func sanitizeDiskLabel(s string) string {
	s = strings.TrimSpace(s)
	if s == "/" {
		return "root"
	}
	// Drop colons and backslashes, replace slashes/spaces with underscores.
	s = strings.ReplaceAll(s, ":", "")
	s = strings.ReplaceAll(s, "\\", "_")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ToLower(strings.Trim(s, "_"))
	if s == "" {
		return "disk"
	}
	return s
}
