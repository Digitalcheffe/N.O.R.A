package rules

import (
	"encoding/json"
	"strings"

	"github.com/digitalcheffe/nora/internal/models"
)

// evaluateConditions checks whether the event satisfies the rule's conditions.
// logic must be "AND" (all must match) or "OR" (any one must match).
// When conditions is empty the rule fires on gate matching alone.
func evaluateConditions(conditions []models.RuleCondition, logic string, event models.Event) bool {
	if len(conditions) == 0 {
		return true
	}
	for _, c := range conditions {
		matched := evaluateCondition(c, event)
		if logic == "OR" && matched {
			return true
		}
		if logic == "AND" && !matched {
			return false
		}
	}
	// AND: every condition passed. OR: no condition passed.
	return logic == "AND"
}

// evaluateCondition evaluates a single condition against an event.
func evaluateCondition(c models.RuleCondition, event models.Event) bool {
	fieldValue := resolveField(c.Field, event)
	switch c.Operator {
	case "is":
		return strings.EqualFold(fieldValue, c.Value)
	case "is_not":
		return !strings.EqualFold(fieldValue, c.Value)
	case "contains":
		return strings.Contains(strings.ToLower(fieldValue), strings.ToLower(c.Value))
	case "does_not_contain":
		return !strings.Contains(strings.ToLower(fieldValue), strings.ToLower(c.Value))
	}
	return false
}

// resolveField extracts a field value from an event.
// Built-in fields: display_text → Title, severity → Level, source_name → SourceName.
// Any other field name looks up in the event Payload JSON.
func resolveField(field string, event models.Event) string {
	switch field {
	case "display_text":
		return event.Title
	case "severity":
		return event.Level
	case "source_name":
		return event.SourceName
	}
	if event.Payload == "" {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(event.Payload), &m); err != nil {
		return ""
	}
	if v, ok := m[field]; ok {
		switch tv := v.(type) {
		case string:
			return tv
		default:
			b, _ := json.Marshal(tv)
			return string(b)
		}
	}
	return ""
}

// renderTemplate replaces {token} placeholders in a template string with event values.
// Built-in tokens: {display_text}, {severity}, {source_name}.
// Any {key} matching a top-level Payload JSON key is also replaced.
func renderTemplate(tmpl string, event models.Event) string {
	result := tmpl
	result = strings.ReplaceAll(result, "{display_text}", event.Title)
	result = strings.ReplaceAll(result, "{severity}", event.Level)
	result = strings.ReplaceAll(result, "{source_name}", event.SourceName)

	if event.Payload != "" {
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(event.Payload), &m); err == nil {
			for k, v := range m {
				var vs string
				switch tv := v.(type) {
				case string:
					vs = tv
				default:
					b, _ := json.Marshal(tv)
					vs = string(b)
				}
				result = strings.ReplaceAll(result, "{"+k+"}", vs)
			}
		}
	}
	return result
}
