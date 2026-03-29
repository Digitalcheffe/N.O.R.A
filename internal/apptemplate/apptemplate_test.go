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
    season: "$.episodes[0].seasonNumber"
    episode: "$.episodes[0].episodeNumber"
    message: "$.message"
  severity_field: event_type
  severity_compound_field: level
  display_template: "{event_type} — {series_title}"
  display_templates:
    Download: "Download — {series_title} S{season}E{episode}"
    Grab: "Grabbed — {series_title} S{season}E{episode}"
    Health: "Health Issue — {message}"
    HealthRestored: "Health Restored — {message}"
  severity_mapping:
    Download: info
    Health: warn
    Health:error: error
    Health:warning: warn
    HealthRestored: info
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
		"message": ""
	}`)

	fields, err := reg.ExtractFields("sonarr", payload)
	if err != nil {
		t.Fatalf("ExtractFields error: %v", err)
	}

	cases := map[string]string{
		"event_type":    "Download",
		"series_title":  "The Expanse",
		"episode_title": "Pilot",
		"season":        "1",
		"episode":       "1",
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

// TestRenderDisplayText verifies per-eventType template is selected for Download events.
func TestRenderDisplayText(t *testing.T) {
	reg := newTestRegistry(t)

	fields := map[string]string{
		"event_type":   "Download",
		"series_title": "The Expanse",
		"season":       "1",
		"episode":      "1",
	}

	got := reg.RenderDisplayText("sonarr", fields)
	want := "Download — The Expanse S1E1"
	if got != want {
		t.Errorf("RenderDisplayText = %q, want %q", got, want)
	}
}

// TestRenderDisplayText_PerEventType verifies each per-eventType template renders correctly.
func TestRenderDisplayText_PerEventType(t *testing.T) {
	reg := newTestRegistry(t)

	cases := []struct {
		fields map[string]string
		want   string
	}{
		{
			fields: map[string]string{
				"event_type":   "Download",
				"series_title": "The Expanse",
				"season":       "2",
				"episode":      "5",
			},
			want: "Download — The Expanse S2E5",
		},
		{
			fields: map[string]string{
				"event_type":   "Grab",
				"series_title": "Breaking Bad",
				"season":       "3",
				"episode":      "10",
			},
			want: "Grabbed — Breaking Bad S3E10",
		},
		{
			fields: map[string]string{
				"event_type": "Health",
				"message":    "Indexer search failed",
			},
			want: "Health Issue — Indexer search failed",
		},
		{
			fields: map[string]string{
				"event_type": "HealthRestored",
				"message":    "Indexer back online",
			},
			want: "Health Restored — Indexer back online",
		},
	}

	for _, c := range cases {
		got := reg.RenderDisplayText("sonarr", c.fields)
		if got != c.want {
			t.Errorf("RenderDisplayText(event_type=%q) = %q, want %q", c.fields["event_type"], got, c.want)
		}
	}
}

// TestRenderDisplayText_FallbackTemplate verifies unknown eventTypes use the fallback display_template.
func TestRenderDisplayText_FallbackTemplate(t *testing.T) {
	reg := newTestRegistry(t)

	fields := map[string]string{
		"event_type":   "Test",
		"series_title": "Sonarr",
	}
	got := reg.RenderDisplayText("sonarr", fields)
	want := "Test — Sonarr"
	if got != want {
		t.Errorf("RenderDisplayText(fallback) = %q, want %q", got, want)
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
		{"Health", "warn"},
		{"HealthRestored", "info"},
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

// TestMapSeverity_Compound verifies compound key lookup for Health events with level field.
func TestMapSeverity_Compound(t *testing.T) {
	reg := newTestRegistry(t)

	cases := []struct {
		eventType string
		level     string
		want      string
	}{
		{"Health", "error", "error"},
		{"Health", "warning", "warn"},
		{"Health", "", "warn"},    // no level — falls back to plain Health key
		{"Health", "unknown", "warn"}, // unknown level — falls back to plain Health key
		{"HealthRestored", "error", "info"}, // no compound key defined, falls back to HealthRestored
		{"Download", "error", "info"},       // Download has no compound key
	}

	for _, c := range cases {
		fields := map[string]string{"event_type": c.eventType, "level": c.level}
		got := reg.MapSeverity("sonarr", fields)
		if got != c.want {
			t.Errorf("MapSeverity(event_type=%q, level=%q) = %q, want %q", c.eventType, c.level, got, c.want)
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

// ---- Plex ----

const plexYAML = `
meta:
  name: Plex
  category: Media
  logo: plex.png
  description: Personal media server and streaming platform
  capability: full
webhook:
  field_mappings:
    event: "$.event"
    metadata_title: "$.Metadata.title"
    metadata_type: "$.Metadata.type"
    grandparent_title: "$.Metadata.grandparentTitle"
    account_title: "$.Account.title"
  severity_field: event
  display_template: "{event} — {metadata_title} ({account_title})"
  severity_mapping:
    media.play: info
    media.pause: info
    media.resume: info
    media.stop: info
    media.scrobble: info
    library.new: info
monitor:
  check_type: url
  check_url: "{base_url}:32400/identity"
  healthy_status: 200
  check_interval: 5m
digest:
  categories:
    - label: Activity
      match_field: ""
      match_value: ""
      match_severity: ""
