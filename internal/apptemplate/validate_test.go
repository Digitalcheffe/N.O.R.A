package apptemplate

import (
	"testing"
)

func TestValidate_ValidEmpty(t *testing.T) {
	tmpl := &AppTemplate{}
	if err := validate("empty", tmpl); err != nil {
		t.Fatalf("expected no error for empty template, got: %v", err)
	}
}

func TestValidate_ValidAPIPolling(t *testing.T) {
	tmpl := &AppTemplate{
		APIPolling: APIPollingBlock{
			AuthType:   "apikey_header",
			AuthHeader: "X-Api-Key",
			Endpoints: []APIPollingEntry{
				{Path: "/api/v3/series", Name: "total_series", Label: "Total Series", Target: "length", ValueType: "count"},
				{Path: "/api/v3/queue", Name: "queue_depth", Label: "Queue Depth", Target: "$.totalRecords", ValueType: "count", EventMessage: "Queue has {value} items"},
			},
		},
		Digest: Digest{
			Widgets: []DigestWidget{
				{Source: "api", Label: "Series Tracked", Metric: "total_series"},
				{Source: "api", Label: "In Queue", Metric: "queue_depth"},
				{Source: "webhook", Label: "Downloads Today", MatchField: "event_type", MatchValue: "Download"},
			},
		},
	}
	if err := validate("sonarr", tmpl); err != nil {
		t.Fatalf("expected no error for valid template, got: %v", err)
	}
}

func TestValidate_MissingAPIPollingPath(t *testing.T) {
	tmpl := &AppTemplate{
		APIPolling: APIPollingBlock{
			Endpoints: []APIPollingEntry{
				{Name: "total_series", Label: "Total Series", Target: "length", ValueType: "count"},
			},
		},
	}
	if err := validate("sonarr", tmpl); err == nil {
		t.Fatal("expected error for missing path, got nil")
	}
}

func TestValidate_MissingAPIPollingName(t *testing.T) {
	tmpl := &AppTemplate{
		APIPolling: APIPollingBlock{
			Endpoints: []APIPollingEntry{
				{Path: "/api/v3/series", Label: "Total Series", Target: "length", ValueType: "count"},
			},
		},
	}
	if err := validate("sonarr", tmpl); err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
}

func TestValidate_InvalidValueType(t *testing.T) {
	tmpl := &AppTemplate{
		APIPolling: APIPollingBlock{
			Endpoints: []APIPollingEntry{
				{Path: "/api/v3/series", Name: "total_series", Label: "Total Series", Target: "length", ValueType: "number"},
			},
		},
	}
	if err := validate("sonarr", tmpl); err == nil {
		t.Fatal("expected error for invalid value_type, got nil")
	}
}

func TestValidate_DuplicateAPIPollingName(t *testing.T) {
	tmpl := &AppTemplate{
		APIPolling: APIPollingBlock{
			Endpoints: []APIPollingEntry{
				{Path: "/api/v3/series", Name: "total_series", Label: "Total Series", Target: "length", ValueType: "count"},
				{Path: "/api/v3/movies", Name: "total_series", Label: "Total Movies", Target: "length", ValueType: "count"},
			},
		},
	}
	if err := validate("sonarr", tmpl); err == nil {
		t.Fatal("expected error for duplicate name, got nil")
	}
}

func TestValidate_WidgetMissingSource(t *testing.T) {
	tmpl := &AppTemplate{
		Digest: Digest{
			Widgets: []DigestWidget{
				{Label: "Downloads Today", MatchField: "event_type", MatchValue: "Download"},
			},
		},
	}
	if err := validate("sonarr", tmpl); err == nil {
		t.Fatal("expected error for widget missing source, got nil")
	}
}

func TestValidate_WidgetInvalidSource(t *testing.T) {
	tmpl := &AppTemplate{
		Digest: Digest{
			Widgets: []DigestWidget{
				{Source: "mqtt", Label: "Downloads Today"},
			},
		},
	}
	if err := validate("sonarr", tmpl); err == nil {
		t.Fatal("expected error for widget invalid source, got nil")
	}
}

func TestValidate_WidgetAPISourceMissingMetric(t *testing.T) {
	tmpl := &AppTemplate{
		APIPolling: APIPollingBlock{
			Endpoints: []APIPollingEntry{
				{Path: "/api/v3/series", Name: "total_series", Label: "Total Series", Target: "length", ValueType: "count"},
			},
		},
		Digest: Digest{
			Widgets: []DigestWidget{
				{Source: "api", Label: "Series Tracked"},
			},
		},
	}
	if err := validate("sonarr", tmpl); err == nil {
		t.Fatal("expected error for api widget missing metric, got nil")
	}
}

func TestValidate_WidgetAPISourceUnknownMetric(t *testing.T) {
	tmpl := &AppTemplate{
		APIPolling: APIPollingBlock{
			Endpoints: []APIPollingEntry{
				{Path: "/api/v3/series", Name: "total_series", Label: "Total Series", Target: "length", ValueType: "count"},
			},
		},
		Digest: Digest{
			Widgets: []DigestWidget{
				{Source: "api", Label: "In Queue", Metric: "queue_depth"},
			},
		},
	}
	if err := validate("sonarr", tmpl); err == nil {
		t.Fatal("expected error for widget referencing unknown metric, got nil")
	}
}

func TestValidate_CategoryInvalidSource(t *testing.T) {
	tmpl := &AppTemplate{
		Digest: Digest{
			Categories: []DigestCategory{
				{Source: "mqtt", Label: "Downloads", MatchField: "event_type", MatchValue: "Download"},
			},
		},
	}
	if err := validate("sonarr", tmpl); err == nil {
		t.Fatal("expected error for category invalid source, got nil")
	}
}

func TestValidate_CategoryEmptySourceAllowed(t *testing.T) {
	// Existing profiles omit source — must remain valid.
	tmpl := &AppTemplate{
		Digest: Digest{
			Categories: []DigestCategory{
				{Label: "Downloads", MatchField: "event_type", MatchValue: "Download"},
			},
		},
	}
	if err := validate("sonarr", tmpl); err != nil {
		t.Fatalf("expected no error for category with empty source (backward compat), got: %v", err)
	}
}

func TestValidate_InvalidAuthType(t *testing.T) {
	tmpl := &AppTemplate{
		APIPolling: APIPollingBlock{AuthType: "magic-tokens"},
	}
	if err := validate("custom", tmpl); err == nil {
		t.Fatal("expected error for unknown auth_type, got nil")
	}
}

func TestValidate_ApiKeyHeaderRequiresHeaderName(t *testing.T) {
	tmpl := &AppTemplate{
		APIPolling: APIPollingBlock{AuthType: "apikey_header"},
	}
	if err := validate("custom", tmpl); err == nil {
		t.Fatal("expected error when auth_header missing for apikey_header, got nil")
	}
}
