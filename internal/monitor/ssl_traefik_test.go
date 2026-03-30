package monitor

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// mockInfraRepo satisfies repo.InfraRepo for SSL checker tests.
type mockInfraRepo struct {
	repo.InfraRepo
	cert *models.TraefikCert
	err  error
}

func (m *mockInfraRepo) GetCertByDomain(_ context.Context, _ string) (*models.TraefikCert, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.cert, nil
}

func newTestStoreWithInfra(checks *mockCheckRepo, events *mockEventRepo, infra repo.InfraRepo) *repo.Store {
	return &repo.Store{Checks: checks, Events: events, Infra: infra}
}

func makeTraefikSSLCheck(domain, lastStatus, appID string, warnDays, critDays int) *models.MonitorCheck {
	src := "traefik"
	return &models.MonitorCheck{
		ID:           "ssl-traefik-1",
		AppID:        appID,
		Name:         "Traefik SSL",
		Type:         "ssl",
		Target:       domain,
		IntervalSecs: 3600,
		SSLWarnDays:  warnDays,
		SSLCritDays:  critDays,
		SSLSource:    &src,
		LastStatus:   lastStatus,
	}
}

// TestSSLChecker_Traefik_HappyPath verifies a cert with >30 days remaining returns "up".
func TestSSLChecker_Traefik_HappyPath(t *testing.T) {
	expires := time.Now().Add(90 * 24 * time.Hour).UTC()
	infraRepo := &mockInfraRepo{cert: &models.TraefikCert{
		Domain:    "example.com",
		ExpiresAt: &expires,
	}}

	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	store := newTestStoreWithInfra(checks, events, infraRepo)
	checker := &SSLChecker{store: store, runner: RunSSL}

	check := makeTraefikSSLCheck("example.com", "up", "", 30, 7)
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(checks.updateStatusCalls) != 1 {
		t.Fatalf("expected 1 UpdateStatus call, got %d", len(checks.updateStatusCalls))
	}
	if checks.updateStatusCalls[0].status != "up" {
		t.Errorf("expected up, got %s", checks.updateStatusCalls[0].status)
	}
	// No network call — runner should NOT have been invoked.
	if len(events.created) != 0 {
		t.Errorf("expected no events, got %d", len(events.created))
	}
}

// TestSSLChecker_Traefik_WarnTransition verifies warn event fires on transition.
func TestSSLChecker_Traefik_WarnTransition(t *testing.T) {
	expires := time.Now().Add(20 * 24 * time.Hour).UTC()
	infraRepo := &mockInfraRepo{cert: &models.TraefikCert{
		Domain:    "example.com",
		ExpiresAt: &expires,
	}}

	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	store := newTestStoreWithInfra(checks, events, infraRepo)
	checker := &SSLChecker{store: store, runner: RunSSL}

	check := makeTraefikSSLCheck("example.com", "up", "app-1", 30, 7)
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if checks.updateStatusCalls[0].status != "warn" {
		t.Errorf("expected warn, got %s", checks.updateStatusCalls[0].status)
	}
	if len(events.created) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events.created))
	}
	if events.created[0].Level != "warn" {
		t.Errorf("expected warn level, got %s", events.created[0].Level)
	}
}

// TestSSLChecker_Traefik_CertNotFound verifies "down" when cert is not in cache.
func TestSSLChecker_Traefik_CertNotFound(t *testing.T) {
	infraRepo := &mockInfraRepo{err: repo.ErrNotFound}

	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	store := newTestStoreWithInfra(checks, events, infraRepo)
	checker := &SSLChecker{store: store, runner: RunSSL}

	check := makeTraefikSSLCheck("unknown.com", "up", "", 30, 7)
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if checks.updateStatusCalls[0].status != "down" {
		t.Errorf("expected down when cert not found, got %s", checks.updateStatusCalls[0].status)
	}
}

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
