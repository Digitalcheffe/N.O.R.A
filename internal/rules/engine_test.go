package rules

import (
	"testing"

	"github.com/digitalcheffe/nora/internal/models"
)

// ── evaluateConditions ────────────────────────────────────────────────────────

func TestEvaluateConditions_ANDAllMatch(t *testing.T) {
	event := models.Event{Title: "download failed", Level: "error"}
	conditions := []models.RuleCondition{
		{Field: "display_text", Operator: "contains", Value: "failed"},
		{Field: "severity", Operator: "is", Value: "error"},
	}
	if !evaluateConditions(conditions, "AND", event) {
		t.Fatal("expected AND match when all conditions pass")
	}
}

func TestEvaluateConditions_ANDOneFails(t *testing.T) {
	event := models.Event{Title: "download failed", Level: "warn"}
	conditions := []models.RuleCondition{
		{Field: "display_text", Operator: "contains", Value: "failed"},
		{Field: "severity", Operator: "is", Value: "error"},
	}
	if evaluateConditions(conditions, "AND", event) {
		t.Fatal("expected AND no-match when one condition fails")
	}
}

func TestEvaluateConditions_OROneMatches(t *testing.T) {
	event := models.Event{Title: "download ok", Level: "error"}
	conditions := []models.RuleCondition{
		{Field: "display_text", Operator: "contains", Value: "failed"},
		{Field: "severity", Operator: "is", Value: "error"},
	}
	if !evaluateConditions(conditions, "OR", event) {
		t.Fatal("expected OR match when at least one condition passes")
	}
}

func TestEvaluateConditions_ORNoneMatch(t *testing.T) {
	event := models.Event{Title: "download ok", Level: "info"}
	conditions := []models.RuleCondition{
		{Field: "display_text", Operator: "contains", Value: "failed"},
		{Field: "severity", Operator: "is", Value: "error"},
	}
	if evaluateConditions(conditions, "OR", event) {
		t.Fatal("expected OR no-match when no condition passes")
	}
}

func TestEvaluateConditions_EmptyConditionsAlwaysMatch(t *testing.T) {
	event := models.Event{Title: "anything", Level: "debug"}
	if !evaluateConditions(nil, "AND", event) {
		t.Fatal("empty conditions should always match (AND)")
	}
	if !evaluateConditions(nil, "OR", event) {
		t.Fatal("empty conditions should always match (OR)")
	}
}

// ── evaluateCondition operators ───────────────────────────────────────────────

func TestEvaluateCondition_Is_CaseInsensitive(t *testing.T) {
	event := models.Event{Level: "ERROR"}
	c := models.RuleCondition{Field: "severity", Operator: "is", Value: "error"}
	if !evaluateCondition(c, event) {
		t.Fatal("is operator should be case-insensitive")
	}
}

func TestEvaluateCondition_IsNot(t *testing.T) {
	event := models.Event{Level: "warn"}
	c := models.RuleCondition{Field: "severity", Operator: "is_not", Value: "error"}
	if !evaluateCondition(c, event) {
		t.Fatal("is_not operator failed")
	}
}

func TestEvaluateCondition_Contains(t *testing.T) {
	event := models.Event{Title: "Sonarr: Download FAILED for Movie X"}
	c := models.RuleCondition{Field: "display_text", Operator: "contains", Value: "failed"}
	if !evaluateCondition(c, event) {
		t.Fatal("contains operator should be case-insensitive")
	}
}

func TestEvaluateCondition_DoesNotContain(t *testing.T) {
	event := models.Event{Title: "Sonarr: Download completed"}
	c := models.RuleCondition{Field: "display_text", Operator: "does_not_contain", Value: "failed"}
	if !evaluateCondition(c, event) {
		t.Fatal("does_not_contain operator failed for non-matching text")
	}
}

// ── source gate ───────────────────────────────────────────────────────────────

func TestPassesGate_SourceIDMismatch(t *testing.T) {
	e := &Engine{}
	sourceID := "app-abc"
	rule := models.Rule{SourceID: &sourceID}
	event := models.Event{SourceID: "app-xyz"}
	if e.passesGate(rule, event) {
		t.Fatal("rule with specific source_id should not match a different source_id")
	}
}

