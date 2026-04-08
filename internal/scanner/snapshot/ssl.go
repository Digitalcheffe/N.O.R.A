package snapshot

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/digitalcheffe/nora/internal/repo"
)

// SSLSnapshotJob captures SSL certificate expiry conditions for all enabled
// SSL monitor checks every SnapshotInterval by making a direct TLS dial.
// It fires events only on condition changes (ok/warn/error/critical bucket
// transitions), not on every status change.
type SSLSnapshotJob struct {
	store *repo.Store
}

// NewSSLSnapshotJob returns an SSLSnapshotJob backed by store.
func NewSSLSnapshotJob(store *repo.Store) *SSLSnapshotJob {
	return &SSLSnapshotJob{store: store}
}

// Run executes one SSL snapshot pass across all enabled SSL checks.
func (j *SSLSnapshotJob) Run(ctx context.Context) {
	checks, err := j.store.Checks.List(ctx)
	if err != nil {
		log.Printf("ssl snapshot: list checks: %v", err)
		return
	}

	now := time.Now().UTC()
	for i := range checks {
		c := &checks[i]
		if !c.Enabled || c.Type != "ssl" {
			continue
		}
		j.snapshotCheck(ctx, c.ID, c.Name, c.Target, now)
	}
}

// snapshotCheck snapshots a single SSL check by dialing the target over TLS.
func (j *SSLSnapshotJob) snapshotCheck(
	ctx context.Context,
	checkID, checkName, target string,
	now time.Time,
) {
	daysRemaining, issuer, subject, err := dialSSL(ctx, target)
	if err != nil {
		log.Printf("ssl snapshot: dial %s: %v", target, err)
		return
	}

	// Snapshot the raw days remaining value.
	daysStr := fmt.Sprintf("%d", daysRemaining)
	captureSnapshot(ctx, j.store, "monitor_check", checkID, "ssl_days_remaining", daysStr, now)

	// Snapshot and event-check the condition bucket.
	newCond := sslCondition(daysRemaining)
	prevCond, changed := captureSnapshot(ctx, j.store, "monitor_check", checkID, "ssl_condition", newCond, now)

	// Snapshot issuer and subject (no events — informational only).
	captureSnapshot(ctx, j.store, "monitor_check", checkID, "ssl_issuer", issuer, now)
	captureSnapshot(ctx, j.store, "monitor_check", checkID, "ssl_subject", subject, now)

	if changed {
		level, title := sslEventTitle(checkName, newCond, prevCond, daysRemaining)
		writeSnapshotEvent(ctx, j.store, checkID, checkName, "monitor_check", level, title)
	}

	writeDebugEvent(ctx, j.store, checkID, checkName, "monitor_check")
}

// dialSSL opens a TLS connection to target and returns days remaining,
// issuer, and subject common name.
func dialSSL(ctx context.Context, target string) (daysRemaining int, issuer, subject string, err error) {
	host := target
	for _, prefix := range []string{"https://", "http://"} {
		host = strings.TrimPrefix(host, prefix)
	}
	if i := strings.Index(host, "/"); i != -1 {
		host = host[:i]
	}
	if !strings.Contains(host, ":") {
		host = host + ":443"
	}

	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: 10 * time.Second},
		Config:    &tls.Config{InsecureSkipVerify: false}, //nolint:gosec
	}
	conn, dialErr := dialer.DialContext(ctx, "tcp", host)
	if dialErr != nil {
		return 0, "", "", fmt.Errorf("tls dial: %w", dialErr)
	}
	defer conn.Close() //nolint:errcheck

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return 0, "", "", fmt.Errorf("not a TLS connection")
	}
	certs := tlsConn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return 0, "", "", fmt.Errorf("no peer certificates")
	}
	cert := certs[0]
	days := int(time.Until(cert.NotAfter.UTC()).Hours() / 24)
	return days, cert.Issuer.CommonName, cert.Subject.CommonName, nil
}

// sslEventTitle returns the level and title for an SSL condition change event.
func sslEventTitle(checkName, newCond, prevCond string, days int) (level, title string) {
	// Renewal: condition improved (e.g. error → ok, or days jumped back up significantly).
	if condLevel(newCond) < condLevel(prevCond) {
		return "info", fmt.Sprintf("[snapshot] SSL renewed — %s: %d days remaining", checkName, days)
	}
	switch newCond {
	case "critical":
		return "critical", fmt.Sprintf("[snapshot] SSL expired — %s", checkName)
	case "error":
		return "error", fmt.Sprintf("[snapshot] SSL expiry critical — %s: %d days remaining", checkName, days)
	case "warn":
		return "warn", fmt.Sprintf("[snapshot] SSL expiring soon — %s: %d days remaining", checkName, days)
	default:
		return "info", fmt.Sprintf("[snapshot] SSL renewed — %s: %d days remaining", checkName, days)
	}
}

// condLevel maps a condition string to an integer for comparison.
func condLevel(cond string) int {
	switch cond {
	case "critical":
		return 4
	case "error":
		return 3
	case "warn":
		return 2
	case "ok":
		return 1
	default:
		return 0
	}
}

// SSLSnapshotJob does not implement scanner.SnapshotScanner intentionally —
// it runs as a standalone job because it iterates monitor_checks rather than
// infrastructure_components. It is started from main.go on a 30-minute ticker.
