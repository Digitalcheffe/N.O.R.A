package apptemplate

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

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

// EventTypeKeyRule synthesizes an event_type field from payload structure.
// Used for apps (like Ghost) that do not include the event type in the payload body.
// Rules are evaluated in order; the first matching top-level key wins.
type EventTypeKeyRule struct {
	// Key is the top-level JSON key whose presence triggers this rule (e.g. "post").
	Key string `yaml:"key"`
	// StatusPath is a JSONPath to extract a status value (e.g. "$.post.current.status").
	// When set, event_type is synthesized as Prefix + status_value.
	// If the path resolves to empty, falls through to Default.
	StatusPath string `yaml:"status_path"`
	// Prefix is prepended to the StatusPath value (e.g. "post.").
	Prefix string `yaml:"prefix"`
	// PresentPath is a JSONPath checked for presence.
	// If non-empty → IfPresent is used; if empty/missing → IfAbsent.
	PresentPath string `yaml:"present_path"`
	// IfPresent is the event_type when PresentPath resolves to a non-empty value.
	IfPresent string `yaml:"if_present"`
	// IfAbsent is the event_type when PresentPath is empty or missing.
	IfAbsent string `yaml:"if_absent"`
	// Default is used when StatusPath yields an empty value and no PresentPath is set.
	Default string `yaml:"default"`
}

// Webhook holds ingest processing configuration for the template.
type Webhook struct {
	SetupInstructions     string             `yaml:"setup_instructions"`
	RecommendedEvents     []string           `yaml:"recommended_events"`
	NotRecommended        []string           `yaml:"not_recommended"`
	FieldMappings         map[string]string  `yaml:"field_mappings"`
	EventTypeKeys         []EventTypeKeyRule `yaml:"event_type_keys"`
	DisplayTemplate       string             `yaml:"display_template"`
	DisplayTemplates      map[string]string  `yaml:"display_templates"`
	SeverityField         string             `yaml:"severity_field"`
	SeverityCompoundField string             `yaml:"severity_compound_field"`
	SeverityMapping       map[string]string  `yaml:"severity_mapping"`
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
	mu         sync.RWMutex
	templates  map[string]*AppTemplate
	builtinDir string
	customDir  string
}

// NewRegistry loads all *.yaml files from fsys and returns a populated Registry.
// Each template is keyed by its filename without extension (e.g. "sonarr").
// Used by tests and legacy callers; does not support disk-based Reload.
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

// ExportBuiltins writes each embedded YAML from fsys into destDir.
// Existing files are skipped so user edits are preserved across restarts.
// New files added to the embedded set are written on the first startup after an upgrade.
func ExportBuiltins(fsys fs.FS, destDir string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("create builtin dir: %w", err)
	}

	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return fmt.Errorf("read embedded templates: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		dest := filepath.Join(destDir, e.Name())
		if _, err := os.Stat(dest); err == nil {
			continue // already exists — preserve any user edits
		}
		data, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", e.Name(), err)
		}
		if err := os.WriteFile(dest, data, 0644); err != nil {
			return fmt.Errorf("write %s: %w", dest, err)
		}
	}
	return nil
}

// NewRegistryFromDisk loads templates from builtinDir and customDir on disk.
// Custom templates override builtins when they share the same ID (filename stem).
func NewRegistryFromDisk(builtinDir, customDir string) (*Registry, error) {
	reg := &Registry{
		templates:  make(map[string]*AppTemplate),
		builtinDir: builtinDir,
		customDir:  customDir,
	}
	if err := reg.reload(); err != nil {
		return nil, err
	}
	return reg, nil
}

// Reload re-reads all templates from the builtin and custom directories.
// Safe to call concurrently — acquires a write lock for the swap.
func (r *Registry) Reload() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reload()
}

// reload is the internal (unlocked) implementation shared by NewRegistryFromDisk and Reload.
func (r *Registry) reload() error {
	newTemplates := make(map[string]*AppTemplate)

	if err := loadDirIntoMap(r.builtinDir, newTemplates); err != nil {
		return fmt.Errorf("load builtin templates: %w", err)
	}
	// Custom templates override builtins with matching IDs.
	if err := loadDirIntoMap(r.customDir, newTemplates); err != nil {
		return fmt.Errorf("load custom templates: %w", err)
	}

	r.templates = newTemplates
	return nil
}

