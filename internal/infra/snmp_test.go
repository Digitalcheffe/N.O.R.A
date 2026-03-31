package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/digitalcheffe/nora/internal/config"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/migrations"
	"github.com/gosnmp/gosnmp"
)

// ── test helpers ──────────────────────────────────────────────────────────────

func newSNMPTestStore(t *testing.T) *repo.Store {
	t.Helper()
	cfg := &config.Config{DBPath: ":memory:", DevMode: true}
	db, err := repo.Open(cfg, migrations.Files)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return repo.NewStore(
		repo.NewAppRepo(db),
		repo.NewEventRepo(db),
		repo.NewCheckRepo(db),
		repo.NewRollupRepo(db),
		repo.NewResourceReadingRepo(db),
		repo.NewResourceRollupRepo(db),
		repo.NewInfraComponentRepo(db),
		repo.NewDockerEngineRepo(db),
		repo.NewInfraRepo(db),
		repo.NewSettingsRepo(db),
		repo.NewMetricsRepo(db),
		repo.NewUserRepo(db),
		repo.NewTraefikComponentRepo(db),
		repo.NewTraefikOverviewRepo(db),
		repo.NewTraefikServiceRepo(db),
		repo.NewDiscoveredContainerRepo(db),
		repo.NewDiscoveredRouteRepo(db),
		nil,
		nil,
		nil,
	)
}

func createSNMPTestComponent(t *testing.T, store *repo.Store, id string) {
	t.Helper()
	cfgJSON := `{"version":"2c","community":"public","port":161}`
	c := &models.InfrastructureComponent{
		ID:               id,
		Name:             "snmp-" + id,
		IP:               "127.0.0.1",
		Type:             "bare_metal",
		CollectionMethod: "snmp",
		SNMPConfig:       &cfgJSON,
		Enabled:          true,
		LastStatus:       "unknown",
		CreatedAt:        "2026-01-01T00:00:00Z",
	}
	if err := store.InfraComponents.Create(context.Background(), c); err != nil {
		t.Fatalf("create snmp test component: %v", err)
	}
}

// ── mockSNMPClient ────────────────────────────────────────────────────────────

// mockSNMPClient is a controllable snmpClient for unit tests.
type mockSNMPClient struct {
	connectErr  error
	walkResults map[string][]gosnmp.SnmpPDU // keyed by root OID
	walkErrors  map[string]error             // keyed by root OID
	getVars     []gosnmp.SnmpPDU            // returned for any Get call
	getErr      error
}

func (m *mockSNMPClient) Connect() error { return m.connectErr }
func (m *mockSNMPClient) Close() error   { return nil }

func (m *mockSNMPClient) BulkWalkAll(rootOid string) ([]gosnmp.SnmpPDU, error) {
	if m.walkErrors != nil {
		if err, ok := m.walkErrors[rootOid]; ok && err != nil {
			return nil, err
		}
	}
	if m.walkResults != nil {
		if pdus, ok := m.walkResults[rootOid]; ok {
			return pdus, nil
		}
	}
	return nil, nil
}

func (m *mockSNMPClient) Get(oids []string) (*gosnmp.SnmpPacket, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return &gosnmp.SnmpPacket{Variables: m.getVars}, nil
}

// pdu builds a SnmpPDU with the given OID and value.
func pdu(oid string, typ gosnmp.Asn1BER, value interface{}) gosnmp.SnmpPDU {
	return gosnmp.SnmpPDU{Name: oid, Type: typ, Value: value}
}

// storageEntryOID builds an hrStorageEntry OID: .1.3.6.1.2.1.25.2.3.1.{col}.{idx}
func storageEntryOID(col, idx int) string {
	return fmt.Sprintf(".1.3.6.1.2.1.25.2.3.1.%d.%d", col, idx)
}

// ── CPU tests ─────────────────────────────────────────────────────────────────

