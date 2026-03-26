package profile

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

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

// NoopLoader returns nil for all profiles (passthrough mode).
// Used in tests and when profile loading is not required.
type NoopLoader struct{}

func (n *NoopLoader) Get(_ string) (*Profile, error) { return nil, nil }
