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

// tlsTransport returns an http.Transport with optional InsecureSkipVerify.
func tlsTransport(skipVerify bool) *http.Transport {
	if !skipVerify {
		return &http.Transport{}
	}
	return &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}
}

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

// dnsDetails is stored in last_result for dns checks.
type dnsDetails struct {
	RecordType string   `json:"record_type"`
	Records    []string `json:"records,omitempty"`
	LatencyMs  int64    `json:"latency_ms"`
	Error      *string  `json:"error,omitempty"`
}

// sslDetails is stored in last_result for ssl checks.
// All fields are pointers so they serialise as null on TLS failure.
type sslDetails struct {
	DaysRemaining *int       `json:"days_remaining"`
	ExpiresAt     *time.Time `json:"expires_at"`
	Issuer        *string    `json:"issuer"`
	Subject       *string    `json:"subject"`
	Error         *string    `json:"error"`
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
// Set skipTLSVerify to true for services using self-signed certificates.
func RunURL(ctx context.Context, target string, expectedStatus int, skipTLSVerify bool) Result {
	now := time.Now().UTC()
	if expectedStatus == 0 {
		expectedStatus = 200
	}

	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: tlsTransport(skipTLSVerify),
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
		errStr := err.Error()
		details, _ := json.Marshal(sslDetails{Error: &errStr})
		return Result{Status: "down", Details: details, CheckedAt: now}
	}
	defer conn.Close()

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		errStr := "not a TLS connection"
		details, _ := json.Marshal(sslDetails{Error: &errStr})
		return Result{Status: "down", Details: details, CheckedAt: now}
	}

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

// RunDNS resolves target using the given recordType (A, AAAA, MX, CNAME, TXT).
// If expectedValue is non-empty, at least one resolved record must contain it as
// a substring for the check to be "up". Returns "down" on lookup failure or
// when the expected value is not found.
func RunDNS(ctx context.Context, target, recordType, expectedValue string) Result {
	now := time.Now().UTC()
	if recordType == "" {
		recordType = "A"
	}

	r := &net.Resolver{}
	start := time.Now()

	var records []string
	var lookupErr error

	switch strings.ToUpper(recordType) {
	case "A", "AAAA":
		addrs, err := r.LookupHost(ctx, target)
		lookupErr = err
		for _, a := range addrs {
			isIPv6 := strings.Contains(a, ":")
			if strings.ToUpper(recordType) == "AAAA" && !isIPv6 {
				continue
			}
			if strings.ToUpper(recordType) == "A" && isIPv6 {
				continue
			}
			records = append(records, a)
		}
	case "MX":
		mxs, err := r.LookupMX(ctx, target)
		lookupErr = err
		for _, mx := range mxs {
			records = append(records, mx.Host)
		}
	case "CNAME":
		cname, err := r.LookupCNAME(ctx, target)
		lookupErr = err
		if err == nil {
			records = []string{cname}
		}
	case "TXT":
		txts, err := r.LookupTXT(ctx, target)
		lookupErr = err
		records = txts
	default:
		addrs, err := r.LookupHost(ctx, target)
		lookupErr = err
		records = addrs
	}

	latency := time.Since(start).Milliseconds()

	if lookupErr != nil {
		errStr := lookupErr.Error()
		details, _ := json.Marshal(dnsDetails{RecordType: recordType, LatencyMs: latency, Error: &errStr})
		return Result{Status: "down", Details: details, CheckedAt: now}
	}

	status := "up"
	if expectedValue != "" {
		found := false
		for _, rec := range records {
			if strings.Contains(rec, expectedValue) {
				found = true
				break
			}
		}
		if !found {
			status = "down"
		}
	}

	details, _ := json.Marshal(dnsDetails{RecordType: recordType, Records: records, LatencyMs: latency})
	return Result{Status: status, Details: details, CheckedAt: now}
}

// Run dispatches a check by type and returns the result.
// checkType must be one of "ping", "url", "ssl", "dns".
// expectedStatus is used for url checks; warnDays/critDays for ssl checks;
// dnsRecordType/dnsExpectedValue for dns checks.
// skipTLSVerify applies only to url checks.
func Run(ctx context.Context, checkType, target string, expectedStatus, warnDays, critDays int, skipTLSVerify bool, dnsRecordType, dnsExpectedValue string) (Result, error) {
	switch checkType {
	case "ping":
		return RunPing(ctx, target), nil
	case "url":
		return RunURL(ctx, target, expectedStatus, skipTLSVerify), nil
	case "ssl":
		return RunSSL(ctx, target, warnDays, critDays), nil
	case "dns":
		return RunDNS(ctx, target, dnsRecordType, dnsExpectedValue), nil
	default:
		return Result{}, fmt.Errorf("unknown check type: %s", checkType)
	}
}
