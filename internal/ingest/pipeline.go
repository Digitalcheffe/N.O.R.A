package ingest

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/profile"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/google/uuid"
)

// Result is returned by Process on success.
type Result struct {
	EventID string
}

// ErrInvalidToken is returned when no app matches the token.
type ErrInvalidToken struct{}

func (e ErrInvalidToken) Error() string { return "invalid token" }

// ErrRateLimited is returned when the app's rate limit is exceeded.
type ErrRateLimited struct{}

func (e ErrRateLimited) Error() string { return "rate limit exceeded" }

// Process runs the full ingest pipeline for a raw JSON payload arriving under token.
// rawBody must be valid JSON. The caller is responsible for validating this before calling Process.
func Process(ctx context.Context, store *repo.Store, profiler profile.Loader, limiter *RateLimiter, token string, rawBody []byte) (*Result, error) {
	// Step 1 — Token lookup
	app, err := store.Apps.GetByToken(ctx, token)
	if err != nil {
		return nil, ErrInvalidToken{}
	}

	// Step 2 — Rate limiting
	rateLimit := app.RateLimit
	if rateLimit <= 0 {
		rateLimit = 100
	}
	if !limiter.Allow(app.ID, rateLimit) {
		log.Printf("rate limit exceeded app_id=%s limit=%d", app.ID, rateLimit)
		return nil, ErrRateLimited{}
	}

	// Step 3 — Profile processing
	severity := "info"
	displayText := "Event received"
	fieldsMap := map[string]string{}

	if profiler != nil && app.ProfileID != "" {
		p, err := profiler.Get(app.ProfileID)
		if err == nil && p != nil {
			fieldsMap = extractFields(rawBody, p.Webhook.FieldMappings)
			severity = mapSeverity(fieldsMap, p.Webhook.SeverityField, p.Webhook.SeverityMapping)
			displayText = renderTemplate(p.Webhook.DisplayTemplate, fieldsMap)
		}
	}

	// Step 4 — Encode fields as JSON
	fieldsJSON, err := json.Marshal(fieldsMap)
	if err != nil {
		fieldsJSON = []byte("{}")
	}

	// Step 5 — Persist event
	event := &models.Event{
		ID:          uuid.NewString(),
		AppID:       app.ID,
		ReceivedAt:  time.Now().UTC(),
		Severity:    severity,
		DisplayText: displayText,
		RawPayload:  string(rawBody),
		Fields:      string(fieldsJSON),
	}
	if err := store.Events.Create(ctx, event); err != nil {
		return nil, err
	}

	log.Printf("event ingested app_id=%s event_id=%s severity=%s", app.ID, event.ID, event.Severity)

	return &Result{EventID: event.ID}, nil
}

// extractFields evaluates each JSONPath in mappings against the decoded payload
// and returns a flat tag→value map. Paths use dot notation: $.field.nested
func extractFields(rawBody []byte, mappings map[string]string) map[string]string {
	if len(mappings) == 0 {
		return map[string]string{}
	}

	var payload interface{}
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return map[string]string{}
	}

	result := make(map[string]string, len(mappings))
	for tag, path := range mappings {
		if v, ok := jsonPathGet(payload, path); ok {
			result[tag] = v
		}
	}
	return result
}

// jsonPathGet resolves a simple JSONPath expression (e.g. "$.eventType", "$.nested.field")
// against a decoded JSON value. Returns the string representation and true on success.
func jsonPathGet(v interface{}, path string) (string, bool) {
	// Strip leading "$." or "$"
	path = strings.TrimPrefix(path, "$.")
	path = strings.TrimPrefix(path, "$")
	if path == "" {
		return toString(v), true
	}

	parts := strings.SplitN(path, ".", 2)
	m, ok := v.(map[string]interface{})
	if !ok {
		return "", false
	}
	child, ok := m[parts[0]]
	if !ok {
		return "", false
	}
	if len(parts) == 1 {
		return toString(child), true
	}
	return jsonPathGet(child, parts[1])
}

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

// mapSeverity looks up the value of severityField in fields against the mapping.
// Returns "info" if no match is found.
func mapSeverity(fields map[string]string, severityField string, mapping map[string]string) string {
	if severityField == "" || len(mapping) == 0 {
		return "info"
	}
	val, ok := fields[severityField]
	if !ok {
		return "info"
	}
	if s, ok := mapping[val]; ok {
		return s
	}
	return "info"
}

// renderTemplate substitutes {field_name} tokens in tmpl with values from fields.
// Returns "Event received" if tmpl is empty.
func renderTemplate(tmpl string, fields map[string]string) string {
	if tmpl == "" {
		return "Event received"
	}
	result := tmpl
	for k, v := range fields {
		result = strings.ReplaceAll(result, "{"+k+"}", v)
	}
	return result
}
