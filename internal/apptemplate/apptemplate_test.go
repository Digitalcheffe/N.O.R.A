package apptemplate_test

import (
	"testing"
	"testing/fstest"

	"github.com/digitalcheffe/nora/internal/apptemplate"
)

const sonarrYAML = `
meta:
  name: Sonarr
  category: Media
  logo: sonarr.png
  description: TV series management
  capability: full
webhook:
  field_mappings:
    event_type: "$.eventType"
    series_title: "$.series.title"
    episode_title: "$.episodes[0].title"
    season_number: "$.episodes[0].seasonNumber"
    episode_number: "$.episodes[0].episodeNumber"
    health_type: "$.healthCheck.type"
  severity_field: event_type
  display_template: "{event_type} — {series_title} S{season_number}E{episode_number}"
  severity_mapping:
    Download: info
    HealthIssue: warn
    ApplicationUpdate: info
monitor:
  check_type: url
  check_url: "{base_url}/api/v3/system/status"
  healthy_status: 200
  check_interval: 5m
digest:
  categories:
    - label: Downloads
      match_field: event_type
      match_value: Download
`

const simpleYAML = `
meta:
  name: Simple
  category: Infrastructure
  logo: simple.png
  description: A simple app
  capability: monitor_only
monitor:
  check_type: ping
  check_interval: 1m
`

func newTestRegistry(t *testing.T) *apptemplate.Registry {
	t.Helper()
	fsys := fstest.MapFS{
		"sonarr.yaml": {Data: []byte(sonarrYAML)},
		"simple.yaml": {Data: []byte(simpleYAML)},
	}
	reg, err := apptemplate.NewRegistry(fsys)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return reg
}

// TestNewRegistry_LoadsTemplates verifies all YAML files in the FS are loaded.
func TestNewRegistry_LoadsTemplates(t *testing.T) {
	reg := newTestRegistry(t)
	all := reg.List()
	if len(all) != 2 {
		t.Fatalf("want 2 templates, got %d", len(all))
	}
	p, ok := all["sonarr"]
	if !ok {
		t.Fatal("sonarr template not found")
	}
	if p.Meta.Name != "Sonarr" {
		t.Errorf("want name Sonarr, got %q", p.Meta.Name)
	}
}

// TestGet verifies Get returns the correct template and nil for unknown IDs.
func TestGet(t *testing.T) {
	reg := newTestRegistry(t)

	p, err := reg.Get("sonarr")
	if err != nil || p == nil {
		t.Fatalf("want sonarr template, got err=%v p=%v", err, p)
	}
	if p.Meta.Category != "Media" {
		t.Errorf("want category Media, got %q", p.Meta.Category)
	}

	missing, err := reg.Get("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if missing != nil {
		t.Errorf("want nil for unknown template, got %+v", missing)
	}
}

// TestExtractFields verifies nested field extraction including array indexing.
func TestExtractFields(t *testing.T) {
	reg := newTestRegistry(t)

	payload := []byte(`{
		"eventType": "Download",
		"series": {"title": "The Expanse"},
		"episodes": [
			{"title": "Pilot", "seasonNumber": 1, "episodeNumber": 1}
		],
		"healthCheck": {"type": "IndexerSearch"}
	}`)

	fields, err := reg.ExtractFields("sonarr", payload)
	if err != nil {
		t.Fatalf("ExtractFields error: %v", err)
	}

	cases := map[string]string{
		"event_type":     "Download",
		"series_title":   "The Expanse",
		"episode_title":  "Pilot",
		"season_number":  "1",
		"episode_number": "1",
		"health_type":    "IndexerSearch",
	}
	for tag, want := range cases {
		if got := fields[tag]; got != want {
			t.Errorf("fields[%q] = %q, want %q", tag, got, want)
		}
	}
}

