// Package apipoller defines per-app-type API authentication configuration.
// Auth details live here — not in the app profile YAML — so users curating
// custom profiles don't need to know implementation details.
package apipoller

import (
	"embed"
	"io/fs"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed *.yaml
var files embed.FS

// AuthConfig describes how to attach the app's api_key to outgoing poll requests.
type AuthConfig struct {
	// AuthType is one of: apikey_header | apikey_query | bearer | none.
	AuthType string `yaml:"auth_type"`
	// AuthHeader is the header name (apikey_header) or query param name (apikey_query).
	AuthHeader string `yaml:"auth_header"`
}

// configs is the in-process registry populated by init().
var configs map[string]*AuthConfig

func init() {
	configs = make(map[string]*AuthConfig)
	entries, _ := fs.ReadDir(files, ".")
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := files.ReadFile(e.Name())
		if err != nil {
			continue
		}
		var cfg AuthConfig
		if err := yaml.Unmarshal(data, &cfg); err != nil || cfg.AuthType == "" {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".yaml")
		configs[id] = &cfg
	}
}

// Get returns the AuthConfig for profileID, or nil if none is registered.
func Get(profileID string) *AuthConfig {
	return configs[profileID]
}

// ResolveAPIBaseURL returns the URL the API poller should use for a given app
// config. It prefers api_url (an explicit internal/API-only address) and falls
// back to base_url. Returns an empty string if neither is set.
func ResolveAPIBaseURL(cfg map[string]interface{}) string {
	if v, _ := cfg["api_url"].(string); v != "" {
		return v
	}
	v, _ := cfg["base_url"].(string)
	return v
}
