package ingest

import (
	"context"
	"encoding/json"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/digitalcheffe/nora/internal/apptemplate"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/google/uuid"
)

// Result is returned by Process on success.
type Result struct {
	EventID string
}

var arrayIndexRe = regexp.MustCompile(`^([^\[]+)\[(\d+)\]$`)

// ErrInvalidToken is returned when no app matches the token.
type ErrInvalidToken struct{}

func (e ErrInvalidToken) Error() string { return "invalid token" }

// ErrRateLimited is returned when the app's rate limit is exceeded.
type ErrRateLimited struct{}

func (e ErrRateLimited) Error() string { return "rate limit exceeded" }

// Process runs the full ingest pipeline for a raw JSON payload arriving under token.
// rawBody must be valid JSON. The caller is responsible for validating this before calling Process.
func Process(ctx context.Context, store *repo.Store, profiler apptemplate.Loader, limiter *RateLimiter, token string, rawBody []byte) (*Result, error) {
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
	// Severity → level mapping for profile data:
	//   critical → critical, high → error, medium → warn, low → info, info → info, debug → debug
	level := "info"
	title := "Event received"
	fieldsMap := map[string]string{}

	if profiler != nil && app.ProfileID != "" {
		p, err := profiler.Get(app.ProfileID)
		if err == nil && p != nil {
			fieldsMap = extractFields(rawBody, p.Webhook.FieldMappings)
			// Synthesize event_type from payload structure when the template uses key detection.
			if _, hasET := fieldsMap["event_type"]; !hasET && len(p.Webhook.EventTypeKeys) > 0 {
				var decoded interface{}
				if jsonErr := json.Unmarshal(rawBody, &decoded); jsonErr == nil {
					if et := apptemplate.InferEventTypeFromKeys(decoded, p.Webhook.EventTypeKeys); et != "" {
						fieldsMap["event_type"] = et
					}
				}
			}
			level = mapSeverity(fieldsMap, p.Webhook.SeverityField, p.Webhook.SeverityCompoundField, p.Webhook.SeverityMapping)
			// Pick per-eventType template if available, fall back to global template.
			tmpl := p.Webhook.DisplayTemplate
			if len(p.Webhook.DisplayTemplates) > 0 {
				if et, ok := fieldsMap["event_type"]; ok {
					if specific, ok := p.Webhook.DisplayTemplates[et]; ok {
						tmpl = specific
					}
				}
			}
			title = renderTemplate(tmpl, fieldsMap)
		}
	}

	// Step 4 — Build merged payload: raw webhook JSON with extracted fields
	// overlaid as top-level keys so event_type etc. are queryable.
	payload := mergePayload(rawBody, fieldsMap)

	// Step 5 — Persist event
	event := &models.Event{
		ID:         uuid.NewString(),
		Level:      level,
		SourceName: app.Name,
		SourceType: "app",
		SourceID:   app.ID,
		Title:      title,
		Payload:    payload,
		CreatedAt:  time.Now().UTC(),
	}
	if err := store.Events.Create(ctx, event); err != nil {
		return nil, err
	}

	log.Printf("event ingested source_id=%s event_id=%s level=%s", app.ID, event.ID, event.Level)

	return &Result{EventID: event.ID}, nil
}

// mergePayload merges extracted fields onto the raw JSON body as top-level keys,
// making profile-normalized fields (e.g. event_type) queryable via json_extract.
// The raw body is returned as-is if merging fails.
func mergePayload(rawBody []byte, fields map[string]string) string {
	var obj map[string]interface{}
	if err := json.Unmarshal(rawBody, &obj); err != nil || obj == nil {
		obj = map[string]interface{}{}
	}
	for k, v := range fields {
		obj[k] = v
	}
	merged, err := json.Marshal(obj)
	if err != nil {
		return string(rawBody)
	}
	return string(merged)
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

// jsonPathGet resolves a JSONPath expression against a decoded JSON value.
// Supports dot-notation ($.field.nested) and array indexing ($.arr[0].field).
func jsonPathGet(v interface{}, path string) (string, bool) {
	path = strings.TrimPrefix(path, "$.")
	path = strings.TrimPrefix(path, "$")
	if path == "" {
		return toString(v), true
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
			return toString(child), true
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
		return toString(child), true
	}
	return jsonPathGet(child, rest)
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
// When compoundField is set, tries a compound key "{primary}:{compound}" first,
// then falls back to the primary key alone. Returns "info" if no match is found.
func mapSeverity(fields map[string]string, severityField, compoundField string, mapping map[string]string) string {
	if severityField == "" || len(mapping) == 0 {
		return "info"
	}
	val, ok := fields[severityField]
	if !ok {
		return "info"
	}
	// Try compound key: "primary:compound" (e.g. "Health:error")
	if compoundField != "" {
		if compound := fields[compoundField]; compound != "" {
			if s, ok := mapping[val+":"+compound]; ok {
				return s
			}
		}
	}
	if s, ok := mapping[val]; ok {
		return s
	}
	return "info"
}

// renderTemplate substitutes {field_name} tokens in tmpl with values from fields.
// Returns "Event received" if tmpl is empty.
// If unresolved {tokens} remain after substitution (payload didn't match this
// template shape), falls back to the event_type field value or "Event received"
// so every event always gets a clean, readable display_text.
func renderTemplate(tmpl string, fields map[string]string) string {
	if tmpl == "" {
		return "Event received"
	}
	result := tmpl
	for k, v := range fields {
		result = strings.ReplaceAll(result, "{"+k+"}", v)
	}
	// Unresolved tokens mean the payload shape didn't match this template.
	// Fall back to a clean label rather than leaving raw {placeholders}.
	if strings.Contains(result, "{") {
		if et := fields["event_type"]; et != "" {
			return et
		}
		return "Event received"
	}
	return result
}
