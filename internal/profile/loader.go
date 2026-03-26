package profile

// Loader retrieves app profiles by ID.
type Loader interface {
	Get(profileID string) (*Profile, error)
}

// Profile describes how to process a webhook payload for a specific app.
type Profile struct {
	// FieldMappings maps a tag name to a JSONPath expression (e.g. "$.eventType").
	FieldMappings map[string]string
	// SeverityMapping maps an extracted field value to a severity level.
	SeverityMapping map[string]string
	// DisplayTemplate is a template string with {field_name} tokens.
	DisplayTemplate string
	// SeverityField is the tag name whose extracted value drives severity mapping.
	SeverityField string
}

// NoopLoader returns nil for all profiles (passthrough mode).
// Used until T-09 (profile loader) is merged.
type NoopLoader struct{}

func (n *NoopLoader) Get(_ string) (*Profile, error) { return nil, nil }
