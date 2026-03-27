package monitor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/digitalcheffe/nora/internal/models"
)

// makeURLCheck returns a MonitorCheck suitable for URL checker tests.
func makeURLCheck(target, lastStatus, appID string, expectedStatus int) *models.MonitorCheck {
	return &models.MonitorCheck{
		ID:             "url-check-1",
		AppID:          appID,
		Name:           "My Service",
		Type:           "url",
		Target:         target,
		IntervalSecs:   60,
		ExpectedStatus: expectedStatus,
		LastStatus:     lastStatus,
	}
}

func newURLChecker(checks *mockCheckRepo, events *mockEventRepo) *URLChecker {
	return &URLChecker{
		store: newTestStore(checks, events),
		client: &http.Client{
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// TestURLChecker_HappyPath verifies a 200 response marks the check "up".
func TestURLChecker_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := newURLChecker(checks, events)

	check := makeURLCheck(srv.URL, "up", "", 200)
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(checks.updateStatusCalls) != 1 {
		t.Fatalf("expected 1 UpdateStatus call, got %d", len(checks.updateStatusCalls))
	}
	if checks.updateStatusCalls[0].status != "up" {
		t.Errorf("expected status=up, got %s", checks.updateStatusCalls[0].status)
	}
	if len(events.created) != 0 {
		t.Errorf("expected no events, got %d", len(events.created))
	}
}

// TestURLChecker_WrongStatusDown verifies a non-matching status code marks the check "down".
func TestURLChecker_WrongStatusDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := newURLChecker(checks, events)

	check := makeURLCheck(srv.URL, "up", "app-1", 200)
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(checks.updateStatusCalls) != 1 {
		t.Fatalf("expected 1 UpdateStatus call, got %d", len(checks.updateStatusCalls))
	}
	if checks.updateStatusCalls[0].status != "down" {
		t.Errorf("expected status=down, got %s", checks.updateStatusCalls[0].status)
	}
	if len(events.created) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events.created))
	}
	if events.created[0].Severity != "error" {
		t.Errorf("expected severity=error, got %s", events.created[0].Severity)
	}
}

// TestURLChecker_Recovery verifies down→up transition creates an info event.
func TestURLChecker_Recovery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := newURLChecker(checks, events)

	check := makeURLCheck(srv.URL, "down", "app-1", 200)
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events.created) != 1 {
		t.Fatalf("expected 1 recovery event, got %d", len(events.created))
	}
	if events.created[0].Severity != "info" {
		t.Errorf("expected severity=info, got %s", events.created[0].Severity)
	}
}

// TestURLChecker_RedirectDown verifies that an unexpected redirect (3xx) is treated as "down".
func TestURLChecker_RedirectDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/other", http.StatusFound)
	}))
	defer srv.Close()

	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := newURLChecker(checks, events)

	// Expect 200 but server returns 302 → should be "down".
	check := makeURLCheck(srv.URL, "up", "app-1", 200)
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if checks.updateStatusCalls[0].status != "down" {
		t.Errorf("expected status=down for redirect, got %s", checks.updateStatusCalls[0].status)
	}
}

// TestURLChecker_RedirectExpected verifies that a 3xx is treated as "up" when explicitly expected.
func TestURLChecker_RedirectExpected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/other", http.StatusFound)
	}))
	defer srv.Close()

	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := newURLChecker(checks, events)

	check := makeURLCheck(srv.URL, "up", "", 302)
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if checks.updateStatusCalls[0].status != "up" {
		t.Errorf("expected status=up for expected redirect, got %s", checks.updateStatusCalls[0].status)
	}
}

// TestURLChecker_Timeout verifies that a connection timeout results in "down".
func TestURLChecker_Timeout(t *testing.T) {
	// Use a non-routable address to force a connection timeout quickly.
	// context.WithTimeout simulates the 10s timeout in unit tests.
	ctx, cancel := context.WithTimeout(context.Background(), 50)
	defer cancel()

	// Point to a closed port on localhost to get a fast connection refused.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	srv.Close() // immediately close so requests get "connection refused"

	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := newURLChecker(checks, events)

	check := makeURLCheck(srv.URL, "up", "app-1", 200)
	// Run may return an error (network error) — that's acceptable.
	_ = checker.Run(ctx, check)

	if len(checks.updateStatusCalls) != 1 {
		t.Fatalf("expected 1 UpdateStatus call, got %d", len(checks.updateStatusCalls))
	}
	if checks.updateStatusCalls[0].status != "down" {
		t.Errorf("expected status=down on network error, got %s", checks.updateStatusCalls[0].status)
	}
}

// TestURLChecker_AuthHeader verifies the auth_header from last_result is sent.
func TestURLChecker_AuthHeader(t *testing.T) {
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := newURLChecker(checks, events)

	lastResult, _ := json.Marshal(map[string]string{"auth_header": "Bearer secret-token"})
	check := makeURLCheck(srv.URL, "up", "", 200)
	check.LastResult = string(lastResult)

	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedAuth != "Bearer secret-token" {
		t.Errorf("expected Authorization header to be sent, got %q", receivedAuth)
	}
}

// TestURLChecker_DefaultExpected200 verifies that ExpectedStatus=0 defaults to 200.
func TestURLChecker_DefaultExpected200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := newURLChecker(checks, events)

	check := makeURLCheck(srv.URL, "up", "", 0) // 0 = unset, should default to 200
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if checks.updateStatusCalls[0].status != "up" {
		t.Errorf("expected status=up with default 200, got %s", checks.updateStatusCalls[0].status)
	}
}

// TestURLChecker_NoEventWithoutApp verifies no event is created for app-less checks.
func TestURLChecker_NoEventWithoutApp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := newURLChecker(checks, events)

	check := makeURLCheck(srv.URL, "up", "", 200) // no AppID
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events.created) != 0 {
		t.Errorf("expected no events for check without app, got %d", len(events.created))
	}
}

// TestURLChecker_NoEventOnFirstRun verifies no event on first execution (LastStatus="").
func TestURLChecker_NoEventOnFirstRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := newURLChecker(checks, events)

	check := makeURLCheck(srv.URL, "", "app-1", 200) // no previous status
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events.created) != 0 {
		t.Errorf("expected no event on first run, got %d", len(events.created))
	}
}

// TestURLChecker_LastResultStoredCorrectly verifies the last_result JSON structure.
func TestURLChecker_LastResultStoredCorrectly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := newURLChecker(checks, events)

	check := makeURLCheck(srv.URL, "up", "", 200)
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(checks.updateStatusCalls) == 0 {
		t.Fatal("expected UpdateStatus to be called")
	}

	var result urlResult
	if err := json.Unmarshal([]byte(checks.updateStatusCalls[0].details), &result); err != nil {
		t.Fatalf("last_result is not valid JSON: %v", err)
	}
	if result.StatusCode != 200 {
		t.Errorf("expected status_code=200, got %d", result.StatusCode)
	}
	if result.Error != nil {
		t.Errorf("expected error=null, got %v", *result.Error)
	}
}
