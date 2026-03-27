package apptemplate

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

// Loader retrieves app templates by ID.
type Loader interface {
	Get(templateID string) (*AppTemplate, error)
}

// AppTemplateMeta holds the template identification and classification fields.
type AppTemplateMeta struct {
	Name        string `yaml:"name"`
	Category    string `yaml:"category"`
	Logo        string `yaml:"logo"`
	Description string `yaml:"description"`
	Capability  string `yaml:"capability"`
}

// Webhook holds ingest processing configuration for the template.
type Webhook struct {
	SetupInstructions string            `yaml:"setup_instructions"`
	RecommendedEvents []string          `yaml:"recommended_events"`
	NotRecommended    []string          `yaml:"not_recommended"`
	FieldMappings     map[string]string `yaml:"field_mappings"`
	DisplayTemplate   string            `yaml:"display_template"`
	SeverityField     string            `yaml:"severity_field"`
	SeverityMapping   map[string]string `yaml:"severity_mapping"`
}

// Monitor holds active check configuration for the template.
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

// AppTemplate describes how to process webhooks and render dashboard data for a specific app.
type AppTemplate struct {
	Meta    AppTemplateMeta `yaml:"meta"`
	Webhook Webhook         `yaml:"webhook"`
	Monitor Monitor         `yaml:"monitor"`
	Digest  Digest          `yaml:"digest"`
}

// Registry loads all bundled YAML app templates from an embedded filesystem.
type Registry struct {
	templates map[string]*AppTemplate
}

// NewRegistry loads all *.yaml files from fsys and returns a populated Registry.
// Each template is keyed by its filename without extension (e.g. "sonarr").
func NewRegistry(fsys fs.FS) (*Registry, error) {
	reg := &Registry{templates: make(map[string]*AppTemplate)}

	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read app template dir: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}

		data, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, fmt.Errorf("read app template %s: %w", e.Name(), err)
		}

		var t AppTemplate
		if err := yaml.Unmarshal(data, &t); err != nil {
			return nil, fmt.Errorf("parse app template %s: %w", e.Name(), err)
		}

		id := strings.TrimSuffix(e.Name(), ".yaml")
		reg.templates[id] = &t
	}

	return reg, nil
}

// Get returns the app template for templateID, or nil if no template is registered.
// A nil template is valid — it means passthrough (no field extraction or mapping).
func (r *Registry) Get(templateID string) (*AppTemplate, error) {
	if t, ok := r.templates[templateID]; ok {
		return t, nil
	}
	return nil, nil
}

// List returns all registered app templates keyed by ID.
func (r *Registry) List() map[string]*AppTemplate {
	out := make(map[string]*AppTemplate, len(r.templates))
	for k, v := range r.templates {
		out[k] = v
	}
	return out
}

// ExtractFields evaluates each JSONPath in the template's field_mappings against payload
// and returns a flat tag→value map. Returns an error only on JSON decode failure.
func (r *Registry) ExtractFields(templateID string, payload []byte) (map[string]string, error) {
	t, ok := r.templates[templateID]
	if !ok || len(t.Webhook.FieldMappings) == 0 {
		return map[string]string{}, nil
	}

	var root interface{}
	if err := json.Unmarshal(payload, &root); err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}

	out := make(map[string]string, len(t.Webhook.FieldMappings))
	for tag, path := range t.Webhook.FieldMappings {
		if v, ok := jsonPathGet(root, path); ok {
			out[tag] = v
		}
	}
	return out, nil
}

// RenderDisplayText substitutes {field_name} tokens in the template's display_template
// with values from fields. Returns "Event received" when the template is empty.
func (r *Registry) RenderDisplayText(templateID string, fields map[string]string) string {
	t, ok := r.templates[templateID]
	if !ok || t.Webhook.DisplayTemplate == "" {
		return "Event received"
	}
	result := t.Webhook.DisplayTemplate
	for k, v := range fields {
		result = strings.ReplaceAll(result, "{"+k+"}", v)
	}
	return result
}

// MapSeverity looks up the value of the template's severity_field in fields against
// severity_mapping. Returns "info" for unknown values or missing configuration.
func (r *Registry) MapSeverity(templateID string, fields map[string]string) string {
	t, ok := r.templates[templateID]
	if !ok || t.Webhook.SeverityField == "" || len(t.Webhook.SeverityMapping) == 0 {
		return "info"
	}
	val, ok := fields[t.Webhook.SeverityField]
	if !ok {
		return "info"
	}
	if s, ok := t.Webhook.SeverityMapping[val]; ok {
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

// NoopLoader returns nil for all templates (passthrough mode).
// Used in tests and when template loading is not required.
type NoopLoader struct{}

func (n *NoopLoader) Get(_ string) (*AppTemplate, error) { return nil, nil }