// TestExtractFields_UnknownTemplate returns empty map for an unregistered template.
func TestExtractFields_UnknownTemplate(t *testing.T) {
	reg := newTestRegistry(t)
	fields, err := reg.ExtractFields("ghost", []byte(`{"x":1}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fields) != 0 {
		t.Errorf("want empty map, got %v", fields)
	}
}

// TestExtractFields_InvalidJSON returns an error on malformed payloads.
func TestExtractFields_InvalidJSON(t *testing.T) {
	reg := newTestRegistry(t)
	_, err := reg.ExtractFields("sonarr", []byte(`not json`))
	if err == nil {
		t.Error("want error for invalid JSON, got nil")
	}
}

// TestRenderDisplayText verifies {token} substitution from extracted fields.
func TestRenderDisplayText(t *testing.T) {
	reg := newTestRegistry(t)

	fields := map[string]string{
		"event_type":     "Download",
		"series_title":   "The Expanse",
		"season_number":  "1",
		"episode_number": "1",
	}

	got := reg.RenderDisplayText("sonarr", fields)
	want := "Download — The Expanse S1E1"
	if got != want {
		t.Errorf("RenderDisplayText = %q, want %q", got, want)
	}
}

// TestRenderDisplayText_UnknownTemplate returns default text for unknown template.
func TestRenderDisplayText_UnknownTemplate(t *testing.T) {
	reg := newTestRegistry(t)
	got := reg.RenderDisplayText("ghost", map[string]string{})
	if got != "Event received" {
		t.Errorf("want %q, got %q", "Event received", got)
	}
}

// TestRenderDisplayText_NoTemplate returns default for template without display template.
func TestRenderDisplayText_NoTemplate(t *testing.T) {
	reg := newTestRegistry(t)
	got := reg.RenderDisplayText("simple", map[string]string{})
	if got != "Event received" {
		t.Errorf("want %q, got %q", "Event received", got)
	}
}

// TestMapSeverity verifies known event values map to correct severity levels.
func TestMapSeverity(t *testing.T) {
	reg := newTestRegistry(t)

	cases := []struct {
		eventType string
		want      string
	}{
		{"Download", "info"},
		{"HealthIssue", "warn"},
		{"ApplicationUpdate", "info"},
		{"UnknownEvent", "info"},
		{"", "info"},
	}

	for _, c := range cases {
		fields := map[string]string{"event_type": c.eventType}
		got := reg.MapSeverity("sonarr", fields)
		if got != c.want {
			t.Errorf("MapSeverity(event_type=%q) = %q, want %q", c.eventType, got, c.want)
		}
	}
}

// TestMapSeverity_UnknownTemplate returns "info" for an unregistered template.
func TestMapSeverity_UnknownTemplate(t *testing.T) {
	reg := newTestRegistry(t)
	got := reg.MapSeverity("ghost", map[string]string{"event_type": "Anything"})
	if got != "info" {
		t.Errorf("want info, got %q", got)
	}
}

// TestMapSeverity_NoSeverityConfig returns "info" when template has no severity config.
func TestMapSeverity_NoSeverityConfig(t *testing.T) {
	reg := newTestRegistry(t)
	got := reg.MapSeverity("simple", map[string]string{"event_type": "Anything"})
	if got != "info" {
		t.Errorf("want info, got %q", got)
	}
}

// ---- n8n ----

const n8nYAML = `
meta:
  name: n8n
  category: Automation
  logo: n8n.png
  description: Workflow automation platform
  capability: full
webhook:
  field_mappings:
    event_type: "$.eventName"
    workflow_name: "$.workflowData.name"
    error_message: "$.error.message"
  severity_field: event_type
  display_template: "{event_type} — {workflow_name}"
  severity_mapping:
    "workflow.finished": info
    "workflow.error": error
    "node.error": warn
monitor:
  check_type: url
  check_url: "{base_url}/healthz"
  healthy_status: 200
  check_interval: 5m
digest:
  categories:
    - label: Workflows
      match_field: event_type
      match_value: "workflow.finished"
      match_severity: ""
    - label: Errors
      match_field: ""
      match_value: ""
      match_severity: error
`

func newN8nRegistry(t *testing.T) *apptemplate.Registry {
	t.Helper()
	fsys := fstest.MapFS{
		"n8n.yaml": {Data: []byte(n8nYAML)},
	}
	reg, err := apptemplate.NewRegistry(fsys)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return reg
}

// TestN8n_ExtractFields verifies n8n field extraction from a workflow error payload.
func TestN8n_ExtractFields(t *testing.T) {
	reg := newN8nRegistry(t)

	payload := []byte(`{
		"eventName": "workflow.error",
		"workflowData": {"name": "Daily Sync", "id": "42"},
		"executionId": "99",
		"error": {"message": "Connection refused"}
	}`)

	fields, err := reg.ExtractFields("n8n", payload)
	if err != nil {
		t.Fatalf("ExtractFields error: %v", err)
	}

	cases := map[string]string{
		"event_type":    "workflow.error",
		"workflow_name": "Daily Sync",
		"error_message": "Connection refused",
	}
	for tag, want := range cases {
		if got := fields[tag]; got != want {
			t.Errorf("fields[%q] = %q, want %q", tag, got, want)
		}
	}
}

// TestN8n_SeverityMapping verifies n8n event names map to correct severity levels.
func TestN8n_SeverityMapping(t *testing.T) {
	reg := newN8nRegistry(t)

	cases := []struct {
		event string
		want  string
	}{
		{"workflow.finished", "info"},
		{"workflow.error", "error"},
		{"node.error", "warn"},
		{"unknown.event", "info"},
	}

	for _, c := range cases {
		fields := map[string]string{"event_type": c.event}
		got := reg.MapSeverity("n8n", fields)
		if got != c.want {
			t.Errorf("MapSeverity(event=%q) = %q, want %q", c.event, got, c.want)
		}
	}
}

// ---- Duplicati ----

const duplicatiYAML = `
meta:
  name: Duplicati
  category: Storage
  logo: duplicati.png
  description: Open-source backup software
  capability: webhook_only
webhook:
  field_mappings:
    event_type: "$.Data.ParsedResult"
    backup_name: "$.Data.OperationName"
    duration: "$.Data.Duration"
    error_message: "$.Data.Message"
  severity_field: event_type
  display_template: "Backup {event_type} — {backup_name}"
  severity_mapping:
    Success: info
    Warning: warn
    Error: error
    Fatal: critical
digest:
  categories:
    - label: Backups
      match_field: event_type
      match_value: Success
      match_severity: ""
    - label: Backup Errors
      match_field: ""
      match_value: ""
      match_severity: error
`

func newDuplicatiRegistry(t *testing.T) *apptemplate.Registry {
	t.Helper()
	fsys := fstest.MapFS{
		"duplicati.yaml": {Data: []byte(duplicatiYAML)},
	}
	reg, err := apptemplate.NewRegistry(fsys)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return reg
}

// TestDuplicati_ExtractFields verifies Duplicati field extraction from a backup payload.
func TestDuplicati_ExtractFields(t *testing.T) {
	reg := newDuplicatiRegistry(t)

	payload := []byte(`{
		"Data": {
			"ParsedResult": "Success",
			"OperationName": "Home NAS",
			"Duration": "00:03:21",
			"Message": ""
		}
	}`)

	fields, err := reg.ExtractFields("duplicati", payload)
	if err != nil {
		t.Fatalf("ExtractFields error: %v", err)
	}

	cases := map[string]string{
		"event_type":  "Success",
		"backup_name": "Home NAS",
		"duration":    "00:03:21",
	}
	for tag, want := range cases {
		if got := fields[tag]; got != want {
			t.Errorf("fields[%q] = %q, want %q", tag, got, want)
		}
	}
}

// TestDuplicati_SeverityMapping verifies Success/Failure map to correct severity levels.
func TestDuplicati_SeverityMapping(t *testing.T) {
	reg := newDuplicatiRegistry(t)

	cases := []struct {
		result string
		want   string
	}{
		{"Success", "info"},
		{"Warning", "warn"},
		{"Error", "error"},
		{"Fatal", "critical"},
		{"Unknown", "info"},
	}

	for _, c := range cases {
		fields := map[string]string{"event_type": c.result}
		got := reg.MapSeverity("duplicati", fields)
		if got != c.want {
			t.Errorf("MapSeverity(result=%q) = %q, want %q", c.result, got, c.want)
		}
	}
}

// TestDuplicati_RenderDisplayText verifies display template renders correctly.
func TestDuplicati_RenderDisplayText(t *testing.T) {
	reg := newDuplicatiRegistry(t)

	fields := map[string]string{
		"event_type":  "Success",
		"backup_name": "Home NAS",
	}
	got := reg.RenderDisplayText("duplicati", fields)
	want := "Backup Success — Home NAS"
	if got != want {
		t.Errorf("RenderDisplayText = %q, want %q", got, want)
	}
}
