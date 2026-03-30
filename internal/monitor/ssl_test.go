package monitor

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
)

// makeSSLCheck returns a MonitorCheck suitable for SSL checker tests.
func makeSSLCheck(target, lastStatus, appID string, warnDays, critDays int) *models.MonitorCheck {
	return &models.MonitorCheck{
		ID:           "ssl-check-1",
		AppID:        appID,
		Name:         "My SSL",
		Type:         "ssl",
		Target:       target,
		IntervalSecs: 3600,
		SSLWarnDays:  warnDays,
		SSLCritDays:  critDays,
		LastStatus:   lastStatus,
	}
}

// fakeSSLRunner returns an injectable runner that simulates a cert result.
// Pass errMsg non-empty to simulate a TLS failure.
func fakeSSLRunner(status string, days int, errMsg string) func(ctx context.Context, target string, warnDays, critDays int) Result {
	return func(_ context.Context, _ string, _, _ int) Result {
		if errMsg != "" {
			errStr := errMsg
			details, _ := json.Marshal(sslDetails{Error: &errStr})
			return Result{Status: "down", Details: details, CheckedAt: time.Now()}
		}
		expiresAt := time.Now().Add(time.Duration(days) * 24 * time.Hour).UTC()
		issuer := "Test CA"
		subject := "test.example.com"
		details, _ := json.Marshal(sslDetails{
			DaysRemaining: &days,
			ExpiresAt:     &expiresAt,
			Issuer:        &issuer,
			Subject:       &subject,
		})
		return Result{Status: status, Details: details, CheckedAt: time.Now()}
	}
}

func newSSLChecker(checks *mockCheckRepo, events *mockEventRepo, runner func(ctx context.Context, target string, warnDays, critDays int) Result) *SSLChecker {
	return &SSLChecker{
		store:  newTestStore(checks, events),
		runner: runner,
	}
}

// TestSSLChecker_HappyPath verifies that a cert with many days remaining is "up".
func TestSSLChecker_HappyPath(t *testing.T) {
	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := newSSLChecker(checks, events, fakeSSLRunner("up", 90, ""))

	check := makeSSLCheck("https://example.com", "up", "", 30, 7)
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

// TestSSLChecker_WarnStatus verifies warn fires at ≤30 days remaining (default threshold).
func TestSSLChecker_WarnStatus(t *testing.T) {
	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := newSSLChecker(checks, events, fakeSSLRunner("warn", 20, ""))

	check := makeSSLCheck("https://example.com", "up", "app-1", 30, 7)
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if checks.updateStatusCalls[0].status != "warn" {
		t.Errorf("expected status=warn, got %s", checks.updateStatusCalls[0].status)
	}
	if len(events.created) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events.created))
	}
	ev := events.created[0]
	if ev.Level != "warn" {
		t.Errorf("expected level=warn, got %s", ev.Level)
	}
}

// TestSSLChecker_CriticalStatus verifies critical fires at ≤7 days remaining.
func TestSSLChecker_CriticalStatus(t *testing.T) {
	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := newSSLChecker(checks, events, fakeSSLRunner("critical", 3, ""))

	check := makeSSLCheck("https://example.com", "up", "app-1", 30, 7)
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if checks.updateStatusCalls[0].status != "critical" {
		t.Errorf("expected status=critical, got %s", checks.updateStatusCalls[0].status)
	}
	if len(events.created) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events.created))
	}
	ev := events.created[0]
	if ev.Level != "critical" {
		t.Errorf("expected level=critical, got %s", ev.Level)
	}
}

// TestSSLChecker_DownOnTLSError verifies TLS dial failure results in "down" with level "error".
func TestSSLChecker_DownOnTLSError(t *testing.T) {
	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := newSSLChecker(checks, events, fakeSSLRunner("down", 0, "x509: certificate has expired"))

	check := makeSSLCheck("https://expired.example.com", "up", "app-1", 30, 7)
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if checks.updateStatusCalls[0].status != "down" {
		t.Errorf("expected status=down, got %s", checks.updateStatusCalls[0].status)
	}
	if len(events.created) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events.created))
	}
	ev := events.created[0]
	if ev.Level != "error" {
		t.Errorf("expected level=error, got %s", ev.Level)
	}
}

// TestSSLChecker_Recovery verifies down→up transition creates an info event.
func TestSSLChecker_Recovery(t *testing.T) {
	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := newSSLChecker(checks, events, fakeSSLRunner("up", 90, ""))

	check := makeSSLCheck("https://example.com", "down", "app-1", 30, 7)
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events.created) != 1 {
		t.Fatalf("expected 1 recovery event, got %d", len(events.created))
	}
	ev := events.created[0]
	if ev.Level != "info" {
		t.Errorf("expected level=info for recovery, got %s", ev.Level)
	}
}

