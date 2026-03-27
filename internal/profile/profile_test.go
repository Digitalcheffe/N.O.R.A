package profile_test

import (
	"testing"
	"testing/fstest"

	"github.com/digitalcheffe/nora/internal/profile"
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

func newTestRegistry(t *testing.T) *profile.Registry {
	t.Helper()
	fsys := fstest.MapFS{
		"sonarr.yaml": {Data: []byte(sonarrYAML)},
		"simple.yaml": {Data: []byte(simpleYAML)},
	}
	reg, err := profile.NewRegistry(fsys)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return reg
}

// TestNewRegistry_LoadsProfiles verifies all YAML files in the FS are loaded.
func TestNewRegistry_LoadsProfiles(t *testing.T) {
	reg := newTestRegistry(t)
	all := reg.List()
	if len(all) != 2 {
		t.Fatalf("want 2 profiles, got %d", len(all))
	}
	p, ok := all["sonarr"]
	if !ok {
		t.Fatal("sonarr profile not found")
	}
	if p.Meta.Name != "Sonarr" {
		t.Errorf("want name Sonarr, got %q", p.Meta.Name)
	}
}

// TestGet verifies Get returns the correct profile and nil for unknown IDs.
func TestGet(t *testing.T) {
	reg := newTestRegistry(t)

	p, err := reg.Get("sonarr")
	if err != nil || p == nil {
		t.Fatalf("want sonarr profile, got err=%v p=%v", err, p)
	}
	if p.Meta.Category != "Media" {
		t.Errorf("want category Media, got %q", p.Meta.Category)
	}

	missing, err := reg.Get("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if missing != nil {
		t.Errorf("want nil for unknown profile, got %+v", missing)
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

// TestExtractFields_UnknownProfile returns empty map for an unregistered profile.
func TestExtractFields_UnknownProfile(t *testing.T) {
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

// TestRenderDisplayText_UnknownProfile returns default text for unknown profile.
func TestRenderDisplayText_UnknownProfile(t *testing.T) {
	reg := newTestRegistry(t)
	got := reg.RenderDisplayText("ghost", map[string]string{})
	if got != "Event received" {
		t.Errorf("want %q, got %q", "Event received", got)
	}
}

// TestRenderDisplayText_NoTemplate returns default for profile without template.
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

// TestMapSeverity_UnknownProfile returns "info" for an unregistered profile.
func TestMapSeverity_UnknownProfile(t *testing.T) {
	reg := newTestRegistry(t)
	got := reg.MapSeverity("ghost", map[string]string{"event_type": "Anything"})
	if got != "info" {
		t.Errorf("want info, got %q", got)
	}
}

// TestMapSeverity_NoSeverityConfig returns "info" when profile has no severity config.
func TestMapSeverity_NoSeverityConfig(t *testing.T) {
	reg := newTestRegistry(t)
	got := reg.MapSeverity("simple", map[string]string{"event_type": "Anything"})
	if got != "info" {
		t.Errorf("want info, got %q", got)
	}
}