// loadDirIntoMap reads all *.yaml files from dir and merges them into m.
// Missing or non-existent dirs are treated as empty (no error).
func loadDirIntoMap(dir string, m map[string]*AppTemplate) error {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read dir %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return fmt.Errorf("read %s: %w", e.Name(), err)
		}
		var t AppTemplate
		if err := yaml.Unmarshal(data, &t); err != nil {
			return fmt.Errorf("parse %s: %w", e.Name(), err)
		}
		id := strings.TrimSuffix(e.Name(), ".yaml")
		m[id] = &t
	}
	return nil
}

// Get returns the app template for templateID, or nil if no template is registered.
// A nil template is valid — it means passthrough (no field extraction or mapping).
func (r *Registry) Get(templateID string) (*AppTemplate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if t, ok := r.templates[templateID]; ok {
		return t, nil
	}
	return nil, nil
}

// List returns all registered app templates keyed by ID.
func (r *Registry) List() map[string]*AppTemplate {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]*AppTemplate, len(r.templates))
	for k, v := range r.templates {
		out[k] = v
	}
	return out
}

// InferEventTypeFromKeys inspects a decoded JSON payload for the first matching
// EventTypeKeyRule and returns a synthesized event_type string.
// Returns "" if no rule matches.
func InferEventTypeFromKeys(payload interface{}, keys []EventTypeKeyRule) string {
	obj, ok := payload.(map[string]interface{})
	if !ok {
		return ""
	}
	for _, rule := range keys {
		if _, ok := obj[rule.Key]; !ok {
			continue
		}
		// Top-level key found — apply rule.
		if rule.StatusPath != "" {
			if v, ok := jsonPathGet(payload, rule.StatusPath); ok && v != "" {
				return rule.Prefix + v
			}
			if rule.Default != "" {
				return rule.Default
			}
		}
		if rule.PresentPath != "" {
			v, _ := jsonPathGet(payload, rule.PresentPath)
			if v != "" {
				return rule.IfPresent
			}
			return rule.IfAbsent
		}
		if rule.Default != "" {
			return rule.Default
		}
	}
	return ""
}

// ExtractFields evaluates each JSONPath in the template's field_mappings against payload
// and returns a flat tag→value map. Returns an error only on JSON decode failure.
// If the template has event_type_keys configured and event_type is not already extracted,
// it synthesizes event_type from the payload structure.
func (r *Registry) ExtractFields(templateID string, payload []byte) (map[string]string, error) {
	r.mu.RLock()
	t, ok := r.templates[templateID]
	r.mu.RUnlock()
	if !ok {
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

	// Synthesize event_type from payload structure when not already extracted.
	if _, hasET := out["event_type"]; !hasET && len(t.Webhook.EventTypeKeys) > 0 {
		if et := InferEventTypeFromKeys(root, t.Webhook.EventTypeKeys); et != "" {
			out["event_type"] = et
		}
	}

	return out, nil
}

// RenderDisplayText substitutes {field_name} tokens in the best matching display template.
// Checks display_templates[event_type] first, then falls back to display_template.
// Returns "Event received" when no template is configured.
func (r *Registry) RenderDisplayText(templateID string, fields map[string]string) string {
	r.mu.RLock()
	t, ok := r.templates[templateID]
	r.mu.RUnlock()
	if !ok {
		return "Event received"
	}

	// Pick per-eventType template if available, fall back to global template.
	tmpl := t.Webhook.DisplayTemplate
	if len(t.Webhook.DisplayTemplates) > 0 {
		if et, ok := fields["event_type"]; ok {
			if specific, ok := t.Webhook.DisplayTemplates[et]; ok {
				tmpl = specific
			}
		}
	}

	if tmpl == "" {
		return "Event received"
	}
	result := tmpl
	for k, v := range fields {
		result = strings.ReplaceAll(result, "{"+k+"}", v)
	}
	return result
}

// MapSeverity looks up the value of the template's severity_field in fields against
// severity_mapping. When severity_compound_field is set, tries a compound key
// "{primary}:{compound}" first, then falls back to the primary key alone.
// Returns "info" for unknown values or missing configuration.
func (r *Registry) MapSeverity(templateID string, fields map[string]string) string {
	r.mu.RLock()
	t, ok := r.templates[templateID]
	r.mu.RUnlock()
	if !ok || t.Webhook.SeverityField == "" || len(t.Webhook.SeverityMapping) == 0 {
		return "info"
	}
	val, ok := fields[t.Webhook.SeverityField]
	if !ok {
		return "info"
	}
	// Try compound key: "primary:compound" (e.g. "Health:error")
	if t.Webhook.SeverityCompoundField != "" {
		if compound := fields[t.Webhook.SeverityCompoundField]; compound != "" {
			if s, ok := t.Webhook.SeverityMapping[val+":"+compound]; ok {
				return s
			}
		}
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