`

func newPlexRegistry(t *testing.T) *apptemplate.Registry {
	t.Helper()
	fsys := fstest.MapFS{
		"plex.yaml": {Data: []byte(plexYAML)},
	}
	reg, err := apptemplate.NewRegistry(fsys)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return reg
}

// TestPlex_ExtractFields verifies Plex field extraction from a media.play payload.
func TestPlex_ExtractFields(t *testing.T) {
	reg := newPlexRegistry(t)

	payload := []byte(`{
		"event": "media.play",
		"Account": {"title": "homeuser"},
		"Metadata": {
			"type": "episode",
			"title": "Pilot",
			"grandparentTitle": "The Expanse"
		}
	}`)

	fields, err := reg.ExtractFields("plex", payload)
	if err != nil {
		t.Fatalf("ExtractFields error: %v", err)
	}

	cases := map[string]string{
		"event":           "media.play",
		"metadata_title":  "Pilot",
		"metadata_type":   "episode",
		"grandparent_title": "The Expanse",
		"account_title":   "homeuser",
	}
	for tag, want := range cases {
		if got := fields[tag]; got != want {
			t.Errorf("fields[%q] = %q, want %q", tag, got, want)
		}
	}
}

// TestPlex_RenderDisplayText verifies the display template substitutes event and account fields.
func TestPlex_RenderDisplayText(t *testing.T) {
	reg := newPlexRegistry(t)

	fields := map[string]string{
		"event":          "media.play",
		"metadata_title": "Pilot",
		"account_title":  "homeuser",
	}
	got := reg.RenderDisplayText("plex", fields)
	want := "media.play — Pilot (homeuser)"
	if got != want {
		t.Errorf("RenderDisplayText = %q, want %q", got, want)
	}
}

// TestPlex_SeverityMapping verifies all known Plex event types map to info.
func TestPlex_SeverityMapping(t *testing.T) {
	reg := newPlexRegistry(t)

	events := []string{"media.play", "media.pause", "media.resume", "media.stop", "media.scrobble", "library.new", "unknown.event"}
	for _, ev := range events {
		fields := map[string]string{"event": ev}
		got := reg.MapSeverity("plex", fields)
		if got != "info" {
			t.Errorf("MapSeverity(event=%q) = %q, want info", ev, got)
		}
	}
}

// ---- Home Assistant ----

const homeassistantYAML = `
meta:
  name: Home Assistant
  category: Automation
  logo: homeassistant.png
  description: Open-source home automation platform
  capability: full
webhook:
  field_mappings:
    event_type: "$.event_type"
    entity_id: "$.entity_id"
    new_state_state: "$.new_state.state"
    friendly_name: "$.new_state.attributes.friendly_name"
  severity_field: event_type
  display_template: "{event_type} — {friendly_name}: {new_state_state}"
  severity_mapping:
    state_changed: info
    automation_triggered: info
    script_started: info
monitor:
  check_type: url
  check_url: "{base_url}/api/"
  auth_header: "Authorization: Bearer {token}"
  healthy_status: 200
  check_interval: 5m
digest:
  categories:
    - label: Events
      match_field: ""
      match_value: ""
      match_severity: ""
`

func newHomeAssistantRegistry(t *testing.T) *apptemplate.Registry {
	t.Helper()
	fsys := fstest.MapFS{
		"homeassistant.yaml": {Data: []byte(homeassistantYAML)},
	}
	reg, err := apptemplate.NewRegistry(fsys)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return reg
}

// TestHomeAssistant_ExtractFields verifies nested field extraction including attributes.
func TestHomeAssistant_ExtractFields(t *testing.T) {
	reg := newHomeAssistantRegistry(t)

	payload := []byte(`{
		"event_type": "state_changed",
		"entity_id": "light.living_room",
		"new_state": {
			"state": "on",
			"attributes": {
				"friendly_name": "Living Room Light"
			}
		}
	}`)

	fields, err := reg.ExtractFields("homeassistant", payload)
	if err != nil {
		t.Fatalf("ExtractFields error: %v", err)
	}

	cases := map[string]string{
		"event_type":      "state_changed",
		"entity_id":       "light.living_room",
		"new_state_state": "on",
		"friendly_name":   "Living Room Light",
	}
	for tag, want := range cases {
		if got := fields[tag]; got != want {
			t.Errorf("fields[%q] = %q, want %q", tag, got, want)
		}
	}
}

// TestHomeAssistant_RenderDisplayText verifies template renders with nested state fields.
func TestHomeAssistant_RenderDisplayText(t *testing.T) {
	reg := newHomeAssistantRegistry(t)

	fields := map[string]string{
		"event_type":      "state_changed",
		"friendly_name":   "Living Room Light",
		"new_state_state": "on",
	}
	got := reg.RenderDisplayText("homeassistant", fields)
	want := "state_changed — Living Room Light: on"
	if got != want {
		t.Errorf("RenderDisplayText = %q, want %q", got, want)
	}
}

// TestHomeAssistant_SeverityMapping verifies known HA event types map to info.
func TestHomeAssistant_SeverityMapping(t *testing.T) {
	reg := newHomeAssistantRegistry(t)

	cases := []struct {
		event string
		want  string
	}{
		{"state_changed", "info"},
		{"automation_triggered", "info"},
		{"script_started", "info"},
		{"unknown_event", "info"},
	}

	for _, c := range cases {
		fields := map[string]string{"event_type": c.event}
		got := reg.MapSeverity("homeassistant", fields)
		if got != c.want {
			t.Errorf("MapSeverity(event=%q) = %q, want %q", c.event, got, c.want)
		}
	}
}