// TestSSLChecker_EventWithoutApp verifies a status-change event IS created for
// checks not linked to an app (source_type=monitor_check, queryable by source_id).
func TestSSLChecker_EventWithoutApp(t *testing.T) {
	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := newSSLChecker(checks, events, fakeSSLRunner("warn", 20, ""))

	check := makeSSLCheck("https://example.com", "up", "", 30, 7) // no AppID, up→warn transition
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events.created) != 1 {
		t.Errorf("expected 1 event for check without app on status change, got %d", len(events.created))
	}
	if events.created[0].SourceType != "monitor_check" {
		t.Errorf("expected source_type=monitor_check on event, got %s", events.created[0].SourceType)
	}
}

// TestSSLChecker_NoEventOnFirstRun verifies no event fires on the first execution (LastStatus="").
func TestSSLChecker_NoEventOnFirstRun(t *testing.T) {
	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := newSSLChecker(checks, events, fakeSSLRunner("warn", 20, ""))

	check := makeSSLCheck("https://example.com", "", "app-1", 30, 7) // no previous status
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events.created) != 0 {
		t.Errorf("expected no event on first run, got %d", len(events.created))
	}
}

// TestSSLChecker_LastResultPopulated verifies last_result contains all required fields.
func TestSSLChecker_LastResultPopulated(t *testing.T) {
	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := newSSLChecker(checks, events, fakeSSLRunner("up", 90, ""))

	check := makeSSLCheck("https://example.com", "up", "", 30, 7)
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(checks.updateStatusCalls) == 0 {
		t.Fatal("expected UpdateStatus to be called")
	}

	var result sslDetails
	if err := json.Unmarshal([]byte(checks.updateStatusCalls[0].details), &result); err != nil {
		t.Fatalf("last_result is not valid JSON: %v", err)
	}
	if result.DaysRemaining == nil {
		t.Error("expected days_remaining to be set")
	}
	if result.ExpiresAt == nil {
		t.Error("expected expires_at to be set")
	}
	if result.Issuer == nil {
		t.Error("expected issuer to be set")
	}
	if result.Subject == nil {
		t.Error("expected subject to be set")
	}
	if result.Error != nil {
		t.Errorf("expected error=null on success, got %q", *result.Error)
	}
}

// TestSSLChecker_LastResultOnError verifies last_result has error set and numeric fields null on failure.
func TestSSLChecker_LastResultOnError(t *testing.T) {
	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := newSSLChecker(checks, events, fakeSSLRunner("down", 0, "connection refused"))

	check := makeSSLCheck("https://unreachable.example.com", "", "", 30, 7)
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result sslDetails
	if err := json.Unmarshal([]byte(checks.updateStatusCalls[0].details), &result); err != nil {
		t.Fatalf("last_result is not valid JSON: %v", err)
	}
	if result.Error == nil || *result.Error == "" {
		t.Error("expected error to be set on TLS failure")
	}
	if result.DaysRemaining != nil {
		t.Error("expected days_remaining=null on TLS failure")
	}
}

// TestSSLChecker_DefaultThresholds verifies SSLWarnDays=0 and SSLCritDays=0 default to 30/7.
func TestSSLChecker_DefaultThresholds(t *testing.T) {
	// Runner captures the warnDays/critDays it was called with.
	var capturedWarn, capturedCrit int
	runner := func(_ context.Context, _ string, warnDays, critDays int) Result {
		capturedWarn = warnDays
		capturedCrit = critDays
		days := 90
		expiresAt := time.Now().Add(90 * 24 * time.Hour).UTC()
		issuer := "CA"
		subject := "example.com"
		details, _ := json.Marshal(sslDetails{
			DaysRemaining: &days,
			ExpiresAt:     &expiresAt,
			Issuer:        &issuer,
			Subject:       &subject,
		})
		return Result{Status: "up", Details: details, CheckedAt: time.Now()}
	}

	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := newSSLChecker(checks, events, runner)

	check := makeSSLCheck("https://example.com", "up", "", 0, 0) // 0 = use defaults
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedWarn != 30 {
		t.Errorf("expected warnDays=30, got %d", capturedWarn)
	}
	if capturedCrit != 7 {
		t.Errorf("expected critDays=7, got %d", capturedCrit)
	}
}

