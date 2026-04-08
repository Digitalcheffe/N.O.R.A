package monitor

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/digitalcheffe/nora/internal/repo"
)

// TestSSLChecker_Standalone_UsesRunner verifies that standalone mode calls the runner.
func TestSSLChecker_Standalone_UsesRunner(t *testing.T) {
	runnerCalled := false
	runner := func(_ context.Context, _ string, _, _ int) Result {
		runnerCalled = true
		days := 90
		expires := time.Now().Add(90 * 24 * time.Hour).UTC()
		issuer, subject := "CA", "example.com"
		details, _ := json.Marshal(sslDetails{DaysRemaining: &days, ExpiresAt: &expires, Issuer: &issuer, Subject: &subject})
		return Result{Status: "up", Details: details, CheckedAt: time.Now()}
	}

	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	store := &repo.Store{Checks: checks, Events: events}
	checker := &SSLChecker{store: store, runner: runner}

	// No ssl_source → standalone mode
	check := makeSSLCheck("https://example.com", "up", "", 30, 7)
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !runnerCalled {
		t.Error("expected runner to be called in standalone mode")
	}
}