func TestSNMPPollCPU_SingleCore(t *testing.T) {
	poller := &SNMPPoller{componentID: "c1", ip: "127.0.0.1", cfg: SNMPConfig{}}
	client := &mockSNMPClient{
		walkResults: map[string][]gosnmp.SnmpPDU{
			oidHrProcessorLoad: {
				pdu(".1.3.6.1.2.1.25.3.3.1.2.1", gosnmp.Gauge32, uint(42)),
			},
		},
	}
	got, err := poller.pollCPU(client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 42.0 {
		t.Errorf("cpu_percent = %.2f, want 42.0", got)
	}
}

func TestSNMPPollCPU_MultiCore_Average(t *testing.T) {
	poller := &SNMPPoller{componentID: "c1", ip: "127.0.0.1", cfg: SNMPConfig{}}
	client := &mockSNMPClient{
		walkResults: map[string][]gosnmp.SnmpPDU{
			oidHrProcessorLoad: {
				pdu(".1.3.6.1.2.1.25.3.3.1.2.1", gosnmp.Gauge32, uint(20)),
				pdu(".1.3.6.1.2.1.25.3.3.1.2.2", gosnmp.Gauge32, uint(60)),
				pdu(".1.3.6.1.2.1.25.3.3.1.2.3", gosnmp.Gauge32, uint(40)),
				pdu(".1.3.6.1.2.1.25.3.3.1.2.4", gosnmp.Gauge32, uint(80)),
			},
		},
	}
	got, err := poller.pollCPU(client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := 50.0
	if got != want {
		t.Errorf("cpu_percent = %.2f, want %.2f", got, want)
	}
}

func TestSNMPPollCPU_WalkError(t *testing.T) {
	poller := &SNMPPoller{componentID: "c1", ip: "127.0.0.1", cfg: SNMPConfig{}}
	client := &mockSNMPClient{
		walkErrors: map[string]error{
			oidHrProcessorLoad: fmt.Errorf("timeout"),
		},
	}
	_, err := poller.pollCPU(client)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSNMPPollCPU_NoEntries(t *testing.T) {
	poller := &SNMPPoller{componentID: "c1", ip: "127.0.0.1", cfg: SNMPConfig{}}
	client := &mockSNMPClient{
		walkResults: map[string][]gosnmp.SnmpPDU{
			oidHrProcessorLoad: {},
		},
	}
	_, err := poller.pollCPU(client)
	if err == nil {
		t.Fatal("expected error for empty CPU walk, got nil")
	}
}

// ── Storage table tests ───────────────────────────────────────────────────────

// buildStorageWalk returns a set of PDUs representing one RAM entry and two disk entries.
//
//	idx 1 = RAM, 4 GB total, 2 GB used (allocUnit=1024, size=4194304, used=2097152)
//	idx 2 = C: drive, 100 GB total, 60 GB used
//	idx 3 = D: drive, 200 GB total, 50 GB used
func buildStorageWalkPDUs() []gosnmp.SnmpPDU {
	return []gosnmp.SnmpPDU{
		// idx 1 — RAM
		pdu(storageEntryOID(2, 1), gosnmp.ObjectIdentifier, oidHrStorageRam),
		pdu(storageEntryOID(3, 1), gosnmp.OctetString, []byte("Physical Memory")),
		pdu(storageEntryOID(4, 1), gosnmp.Integer, 1024),
		pdu(storageEntryOID(5, 1), gosnmp.Integer, 4194304),  // 4 GB in KB
		pdu(storageEntryOID(6, 1), gosnmp.Integer, 2097152),  // 2 GB in KB
		// idx 2 — C: drive
		pdu(storageEntryOID(2, 2), gosnmp.ObjectIdentifier, oidHrStorageFixDisk),
		pdu(storageEntryOID(3, 2), gosnmp.OctetString, []byte("C:")),
		pdu(storageEntryOID(4, 2), gosnmp.Integer, 512),
		pdu(storageEntryOID(5, 2), gosnmp.Integer, 204800000), // ~100 GB in 512-byte units
		pdu(storageEntryOID(6, 2), gosnmp.Integer, 122880000), // ~60 GB in 512-byte units
		// idx 3 — D: drive
		pdu(storageEntryOID(2, 3), gosnmp.ObjectIdentifier, oidHrStorageFixDisk),
		pdu(storageEntryOID(3, 3), gosnmp.OctetString, []byte("D:")),
		pdu(storageEntryOID(4, 3), gosnmp.Integer, 512),
		pdu(storageEntryOID(5, 3), gosnmp.Integer, 400000000), // ~200 GB
		pdu(storageEntryOID(6, 3), gosnmp.Integer, 100000000), // ~50 GB
	}
}

func TestSNMPWalkStorageTable_Memory(t *testing.T) {
	poller := &SNMPPoller{componentID: "c1", ip: "127.0.0.1", cfg: SNMPConfig{}}
	client := &mockSNMPClient{
		walkResults: map[string][]gosnmp.SnmpPDU{
			oidHrStorageEntry: buildStorageWalkPDUs(),
		},
	}
	rows, err := poller.walkStorageTable(client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pct, ok := computeMemPercent(rows)
	if !ok {
		t.Fatal("computeMemPercent: no RAM entry found")
	}
	want := 50.0
	if pct != want {
		t.Errorf("mem_percent = %.2f, want %.2f", pct, want)
	}
}

func TestSNMPWalkStorageTable_MemoryBytes(t *testing.T) {
	poller := &SNMPPoller{componentID: "c1", ip: "127.0.0.1", cfg: SNMPConfig{}}
	client := &mockSNMPClient{
		walkResults: map[string][]gosnmp.SnmpPDU{
			oidHrStorageEntry: buildStorageWalkPDUs(),
		},
	}
	rows, err := poller.walkStorageTable(client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mem, ok := computeMemResult(rows)
	if !ok {
		t.Fatal("computeMemResult: no RAM entry found")
	}
	// allocUnit=1024, size=4194304 → total = 4 GB = 4294967296 bytes
	wantTotal := int64(4194304) * 1024
	if mem.totalBytes != wantTotal {
		t.Errorf("total_bytes = %d, want %d", mem.totalBytes, wantTotal)
	}
	// used=2097152 → used = 2 GB = 2147483648 bytes
	wantUsed := int64(2097152) * 1024
	if mem.usedBytes != wantUsed {
		t.Errorf("used_bytes = %d, want %d", mem.usedBytes, wantUsed)
	}
	if mem.percent != 50.0 {
		t.Errorf("percent = %.2f, want 50.0", mem.percent)
	}
}

func TestSNMPWalkStorageTable_Disks(t *testing.T) {
	poller := &SNMPPoller{componentID: "c1", ip: "127.0.0.1", cfg: SNMPConfig{}}
	client := &mockSNMPClient{
		walkResults: map[string][]gosnmp.SnmpPDU{
			oidHrStorageEntry: buildStorageWalkPDUs(),
		},
	}
	rows, err := poller.walkStorageTable(client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	disks := computeDiskPercents(rows)
	if len(disks) != 2 {
		t.Fatalf("expected 2 disk entries, got %d", len(disks))
	}

	// C: should be ~60%
	cPct, ok := disks["c"]
	if !ok {
		t.Fatal("disk_percent_c not found")
	}
	if cPct < 59.9 || cPct > 60.1 {
		t.Errorf("disk_percent_c = %.4f, want ~60.0", cPct)
	}

	// D: should be ~25%
	dPct, ok := disks["d"]
	if !ok {
		t.Fatal("disk_percent_d not found")
	}
	if dPct < 24.9 || dPct > 25.1 {
		t.Errorf("disk_percent_d = %.4f, want ~25.0", dPct)
	}
}

func TestSNMPWalkStorageTable_DiskResults_Bytes(t *testing.T) {
	poller := &SNMPPoller{componentID: "c1", ip: "127.0.0.1", cfg: SNMPConfig{}}
	client := &mockSNMPClient{
		walkResults: map[string][]gosnmp.SnmpPDU{
			oidHrStorageEntry: buildStorageWalkPDUs(),
		},
	}
	rows, err := poller.walkStorageTable(client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	results := computeDiskResults(rows)
	if len(results) != 2 {
		t.Fatalf("expected 2 disk results, got %d", len(results))
	}

	// Find C: entry by label
	var cDisk *diskResult
	for i := range results {
		if results[i].label == "C:" {
			cDisk = &results[i]
			break
		}
	}
	if cDisk == nil {
		t.Fatal("disk with label 'C:' not found")
	}

	// C: allocUnit=512, size=204800000 → total = 104857600000 bytes (~100 GB)
	wantTotal := int64(204800000) * 512
	if cDisk.totalBytes != wantTotal {
		t.Errorf("C: total_bytes = %d, want %d", cDisk.totalBytes, wantTotal)
	}
	wantUsed := int64(122880000) * 512
	if cDisk.usedBytes != wantUsed {
		t.Errorf("C: used_bytes = %d, want %d", cDisk.usedBytes, wantUsed)
	}
	if cDisk.percent < 59.9 || cDisk.percent > 60.1 {
		t.Errorf("C: percent = %.4f, want ~60.0", cDisk.percent)
	}
}

func TestSNMPWalkStorageTable_LinuxRoot(t *testing.T) {
	pdus := []gosnmp.SnmpPDU{
		pdu(storageEntryOID(2, 1), gosnmp.ObjectIdentifier, oidHrStorageFixDisk),
		pdu(storageEntryOID(3, 1), gosnmp.OctetString, []byte("/")),
		pdu(storageEntryOID(4, 1), gosnmp.Integer, 4096),
		pdu(storageEntryOID(5, 1), gosnmp.Integer, 10000000),
		pdu(storageEntryOID(6, 1), gosnmp.Integer, 3000000),
	}
	poller := &SNMPPoller{componentID: "c1", ip: "127.0.0.1", cfg: SNMPConfig{}}
	client := &mockSNMPClient{
		walkResults: map[string][]gosnmp.SnmpPDU{
			oidHrStorageEntry: pdus,
		},
	}
	rows, err := poller.walkStorageTable(client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	disks := computeDiskPercents(rows)
	if _, ok := disks["root"]; !ok {
		t.Errorf("expected disk label 'root' for '/', got keys: %v", disks)
	}

	// Also verify computeDiskResults returns "/" as the label (not "root").
	results := computeDiskResults(rows)
	if len(results) != 1 {
		t.Fatalf("expected 1 disk result, got %d", len(results))
	}
	if results[0].label != "/" {
		t.Errorf("label = %q, want %q", results[0].label, "/")
	}
}

// ── System info tests ─────────────────────────────────────────────────────────

func TestSNMPPollSystemInfo_HappyPath(t *testing.T) {
	poller := &SNMPPoller{componentID: "c1", ip: "127.0.0.1", cfg: SNMPConfig{}}
	client := &mockSNMPClient{
		getVars: []gosnmp.SnmpPDU{
			pdu(oidSysDescr, gosnmp.OctetString, []byte("Linux proxmox 6.8.12 #1 SMP x86_64")),
			pdu(oidSysUpTime, gosnmp.TimeTicks, uint32(12_276_000)), // 14d 3h 0m (approx)
			pdu(oidSysName, gosnmp.OctetString, []byte("proxmox-node1")),
		},
	}
	result, err := poller.pollSystemInfo(client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OSDescription != "Linux proxmox 6.8.12 #1 SMP x86_64" {
		t.Errorf("os_description = %q", result.OSDescription)
	}
	if result.Hostname != "proxmox-node1" {
		t.Errorf("hostname = %q, want %q", result.Hostname, "proxmox-node1")
	}
	if result.Uptime == "" {
		t.Error("uptime should not be empty")
	}
}

func TestSNMPPollSystemInfo_GetError(t *testing.T) {
	poller := &SNMPPoller{componentID: "c1", ip: "127.0.0.1", cfg: SNMPConfig{}}
	client := &mockSNMPClient{
		getErr: fmt.Errorf("no such object"),
	}
	_, err := poller.pollSystemInfo(client)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── Timetick parsing tests ────────────────────────────────────────────────────

func TestTicksToUptime(t *testing.T) {
	cases := []struct {
		ticks uint32
		want  string
	}{
		{0, "0m"},
		{6000, "1m"},        // 60 seconds
		{360000, "1h 0m"},   // 1 hour
		{8640000, "1d 0h 0m"}, // 1 day
		{122_040_000, "14d 3h 0m"}, // ~14 days
	}
	for _, tc := range cases {
		got := ticksToUptime(tc.ticks)
		if got != tc.want {
			t.Errorf("ticksToUptime(%d) = %q, want %q", tc.ticks, got, tc.want)
		}
	}
}

// ── Full Poll integration tests ───────────────────────────────────────────────

func TestSNMPPoller_Poll_HappyPath(t *testing.T) {
	store := newSNMPTestStore(t)
	createSNMPTestComponent(t, store, "snmp-01")

	poller, err := NewSNMPPoller("snmp-01", "127.0.0.1", `{"version":"2c","community":"public"}`)
	if err != nil {
		t.Fatalf("NewSNMPPoller: %v", err)
	}

	mockClient := &mockSNMPClient{
		walkResults: map[string][]gosnmp.SnmpPDU{
			oidHrProcessorLoad: {
				pdu(".1.3.6.1.2.1.25.3.3.1.2.1", gosnmp.Gauge32, uint(30)),
				pdu(".1.3.6.1.2.1.25.3.3.1.2.2", gosnmp.Gauge32, uint(70)),
			},
			oidHrStorageEntry: buildStorageWalkPDUs(),
		},
		getVars: []gosnmp.SnmpPDU{
			pdu(oidSysDescr, gosnmp.OctetString, []byte("Linux test 5.15")),
			pdu(oidSysUpTime, gosnmp.TimeTicks, uint32(360000)),
			pdu(oidSysName, gosnmp.OctetString, []byte("testhost")),
		},
	}
	poller.newClient = func() snmpClient { return mockClient }

	ctx := context.Background()
	if err := poller.Poll(ctx, store); err != nil {
		t.Fatalf("Poll: %v", err)
	}

	// Component should be marked online.
	comp, err := store.InfraComponents.Get(ctx, "snmp-01")
	if err != nil {
		t.Fatalf("get component: %v", err)
	}
	if comp.LastStatus != "online" {
		t.Errorf("last_status = %q, want %q", comp.LastStatus, "online")
	}
	if comp.LastPolledAt == nil {
		t.Error("last_polled_at should be set")
	}

	// snmp_meta should be written and parseable.
	if comp.SNMPMeta == nil || *comp.SNMPMeta == "" {
		t.Fatal("snmp_meta should be set after poll")
	}
	var meta SNMPMeta
	if err := json.Unmarshal([]byte(*comp.SNMPMeta), &meta); err != nil {
		t.Fatalf("parse snmp_meta JSON: %v", err)
	}
	if meta.Hostname != "testhost" {
		t.Errorf("hostname = %q, want %q", meta.Hostname, "testhost")
	}
	if meta.OSDescription != "Linux test 5.15" {
		t.Errorf("os_description = %q, want %q", meta.OSDescription, "Linux test 5.15")
	}
	if meta.Memory.TotalBytes == 0 {
		t.Error("memory total_bytes should be non-zero")
	}
	if meta.Memory.Percent != 50.0 {
		t.Errorf("memory percent = %.2f, want 50.0", meta.Memory.Percent)
	}
	if len(meta.Disks) != 2 {
		t.Errorf("expected 2 disk entries in snmp_meta, got %d", len(meta.Disks))
	}
}

func TestSNMPPoller_Poll_ConnectFailure(t *testing.T) {
	store := newSNMPTestStore(t)
	createSNMPTestComponent(t, store, "snmp-02")

	poller, err := NewSNMPPoller("snmp-02", "192.0.2.1", `{"version":"2c","community":"public"}`)
	if err != nil {
		t.Fatalf("NewSNMPPoller: %v", err)
	}
	poller.newClient = func() snmpClient {
		return &mockSNMPClient{connectErr: fmt.Errorf("connection refused")}
	}

	err = poller.Poll(context.Background(), store)
	if err == nil {
		t.Fatal("expected error on connect failure, got nil")
	}
}

func TestSNMPPoller_Poll_DegradedWhenCPUFails(t *testing.T) {
	store := newSNMPTestStore(t)
	createSNMPTestComponent(t, store, "snmp-03")

	poller, err := NewSNMPPoller("snmp-03", "127.0.0.1", `{"version":"2c","community":"public"}`)
	if err != nil {
		t.Fatalf("NewSNMPPoller: %v", err)
	}
	poller.newClient = func() snmpClient {
		return &mockSNMPClient{
			walkErrors: map[string]error{
				oidHrProcessorLoad: fmt.Errorf("no such object"),
			},
			walkResults: map[string][]gosnmp.SnmpPDU{
				oidHrStorageEntry: buildStorageWalkPDUs(),
			},
		}
	}

	if err := poller.Poll(context.Background(), store); err != nil {
		t.Fatalf("Poll: %v", err)
	}

	comp, err := store.InfraComponents.Get(context.Background(), "snmp-03")
	if err != nil {
		t.Fatalf("get component: %v", err)
	}
	if comp.LastStatus != "degraded" {
		t.Errorf("last_status = %q, want %q", comp.LastStatus, "degraded")
	}
}

// ── SNMPConfig parsing tests ──────────────────────────────────────────────────

func TestNewSNMPPoller_DefaultPort(t *testing.T) {
	p, err := NewSNMPPoller("c1", "10.0.0.1", `{"version":"2c","community":"public"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.cfg.Port != 161 {
		t.Errorf("default port = %d, want 161", p.cfg.Port)
	}
}

func TestNewSNMPPoller_V3ConfigParsed(t *testing.T) {
	cfgJSON := `{
		"version": "3",
		"community": "admin",
		"port": 161,
		"auth_protocol": "SHA",
		"auth_passphrase": "authpass",
		"priv_protocol": "AES",
		"priv_passphrase": "privpass",
		"context_name": "myctx"
	}`
	p, err := NewSNMPPoller("c1", "10.0.0.1", cfgJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.cfg.Version != "3" {
		t.Errorf("version = %q, want %q", p.cfg.Version, "3")
	}
	if p.cfg.ContextName != "myctx" {
		t.Errorf("context_name = %q, want %q", p.cfg.ContextName, "myctx")
	}
	// buildGoSNMPClient must not panic with v3 config.
	_ = p.buildGoSNMPClient()
}

func TestNewSNMPPoller_InvalidJSON(t *testing.T) {
	_, err := NewSNMPPoller("c1", "10.0.0.1", `{bad json}`)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// ── Label sanitisation tests ──────────────────────────────────────────────────

func TestSanitizeDiskLabel(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"/", "root"},
		{"C:", "c"},
		{"D:", "d"},
		{"/dev/sda1", "dev_sda1"},
		{"Label /mnt/data", "label__mnt_data"},
		{"", "disk"},
		{"   ", "disk"},
	}
	for _, tc := range cases {
		got := sanitizeDiskLabel(tc.input)
		if got != tc.want {
			t.Errorf("sanitizeDiskLabel(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ── Context timeout test ──────────────────────────────────────────────────────

func TestSNMPPoller_Poll_ContextCancelled(t *testing.T) {
	store := newSNMPTestStore(t)
	createSNMPTestComponent(t, store, "snmp-04")

	poller, err := NewSNMPPoller("snmp-04", "127.0.0.1", `{"version":"2c","community":"public"}`)
	if err != nil {
		t.Fatalf("NewSNMPPoller: %v", err)
	}

	// Connect succeeds, but the context is already cancelled when Poll is called.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(1 * time.Millisecond) // ensure cancellation

	poller.newClient = func() snmpClient {
		return &mockSNMPClient{
			walkResults: map[string][]gosnmp.SnmpPDU{
				oidHrProcessorLoad: {},
				oidHrStorageEntry:  {},
			},
		}
	}

	// Should complete (may log errors) but must not panic.
	_ = poller.Poll(ctx, store)
}