// TestSSLChecker_RealTLSServer_InvalidCert verifies that a TLS server with
// an unverifiable certificate results in a "down" status (cert validation fails).
func TestSSLChecker_RealTLSServer_InvalidCert(t *testing.T) {
	// httptest.NewTLSServer uses a self-signed cert; RunSSL with InsecureSkipVerify=false
	// will reject it, resulting in a "down" status.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	checks := &mockCheckRepo{}
	events := &mockEventRepo{}

	// Use the real RunSSL runner (InsecureSkipVerify: false rejects self-signed certs).
	checker := &SSLChecker{store: newTestStore(checks, events), runner: RunSSL}

	check := makeSSLCheck(srv.URL, "", "", 30, 7)
	_ = checker.Run(context.Background(), check)

	if len(checks.updateStatusCalls) != 1 {
		t.Fatalf("expected 1 UpdateStatus call, got %d", len(checks.updateStatusCalls))
	}
	if checks.updateStatusCalls[0].status != "down" {
		t.Errorf("expected status=down for untrusted cert, got %s", checks.updateStatusCalls[0].status)
	}
}

// TestSSLChecker_RealTLSServer_ValidCert verifies that a trusted TLS server
// results in "up" when the cert has sufficient days remaining.
func TestSSLChecker_RealTLSServer_ValidCert(t *testing.T) {
	// Use httptest's TLS server cert and trust it explicitly via custom runner.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// httptest.Server.Client() returns an *http.Client whose TLS config trusts
	// the test server's self-signed certificate.
	transport := srv.Client().Transport.(*http.Transport)
	trustedRunner := func(ctx context.Context, target string, warnDays, critDays int) Result {
		return runSSLWithConfig(ctx, target, warnDays, critDays, transport.TLSClientConfig)
	}

	checks := &mockCheckRepo{}
	events := &mockEventRepo{}
	checker := newSSLChecker(checks, events, trustedRunner)

	check := makeSSLCheck(srv.URL, "", "", 30, 7)
	if err := checker.Run(context.Background(), check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(checks.updateStatusCalls) != 1 {
		t.Fatalf("expected 1 UpdateStatus call, got %d", len(checks.updateStatusCalls))
	}
	// The httptest cert is valid for ~10 years, so status must be "up".
	got := checks.updateStatusCalls[0].status
	if got != "up" {
		t.Errorf("expected status=up for valid trusted cert, got %s", got)
	}

	// Verify last_result has the required fields populated.
	var result sslDetails
	if err := json.Unmarshal([]byte(checks.updateStatusCalls[0].details), &result); err != nil {
		t.Fatalf("last_result is not valid JSON: %v", err)
	}
	if result.DaysRemaining == nil || *result.DaysRemaining <= 0 {
		t.Error("expected days_remaining > 0 for a valid cert")
	}
	if result.ExpiresAt == nil {
		t.Error("expected expires_at to be set")
	}
}

// runSSLWithConfig is a variant of RunSSL that uses the provided *tls.Config
// to trust a specific set of certificates — used only in tests.
func runSSLWithConfig(ctx context.Context, target string, warnDays, critDays int, cfg *tls.Config) Result {
	now := time.Now().UTC()
	if warnDays == 0 {
		warnDays = 30
	}
	if critDays == 0 {
		critDays = 7
	}

	host := extractDomain(target)
	if len(host) > 0 && host[0] == '[' {
		// IPv6 literal — strip brackets for port check.
	}
	// Default to 443 if no port; httptest servers use a random port so we keep whatever's there.
	if !containsPort(host) {
		host += ":443"
	}

	dialer := &tls.Dialer{
		Config: cfg,
	}
	conn, err := dialer.DialContext(ctx, "tcp", host)
	if err != nil {
		errStr := err.Error()
		details, _ := json.Marshal(sslDetails{Error: &errStr})
		return Result{Status: "down", Details: details, CheckedAt: now}
	}
	defer conn.Close()

	tlsConn := conn.(*tls.Conn)
	certs := tlsConn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		errStr := "no certificates"
		details, _ := json.Marshal(sslDetails{Error: &errStr})
		return Result{Status: "down", Details: details, CheckedAt: now}
	}

	cert := certs[0]
	expiry := cert.NotAfter.UTC()
	daysRemaining := int(time.Until(expiry).Hours() / 24)
	issuer := cert.Issuer.CommonName
	subject := cert.Subject.CommonName
	details, _ := json.Marshal(sslDetails{
		DaysRemaining: &daysRemaining,
		ExpiresAt:     &expiry,
		Issuer:        &issuer,
		Subject:       &subject,
	})

	status := "up"
	switch {
	case daysRemaining <= 0:
		status = "down"
	case daysRemaining <= critDays:
		status = "critical"
	case daysRemaining <= warnDays:
		status = "warn"
	}
	return Result{Status: status, Details: details, CheckedAt: now}
}

// containsPort reports whether host already contains a port number.
func containsPort(host string) bool {
	// IPv6: [::1]:443
	if len(host) > 0 && host[0] == '[' {
		i := strings.Index(host, "]:")
		return i != -1
	}
	return strings.Contains(host, ":")
}
