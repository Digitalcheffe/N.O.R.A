package profile

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var arrayIndexRe = regexp.MustCompile(`^([^\[]+)\[(\d+)\]$`)

// Loader retrieves app profiles by ID.
type Loader interface {
	Get(profileID string) (*Profile, error)
}

// Meta holds the profile identification and classification fields.
type Meta struct {
	Name        string `yaml:"name"`
	Category    string `yaml:"category"`
	Logo        string `yaml:"logo"`
	Description string `yaml:"description"`
	Capability  string `yaml:"capability"`
}

// Webhook holds ingest processing configuration for the profile.
type Webhook struct {
	SetupInstructions string            `yaml:"setup_instructions"`
	RecommendedEvents []string          `yaml:"recommended_events"`
	NotRecommended    []string          `yaml:"not_recommended"`
	FieldMappings     map[string]string `yaml:"field_mappings"`
	DisplayTemplate   string            `yaml:"display_template"`
	SeverityField     string            `yaml:"severity_field"`
	SeverityMapping   map[string]string `yaml:"severity_mapping"`
}

// Monitor holds active check configuration for the profile.
type Monitor struct {
	CheckType     string `yaml:"check_type"`
	CheckURL      string `yaml:"check_url"`
	AuthHeader    string `yaml:"auth_header"`
	HealthyStatus int    `yaml:"healthy_status"`
	CheckInterval string `yaml:"check_interval"`
}

// DigestCategory defines a named event category used in the dashboard summary bar.
// A category matches events where the given field equals the given value,
// and/or where severity equals MatchSeverity. Empty strings are ignored.
type DigestCategory struct {
	Label         string `yaml:"label"`
	MatchField    string `yaml:"match_field"`
	MatchValue    string `yaml:"match_value"`
	MatchSeverity string `yaml:"match_severity"`
}

// Digest holds digest category definitions for the dashboard.
type Digest struct {
	Categories []DigestCategory `yaml:"categories"`
}

// Profile describes how to process webhooks and render dashboard data for a specific app.
type Profile struct {
	Meta    Meta    `yaml:"meta"`
	Webhook Webhook `yaml:"webhook"`
	Monitor Monitor `yaml:"monitor"`
	Digest  Digest  `yaml:"digest"`
}

// Registry loads all bundled YAML profiles from an embedded filesystem.
type Registry struct {
	profiles map[string]*Profile
}

// NewRegistry loads all *.yaml files from fsys and returns a populated Registry.
// Each profile is keyed by its filename without extension (e.g. "sonarr").
func NewRegistry(fsys fs.FS) (*Registry, error) {
	reg := &Registry{profiles: make(map[string]*Profile)}

	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read profile dir: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}

		data, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, fmt.Errorf("read profile %s: %w", e.Name(), err)
		}

		var p Profile
		if err := yaml.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("parse profile %s: %w", e.Name(), err)
		}

		id := strings.TrimSuffix(e.Name(), ".yaml")
		reg.profiles[id] = &p
	}

	return reg, nil
}

// Get returns the profile for profileID, or nil if no profile is registered.
// A nil profile is valid — it means passthrough (no field extraction or mapping).
func (r *Registry) Get(profileID string) (*Profile, error) {
	if p, ok := r.profiles[profileID]; ok {
		return p, nil
	}
	return nil, nil
}

// List returns all registered profiles keyed by ID.
func (r *Registry) List() map[string]*Profile {
	out := make(map[string]*Profile, len(r.profiles))
	for k, v := range r.profiles {
		out[k] = v
	}
	return out
}

// ExtractFields evaluates each JSONPath in the profile's field_mappings against payload
// and returns a flat tag→value map. Returns an error only on JSON decode failure.
func (r *Registry) ExtractFields(profileID string, payload []byte) (map[string]string, error) {
	p, ok := r.profiles[profileID]
	if !ok || len(p.Webhook.FieldMappings) == 0 {
		return map[string]string{}, nil
	}

	var root interface{}
	if err := json.Unmarshal(payload, &root); err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}

	out := make(map[string]string, len(p.Webhook.FieldMappings))
	for tag, path := range p.Webhook.FieldMappings {
		if v, ok := jsonPathGet(root, path); ok {
			out[tag] = v
		}
	}
	return out, nil
}

// RenderDisplayText substitutes {field_name} tokens in the profile's display_template
// with values from fields. Returns "Event received" when the template is empty.
func (r *Registry) RenderDisplayText(profileID string, fields map[string]string) string {
	p, ok := r.profiles[profileID]
	if !ok || p.Webhook.DisplayTemplate == "" {
		return "Event received"
	}
	result := p.Webhook.DisplayTemplate
	for k, v := range fields {
		result = strings.ReplaceAll(result, "{"+k+"}", v)
	}
	return result
}

// MapSeverity looks up the value of the profile's severity_field in fields against
// severity_mapping. Returns "info" for unknown values or missing configuration.
func (r *Registry) MapSeverity(profileID string, fields map[string]string) string {
	p, ok := r.profiles[profileID]
	if !ok || p.Webhook.SeverityField == "" || len(p.Webhook.SeverityMapping) == 0 {
		return "info"
	}
	val, ok := fields[p.Webhook.SeverityField]
	if !ok {
		return "info"
	}
	if s, ok := p.Webhook.SeverityMapping[val]; ok {
		return s
	}
	return "info"
}

// jsonPathGet resolves a JSONPath expression against a decoded JSON value.
// Supports dot-notation ($.field.nested) and array indexing ($.arr[0].field).
func jsonPathGet(v interface{}, path string) (string, bool) {
	path = strings.TrimPrefix(path, "$.")
	path = strings.TrimPrefix(path, "$")
	if path == "" {
		return jsonToString(v), true
	}

	parts := strings.SplitN(path, ".", 2)
	segment := parts[0]
	rest := ""
	if len(parts) == 2 {
		rest = parts[1]
	}

	// Handle array index notation: episodes[0]
	if m := arrayIndexRe.FindStringSubmatch(segment); m != nil {
		key := m[1]
		idx, _ := strconv.Atoi(m[2])

		obj, ok := v.(map[string]interface{})
		if !ok {
			return "", false
		}
		arr, ok := obj[key].([]interface{})
		if !ok || idx >= len(arr) {
			return "", false
		}
		child := arr[idx]
		if rest == "" {
			return jsonToString(child), true
		}
		return jsonPathGet(child, rest)
	}

	obj, ok := v.(map[string]interface{})
	if !ok {
		return "", false
	}
	child, ok := obj[segment]
	if !ok {
		return "", false
	}
	if rest == "" {
		return jsonToString(child), true
	}
	return jsonPathGet(child, rest)
}

func jsonToString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// NoopLoader returns nil for all profiles (passthrough mode).
// Used in tests and when profile loading is not required.
type NoopLoader struct{}

func (n *NoopLoader) Get(_ string) (*Profile, error) { return nil, nil }
