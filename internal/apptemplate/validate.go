package apptemplate

import "fmt"

var validValueTypes = map[string]bool{
	"count":   true,
	"string":  true,
	"boolean": true,
	"list":    true,
}

var validAuthTypes = map[string]bool{
	"":             true, // none / omitted
	"none":         true,
	"apikey_header": true,
	"apikey_query":  true,
	"bearer":        true,
}

// validate checks the structural integrity of a loaded AppTemplate.
// Returns a descriptive error if any constraint is violated.
// Called at load time; callers should log the error and skip the profile rather than crashing.
func validate(id string, t *AppTemplate) error {
	// Build api_polling name index and validate each entry.
	pollingNames := make(map[string]struct{}, len(t.APIPolling))
	for i, p := range t.APIPolling {
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
		if !validAuthTypes[p.AuthType] {
			return fmt.Errorf("%s: api_polling[%d]: name=%q: auth_type %q must be one of apikey_header, apikey_query, bearer, none", id, i, p.Name, p.AuthType)
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
