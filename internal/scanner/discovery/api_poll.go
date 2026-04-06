package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/digitalcheffe/nora/internal/apipoller"
	"github.com/digitalcheffe/nora/internal/apptemplate"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/google/uuid"
)

// RunAPIPolling iterates all apps, finds those with a non-empty api_polling block
// in their profile, and calls pollApp for each. It is registered as a
// GlobalDiscoveryJob and runs on the hourly Discovery cadence.
func RunAPIPolling(ctx context.Context, store *repo.Store, registry apptemplate.Loader) error {
	apps, err := store.Apps.List(ctx)
	if err != nil {
		return fmt.Errorf("api polling: list apps: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}

	for _, app := range apps {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if app.ProfileID == "" {
			continue
		}
		tmpl, err := registry.Get(app.ProfileID)
		if err != nil || tmpl == nil || len(tmpl.APIPolling) == 0 {
			continue
		}
		if err := pollApp(ctx, store, client, app, tmpl.APIPolling); err != nil {
			log.Printf("api polling: app %s (%s): %v", app.Name, app.ID, err)
		}
	}
	return nil
}

// pollApp executes each api_polling entry for a single app.
func pollApp(
	ctx context.Context,
	store *repo.Store,
	client *http.Client,
	app models.App,
	entries []apptemplate.APIPollingEntry,
) error {
	var cfg map[string]interface{}
	if err := json.Unmarshal(app.Config, &cfg); err != nil {
		cfg = map[string]interface{}{}
	}
	baseURL := apipoller.ResolveAPIBaseURL(cfg)
	apiKey, _ := cfg["api_key"].(string)

	for _, entry := range entries {
		if err := pollEntry(ctx, store, client, app, entry, baseURL, apiKey); err != nil {
			log.Printf("api polling: app %s (%s): entry %q: %v", app.Name, app.ID, entry.Name, err)
		}
	}
	return nil
}

// pollEntry executes a single APIPollingEntry for an app.
func pollEntry(
	ctx context.Context,
	store *repo.Store,
	client *http.Client,
	app models.App,
	entry apptemplate.APIPollingEntry,
	baseURL, apiKey string,
) error {
	rawURL := strings.TrimRight(baseURL, "/") + entry.Path

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	// Attach auth credentials to the request without embedding secrets in any URL string.
	if auth := apipoller.Get(app.ProfileID); auth != nil && apiKey != "" {
		switch auth.AuthType {
		case "apikey_header":
			if auth.AuthHeader != "" {
				req.Header.Set(auth.AuthHeader, apiKey)
			}
		case "apikey_query":
			if auth.AuthHeader != "" {
				q := req.URL.Query()
				q.Set(auth.AuthHeader, apiKey)
				req.URL.RawQuery = q.Encode()
			}
		case "bearer":
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("api polling: app %s: GET %s: %v", app.Name, entry.Path, err)
		return nil // log and skip; do not fire an event
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("api polling: app %s: GET %s: non-200 status %d", app.Name, entry.Path, resp.StatusCode)
		return nil // log and skip; do not fire an event
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("api polling: app %s: read body %s: %v", app.Name, entry.Path, err)
		return nil
	}

	value, err := ExtractValue(body, entry.Target, entry.ValueType)
	if err != nil {
		log.Printf("api polling: app %s: entry %q: extract value: %v", app.Name, entry.Name, err)
		return nil
	}

	now := time.Now().UTC()
	snap := models.AppMetricSnapshot{
		ID:         uuid.NewString(),
		AppID:      app.ID,
		ProfileID:  app.ProfileID,
		MetricName: entry.Name,
		Label:      entry.Label,
		Value:      value,
		ValueType:  entry.ValueType,
		PolledAt:   now,
	}
	if err := store.AppMetricSnapshots.Upsert(ctx, snap); err != nil {
		return fmt.Errorf("upsert snapshot: %w", err)
	}

	msg := entry.EventMessage
	if msg == "" {
		msg = "{label}: {value}"
	}
	msg = strings.ReplaceAll(msg, "{label}", entry.Label)
	msg = strings.ReplaceAll(msg, "{value}", value)

	ev := &models.Event{
		ID:         uuid.NewString(),
		Level:      "info",
		SourceName: app.Name,
		SourceType: "app",
		SourceID:   app.ID,
		Title:      msg,
		Payload:    fmt.Sprintf(`{"metric":%q,"value":%q,"label":%q}`, entry.Name, value, entry.Label),
		CreatedAt:  now,
	}
	if err := store.Events.Create(ctx, ev); err != nil {
		log.Printf("api polling: app %s: create event: %v", app.Name, err)
	}

	return nil
}

// ExtractValue extracts a scalar value from a JSON response body.
//
//   - target == "length": body must be a JSON array; returns its length as a string.
//   - target starts with "$": JSONPath extraction; for value_type "list" the
//     extracted slice is marshalled back to a JSON string.
//   - any other target: returns an error.
func ExtractValue(body []byte, target, valueType string) (string, error) {
	if target == "length" {
		var arr []interface{}
		if err := json.Unmarshal(body, &arr); err != nil {
			return "", fmt.Errorf("length target: unmarshal as array: %w", err)
		}
		return strconv.Itoa(len(arr)), nil
	}

	if strings.HasPrefix(target, "$") {
		var root interface{}
		if err := json.Unmarshal(body, &root); err != nil {
			return "", fmt.Errorf("jsonpath: unmarshal body: %w", err)
		}
		raw, ok := pollJsonPathGetRaw(root, target)
		if !ok {
			return "", fmt.Errorf("jsonpath %q: path not found", target)
		}
		if valueType == "list" {
			b, err := json.Marshal(raw)
			if err != nil {
				return "", fmt.Errorf("marshal list value: %w", err)
			}
			return string(b), nil
		}
		return pollJsonPathToString(raw), nil
	}

	return "", fmt.Errorf("unsupported target %q: must be \"length\" or a JSONPath starting with \"$\"", target)
}

// pollJsonPathGetRaw resolves a JSONPath expression and returns the raw
// interface{} value (not stringified). Used so callers can choose how to
// serialise the result (e.g. marshal a slice for value_type=list).
func pollJsonPathGetRaw(v interface{}, path string) (interface{}, bool) {
	path = strings.TrimPrefix(path, "$.")
	path = strings.TrimPrefix(path, "$")
	if path == "" {
		return v, true
	}

	parts := strings.SplitN(path, ".", 2)
	segment := parts[0]
	rest := ""
	if len(parts) == 2 {
		rest = parts[1]
	}

	if m := pollArrayIndexRe.FindStringSubmatch(segment); m != nil {
		key := m[1]
		idx, _ := strconv.Atoi(m[2])
		obj, ok := v.(map[string]interface{})
		if !ok {
			return nil, false
		}
		arr, ok := obj[key].([]interface{})
		if !ok || idx >= len(arr) {
			return nil, false
		}
		child := arr[idx]
		if rest == "" {
			return child, true
		}
		return pollJsonPathGetRaw(child, rest)
	}

	obj, ok := v.(map[string]interface{})
	if !ok {
		return nil, false
	}
	child, ok := obj[segment]
	if !ok {
		return nil, false
	}
	if rest == "" {
		return child, true
	}
	return pollJsonPathGetRaw(child, rest)
}

// pollJsonPathToString converts a JSON value to a string for storage.
func pollJsonPathToString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

var pollArrayIndexRe = regexp.MustCompile(`^([^\[]+)\[(\d+)\]$`)
