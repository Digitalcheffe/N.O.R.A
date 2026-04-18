package apptemplate

import "fmt"

var validValueTypes = map[string]bool{
	"count":   true,
	"string":  true,
	"boolean": true,
	"list":    true,
}

// validAuthTypes lists the auth schemes the API poller understands. Profiles
// and apps may omit auth_type entirely; only reject explicit bad values.
var validAuthTypes = map[string]bool{
	"":              true,
	"none":          true,
	"apikey_header": true,
	"apikey_query":  true,
	"bearer":        true,
	"basic":         true,
}

// validate checks the structural integrity of a loaded AppTemplate.
// Returns a descriptive error if any constraint is violated.
// Called at load time; callers should log the error and skip the profile rather than crashing.
func validate(id string, t *AppTemplate) error {
	if !validAuthTypes[t.APIPolling.AuthType] {
		return fmt.Errorf("%s: api_polling.auth_type %q must be one of apikey_header, apikey_query, bearer, basic, none", id, t.APIPolling.AuthType)
	}
	if t.APIPolling.AuthType == "apikey_header" || t.APIPolling.AuthType == "apikey_query" {
		if t.APIPolling.AuthHeader == "" {
			return fmt.Errorf("%s: api_polling.auth_header is required when auth_type=%s", id, t.APIPolling.AuthType)
		}
	}

	// Build api_polling name index and validate each entry.
	pollingNames := make(map[string]struct{}, len(t.APIPolling.Endpoints))
	for i, p := range t.APIPolling.Endpoints {
		if p.Path == "" {
			return fmt.Errorf("%s: api_polling[%d]: path is required", id, i)
		}
		if p.Name == "" {
			return fmt.Errorf("%s: api_polling[%d]: name is required", id, i)
		}
		if p.Label == "" {
			return fmt.Errorf("%s: api_polling[%d]: name=%q: label is required", id, i, p.Name)
		}
		if p.Target == "" {
			return fmt.Errorf("%s: api_polling[%d]: name=%q: target is required", id, i, p.Name)
		}
		if p.ValueType == "" {
			return fmt.Errorf("%s: api_polling[%d]: name=%q: value_type is required", id, i, p.Name)
		}
		if !validValueTypes[p.ValueType] {
			return fmt.Errorf("%s: api_polling[%d]: name=%q: value_type %q must be one of count, string, boolean, list", id, i, p.Name, p.ValueType)
		}
		if _, dup := pollingNames[p.Name]; dup {
			return fmt.Errorf("%s: api_polling: duplicate name %q", id, p.Name)
		}
		pollingNames[p.Name] = struct{}{}
	}

	// Validate digest.categories — source is optional for backward compatibility,
	// but if present it must be a known value.
	for i, c := range t.Digest.Categories {
		if c.Source != "" && c.Source != "webhook" && c.Source != "api" {
			return fmt.Errorf("%s: digest.categories[%d] %q: source must be webhook or api, got %q", id, i, c.Label, c.Source)
		}
	}

	// Validate digest.widgets.
	for i, w := range t.Digest.Widgets {
		if w.Source == "" {
			return fmt.Errorf("%s: digest.widgets[%d] %q: source is required", id, i, w.Label)
		}
		if w.Source != "api" && w.Source != "webhook" {
			return fmt.Errorf("%s: digest.widgets[%d] %q: source must be api or webhook, got %q", id, i, w.Label, w.Source)
		}
		if w.Source == "api" {
			if w.Metric == "" {
				return fmt.Errorf("%s: digest.widgets[%d] %q: metric is required when source=api", id, i, w.Label)
			}
			if _, ok := pollingNames[w.Metric]; !ok {
				return fmt.Errorf("%s: digest.widgets[%d] %q: metric %q not found in api_polling", id, i, w.Label, w.Metric)
			}
		}
	}

	return nil
}
