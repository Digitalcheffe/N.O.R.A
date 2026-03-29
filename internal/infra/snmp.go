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

// SNMPPoller polls a single host via SNMP (HOST-RESOURCES-MIB), writing
// cpu_percent, mem_percent, and per-disk disk_percent_{label} into resource_readings.
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

// ── Poll ──────────────────────────────────────────────────────────────────────

// Poll opens an SNMP connection, collects CPU / memory / disk metrics, writes
// resource_readings, and updates last_status. A Connect() error is returned
// directly so the caller can mark the component offline. Metric-level errors
// are logged but do not abort the poll; partial success yields status="degraded".
func (p *SNMPPoller) Poll(ctx context.Context, store *repo.Store) error {
	client := p.newClient()

	if err := client.Connect(); err != nil {
		return fmt.Errorf("snmp connect %s: %w", p.ip, err)
	}
	defer client.Close() //nolint:errcheck

	now := time.Now().UTC()
	degraded := false

	// ── CPU ──────────────────────────────────────────────────────────────────
	cpuPct, err := p.pollCPU(client)
	if err != nil {
		log.Printf("snmp poller %s: cpu: %v", p.componentID, err)
		degraded = true
	} else {
		p.writeReading(ctx, store, now, "cpu_percent", cpuPct)
	}

	// ── Storage (memory + disks) ──────────────────────────────────────────────
	storageEntries, err := p.walkStorageTable(client)
	if err != nil {
		log.Printf("snmp poller %s: storage table: %v", p.componentID, err)
		degraded = true
	} else {
		if memPct, ok := computeMemPercent(storageEntries); ok {
			p.writeReading(ctx, store, now, "mem_percent", memPct)
		} else {
			log.Printf("snmp poller %s: no RAM storage entry found", p.componentID)
			degraded = true
		}
		for label, pct := range computeDiskPercents(storageEntries) {
			p.writeReading(ctx, store, now, "disk_percent_"+label, pct)
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

// computeMemPercent finds the hrStorageRam entry and returns used/size as a percent.
func computeMemPercent(rows []storageRow) (float64, bool) {
	for _, r := range rows {
		if !oidMatch(r.storageType, oidHrStorageRam) {
			continue
		}
		if r.size == 0 {
			return 0, false
		}
		return float64(r.used) / float64(r.size) * 100, true
	}
	return 0, false
}

// computeDiskPercents returns a map of sanitized label → used% for all fixed-disk entries.
func computeDiskPercents(rows []storageRow) map[string]float64 {
	result := make(map[string]float64)
	for _, r := range rows {
		if !oidMatch(r.storageType, oidHrStorageFixDisk) {
			continue
		}
		if r.size == 0 {
			continue
		}
		pct := float64(r.used) / float64(r.size) * 100
		label := sanitizeDiskLabel(r.descr)
		result[label] = pct
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