func TestPassesGate_SourceIDMatch(t *testing.T) {
	e := &Engine{}
	sourceID := "app-abc"
	rule := models.Rule{SourceID: &sourceID}
	event := models.Event{SourceID: "app-abc"}
	if !e.passesGate(rule, event) {
		t.Fatal("rule with matching source_id should pass gate")
	}
}

func TestPassesGate_SourceTypeDocker(t *testing.T) {
	e := &Engine{}
	st := "docker"
	rule := models.Rule{SourceType: &st}
	if !e.passesGate(rule, models.Event{SourceType: "docker_engine"}) {
		t.Fatal("docker rule should match docker_engine events")
	}
	if e.passesGate(rule, models.Event{SourceType: "app"}) {
		t.Fatal("docker rule should not match app events")
	}
}

func TestPassesGate_SourceTypeMonitor(t *testing.T) {
	e := &Engine{}
	st := "monitor"
	rule := models.Rule{SourceType: &st}
	if !e.passesGate(rule, models.Event{SourceType: "monitor_check"}) {
		t.Fatal("monitor rule should match monitor_check events")
	}
	if e.passesGate(rule, models.Event{SourceType: "app"}) {
		t.Fatal("monitor rule should not match app events")
	}
}

// ── severity gate ─────────────────────────────────────────────────────────────

func TestPassesGate_SeverityMismatch(t *testing.T) {
	e := &Engine{}
	sev := "critical"
	rule := models.Rule{Severity: &sev}
	event := models.Event{Level: "error"}
	if e.passesGate(rule, event) {
		t.Fatal("rule with specific severity should not match a different level")
	}
}

func TestPassesGate_SeverityMatch(t *testing.T) {
	e := &Engine{}
	sev := "error"
	rule := models.Rule{Severity: &sev}
	event := models.Event{Level: "error"}
	if !e.passesGate(rule, event) {
		t.Fatal("rule with matching severity should pass gate")
	}
}

func TestPassesGate_NilGatesAlwaysPass(t *testing.T) {
	e := &Engine{}
	rule := models.Rule{} // all nil
	event := models.Event{SourceID: "any", SourceType: "app", Level: "debug"}
	if !e.passesGate(rule, event) {
		t.Fatal("rule with no gates should pass for any event")
	}
}

// ── template rendering ────────────────────────────────────────────────────────

func TestRenderTemplate_BuiltinTokens(t *testing.T) {
	event := models.Event{
		Title:      "Download failed",
		Level:      "error",
		SourceName: "Sonarr",
	}
	got := renderTemplate("{source_name}: {display_text} [{severity}]", event)
	want := "Sonarr: Download failed [error]"
	if got != want {
		t.Fatalf("renderTemplate got %q, want %q", got, want)
	}
}

func TestRenderTemplate_PayloadToken(t *testing.T) {
	event := models.Event{
		Title:   "Series grabbed",
		Level:   "info",
		Payload: `{"series_title":"Breaking Bad"}`,
	}
	got := renderTemplate("Grabbed: {series_title}", event)
	want := "Grabbed: Breaking Bad"
	if got != want {
		t.Fatalf("renderTemplate got %q, want %q", got, want)
	}
}

func TestRenderTemplate_NoMatch_LeftAsIs(t *testing.T) {
	event := models.Event{Title: "test"}
	got := renderTemplate("{unknown_token}", event)
	want := "{unknown_token}"
	if got != want {
		t.Fatalf("unmatched token should be left as-is, got %q", got)
	}
}

// ── resolveField ──────────────────────────────────────────────────────────────

func TestResolveField_PayloadKey(t *testing.T) {
	event := models.Event{Payload: `{"event_type":"download_failed"}`}
	got := resolveField("event_type", event)
	if got != "download_failed" {
		t.Fatalf("resolveField got %q, want %q", got, "download_failed")
	}
}

func TestResolveField_MissingPayloadKey(t *testing.T) {
	event := models.Event{Payload: `{"other_key":"value"}`}
	got := resolveField("nonexistent", event)
	if got != "" {
		t.Fatalf("missing field should return empty string, got %q", got)
	}
}

func TestResolveField_NoMatch_NoDelivery(t *testing.T) {
	// Verify that a rule with conditions that never match does not fire.
	event := models.Event{Title: "all is well", Level: "info"}
	conditions := []models.RuleCondition{
		{Field: "display_text", Operator: "contains", Value: "failed"},
	}
	if evaluateConditions(conditions, "AND", event) {
		t.Fatal("rule should not match when condition does not pass")
	}
}
