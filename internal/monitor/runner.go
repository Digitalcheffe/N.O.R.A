package monitor

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// Result is the outcome of a single check execution.
type Result struct {
	Status    string          `json:"status"`     // up | warn | down
	Details   json.RawMessage `json:"details"`    // check-type-specific data
	CheckedAt time.Time       `json:"checked_at"`
}

// pingDetails is stored in last_result for ping checks.
type pingDetails struct {
	LatencyMs int64 `json:"latency_ms,omitempty"`
}

// urlDetails is stored in last_result for url checks.
type urlDetails struct {
	StatusCode int   `json:"status_code"`
	LatencyMs  int64 `json:"latency_ms"`
}

// sslDetails is stored in last_result for ssl checks.
type sslDetails struct {
	DaysRemaining int       `json:"days_remaining"`
	ExpiresAt     time.Time `json:"expires_at"`
}

// RunPing executes a ping check against target using os/exec.
// Returns up/down and round-trip latency.
func RunPing(ctx context.Context, target string) Result {
	now := time.Now().UTC()
	start := time.Now()

	// -c 1: one packet, -W 2: 2 second wait (Linux/Alpine compatible)
	cmd := exec.CommandContext(ctx, "ping", "-c", "1", "-W", "2", target)
	err := cmd.Run()

	latency := time.Since(start).Milliseconds()
	details, _ := json.Marshal(pingDetails{LatencyMs: latency})

	status := "up"
	if err != nil {
		status = "down"
		details, _ = json.Marshal(pingDetails{})
	}
	return Result{Status: status, Details: details, CheckedAt: now}
}

// RunURL executes an HTTP check against target, verifying the response status code.
func RunURL(ctx context.Context, target string, expectedStatus int) Result {
	now := time.Now().UTC()
	if expectedStatus == 0 {
		expectedStatus = 200
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		// Don't follow redirects — check the actual status code returned.
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	start := time.Now()
	resp, err := client.Get(target) //nolint:noctx // context applied via client timeout
	latency := time.Since(start).Milliseconds()

	if err != nil {
		details, _ := json.Marshal(urlDetails{LatencyMs: latency})
		return Result{Status: "down", Details: details, CheckedAt: now}
	}
	resp.Body.Close()

	details, _ := json.Marshal(urlDetails{StatusCode: resp.StatusCode, LatencyMs: latency})
	status := "up"
	if resp.StatusCode != expectedStatus {
		status = "down"
	}
	return Result{Status: status, Details: details, CheckedAt: now}
}

// RunSSL dials target over TLS and inspects the certificate expiry.
// warn threshold is warnDays before expiry; crit is critDays.
func RunSSL(ctx context.Context, target string, warnDays, critDays int) Result {
	now := time.Now().UTC()
	if warnDays == 0 {
		warnDays = 30
	}
	if critDays == 0 {
		critDays = 7
	}

	// Strip scheme if present to get host:port for TLS dial.
	host := target
	for _, prefix := range []string{"https://", "http://"} {
		host = strings.TrimPrefix(host, prefix)
	}
	// Strip any path component.
	if i := strings.Index(host, "/"); i != -1 {
		host = host[:i]
	}
	// Default to port 443 if not specified.
	if !strings.Contains(host, ":") {
		host = host + ":443"
	}

	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: 10 * time.Second},
		Config:    &tls.Config{InsecureSkipVerify: false}, //nolint:gosec
	}
	conn, err := dialer.DialContext(ctx, "tcp", host)
	if err != nil {
		details, _ := json.Marshal(map[string]string{"error": err.Error()})
		return Result{Status: "down", Details: details, CheckedAt: now}
	}
	defer conn.Close()

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		details, _ := json.Marshal(map[string]string{"error": "not a TLS connection"})
		return Result{Status: "down", Details: details, CheckedAt: now}
	}

	certs := tlsConn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		details, _ := json.Marshal(map[string]string{"error": "no certificates"})
		return Result{Status: "down", Details: details, CheckedAt: now}
	}

	expiry := certs[0].NotAfter
	daysRemaining := int(time.Until(expiry).Hours() / 24)
	details, _ := json.Marshal(sslDetails{DaysRemaining: daysRemaining, ExpiresAt: expiry.UTC()})

	status := "up"
	switch {
	case daysRemaining <= 0:
		status = "down"
	case daysRemaining <= critDays:
		status = "down"
	case daysRemaining <= warnDays:
		status = "warn"
	}

	return Result{Status: status, Details: details, CheckedAt: now}
}

// Run dispatches a check by type and returns the result.
// checkType must be one of "ping", "url", "ssl".
// expectedStatus is used for url checks; warnDays/critDays for ssl checks.
func Run(ctx context.Context, checkType, target string, expectedStatus, warnDays, critDays int) (Result, error) {
	switch checkType {
	case "ping":
		return RunPing(ctx, target), nil
	case "url":
		return RunURL(ctx, target, expectedStatus), nil
	case "ssl":
		return RunSSL(ctx, target, warnDays, critDays), nil
	default:
		return Result{}, fmt.Errorf("unknown check type: %s", checkType)
	}
}
