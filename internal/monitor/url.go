package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/google/uuid"
)

// clientForCheck returns an http.Client appropriate for check.
// When SkipTLSVerify is set a fresh client with InsecureSkipVerify is returned;
// otherwise the shared URLChecker client is reused.
func (u *URLChecker) clientForCheck(check *models.MonitorCheck) *http.Client {
	if !check.SkipTLSVerify {
		return u.client
	}
	return &http.Client{
		Timeout:   u.client.Timeout,
		Transport: tlsTransport(true),
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// URLChecker executes HTTP GET health checks and persists results via the store.
type URLChecker struct {
	store  *repo.Store
	client *http.Client
}

// NewURLChecker returns a URLChecker backed by store with a 10-second timeout.
func NewURLChecker(store *repo.Store) *URLChecker {
	return &URLChecker{
		store: store,
		client: &http.Client{
			Timeout: 10 * time.Second,
			// Do not follow redirects automatically — a 3xx should be treated
			// as a non-match unless the check explicitly expects one.
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// urlResult is the JSON payload stored in last_result.
type urlResult struct {
	StatusCode int    `json:"status_code"`
	LatencyMs  int64  `json:"latency_ms"`
	Error      *string `json:"error"`
}

// authConfig is the subset of last_result we parse to find an auth header.
type authConfig struct {
	AuthHeader string `json:"auth_header"`
}

// Run performs one URL health check cycle for check.
//
// It makes an HTTP GET to check.Target, optionally adding an auth header from
// last_result, and compares the response status code to check.ExpectedStatus
// (defaulting to 200). On a status transition, an event is created — but only
// when the check is linked to an app (events require a valid app_id).
func (u *URLChecker) Run(ctx context.Context, check *models.MonitorCheck) error {
	expected := check.ExpectedStatus
	if expected == 0 {
		expected = 200
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, check.Target, nil)
	if err != nil {
		return u.recordError(ctx, check, fmt.Sprintf("build request: %v", err))
	}

	// If last_result contains an auth_header, add it to the request.
	if check.LastResult != "" {
		var cfg authConfig
		if jsonErr := json.Unmarshal([]byte(check.LastResult), &cfg); jsonErr == nil && cfg.AuthHeader != "" {
			req.Header.Set("Authorization", cfg.AuthHeader)
		}
	}

	start := time.Now()
	resp, err := u.clientForCheck(check).Do(req)
	latencyMs := time.Since(start).Milliseconds()

	now := time.Now().UTC()

	if err != nil {
		return u.recordError(ctx, check, err.Error())
	}
	resp.Body.Close()

	newStatus := "up"
	if resp.StatusCode != expected {
		newStatus = "down"
	}

	errPtr := (*string)(nil)
	result := urlResult{
		StatusCode: resp.StatusCode,
		LatencyMs:  latencyMs,
		Error:      errPtr,
	}
	details, _ := json.Marshal(result)

	// Emit a status-change event when there is a known previous state and the
	// check is linked to an app.
	prevStatus := check.LastStatus
	if prevStatus != "" && prevStatus != newStatus {
		if evErr := u.createStatusEvent(ctx, check, newStatus, resp.StatusCode, expected, now); evErr != nil {
			log.Printf("url checker: create event for check %s: %v", check.ID, evErr)
		}
	}

	if updateErr := u.store.Checks.UpdateStatus(ctx, check.ID, newStatus, string(details), now); updateErr != nil {
		return fmt.Errorf("url checker: update status for %s: %w", check.ID, updateErr)
	}
	return nil
}

// recordError handles a network-level failure: records the check as "down",
// persists an error last_result, emits a status-change event if needed, and
// returns the wrapped error.
func (u *URLChecker) recordError(ctx context.Context, check *models.MonitorCheck, errMsg string) error {
	now := time.Now().UTC()
	newStatus := "down"

	result := urlResult{StatusCode: 0, LatencyMs: 0, Error: &errMsg}
	details, _ := json.Marshal(result)

	expected := check.ExpectedStatus
	if expected == 0 {
		expected = 200
	}

	prevStatus := check.LastStatus
	if prevStatus != "" && prevStatus != newStatus {
		if evErr := u.createStatusEvent(ctx, check, newStatus, 0, expected, now); evErr != nil {
			log.Printf("url checker: create event for check %s: %v", check.ID, evErr)
		}
	}

	if updateErr := u.store.Checks.UpdateStatus(ctx, check.ID, newStatus, string(details), now); updateErr != nil {
		return fmt.Errorf("url checker: update status for %s: %w", check.ID, updateErr)
	}
	return fmt.Errorf("url checker: %s: %s", check.Name, errMsg)
}

// createStatusEvent persists a down or recovery event for check.
func (u *URLChecker) createStatusEvent(
	ctx context.Context,
	check *models.MonitorCheck,
	newStatus string,
	gotStatus, expectedStatus int,
	now time.Time,
) error {
	var severity, displayText string
	if newStatus == "down" {
		severity = "error"
		displayText = fmt.Sprintf("URL check failed — %s: got %d, expected %d",
			check.Name, gotStatus, expectedStatus)
	} else {
		severity = "info"
		displayText = fmt.Sprintf("URL check restored — %s", check.Name)
	}

	event := &models.Event{
		ID:         uuid.New().String(),
		Level:      severity,
		SourceName: check.Name,
		SourceType: "monitor_check",
		SourceID:   check.ID,
		Title:      displayText,
		Payload:    `{"type":"url"}`,
		CreatedAt:  now,
	}
	return u.store.Events.Create(ctx, event)
}
