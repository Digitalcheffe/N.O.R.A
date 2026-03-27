package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/google/uuid"
)

// PingChecker executes ping health checks and persists results via the store.
//
// Implementation note: we use os/exec to invoke the system ping binary rather
// than raw ICMP sockets. Raw ICMP requires CAP_NET_RAW (root or a specific
// capability), which is not available in unprivileged containers. The trade-off
// is a dependency on the ping binary being present in the image — Alpine
// provides it via busybox. If ICMP is unavailable, consider using a TCP
// connect approach instead (e.g. net.DialTimeout to port 80/443).
type PingChecker struct {
	store  *repo.Store
	pinger func(ctx context.Context, target string) Result // injectable for tests
}

// NewPingChecker returns a PingChecker backed by store.
func NewPingChecker(store *repo.Store) *PingChecker {
	return &PingChecker{store: store, pinger: RunPing}
}

// Run executes a ping health check for the given MonitorCheck.
//
// It sends up to 3 pings with a 5-second timeout each. The check is considered
// down only if all 3 pings fail, reducing false positives from transient packet
// loss. On a status transition (up→down or down→up), an event is created —
// but only when the check is associated with an app, because the events table
// requires a valid app_id foreign key.
func (p *PingChecker) Run(ctx context.Context, check *models.MonitorCheck) error {
	const attempts = 3

	downCount := 0
	var latencyMs int64

	for i := 0; i < attempts; i++ {
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		result := p.pinger(pingCtx, check.Target)
		cancel()

		if result.Status == "down" {
			downCount++
		} else {
			var d pingDetails
			_ = json.Unmarshal(result.Details, &d)
			latencyMs = d.LatencyMs
		}
	}

	now := time.Now().UTC()

	newStatus := "up"
	var detailsBytes []byte
	if downCount == attempts {
		newStatus = "down"
		detailsBytes, _ = json.Marshal(pingDetails{})
	} else {
		detailsBytes, _ = json.Marshal(pingDetails{LatencyMs: latencyMs})
	}

	// Emit a status-change event when there is a known previous state and the
	// check is linked to an app. Checks without an app_id are tracked in
	// last_status only — the events table requires a valid app_id reference.
	prevStatus := check.LastStatus
	if prevStatus != "" && prevStatus != newStatus && check.AppID != "" {
		if err := p.createStatusEvent(ctx, check, newStatus, now); err != nil {
			log.Printf("ping checker: create event for check %s: %v", check.ID, err)
		}
	}

	if err := p.store.Checks.UpdateStatus(ctx, check.ID, newStatus, string(detailsBytes), now); err != nil {
		return fmt.Errorf("ping checker: update status for %s: %w", check.ID, err)
	}
	return nil
}

// createStatusEvent persists a down or recovery event for a check.
func (p *PingChecker) createStatusEvent(ctx context.Context, check *models.MonitorCheck, newStatus string, now time.Time) error {
	var severity, displayText string
	if newStatus == "down" {
		severity = "error"
		displayText = fmt.Sprintf("Ping failed — %s (%s)", check.Name, check.Target)
	} else {
		severity = "info"
		displayText = fmt.Sprintf("Ping restored — %s (%s)", check.Name, check.Target)
	}

	event := &models.Event{
		ID:          uuid.New().String(),
		AppID:       check.AppID,
		ReceivedAt:  now,
		Severity:    severity,
		DisplayText: displayText,
		RawPayload:  "{}",
		Fields:      `{"source":"monitor","check_id":"` + check.ID + `","type":"ping"}`,
	}
	return p.store.Events.Create(ctx, event)
}
