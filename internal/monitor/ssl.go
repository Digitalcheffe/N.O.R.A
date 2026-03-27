package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/google/uuid"
)

// SSLChecker executes TLS certificate expiry checks and persists results via the store.
type SSLChecker struct {
	store  *repo.Store
	runner func(ctx context.Context, target string, warnDays, critDays int) Result
}

// NewSSLChecker returns an SSLChecker backed by store.
func NewSSLChecker(store *repo.Store) *SSLChecker {
	return &SSLChecker{store: store, runner: RunSSL}
}

// Run executes one SSL certificate expiry check cycle for check.
//
// It dials the target over TLS, reads the leaf certificate, and maps days
// remaining to one of four statuses: up / warn / critical / down.
// On a status transition, an event is created — but only when the check is
// linked to an app (events require a valid app_id).
func (s *SSLChecker) Run(ctx context.Context, check *models.MonitorCheck) error {
	warnDays := check.SSLWarnDays
	if warnDays == 0 {
		warnDays = 30
	}
	critDays := check.SSLCritDays
	if critDays == 0 {
		critDays = 7
	}

	result := s.runner(ctx, check.Target, warnDays, critDays)
	now := time.Now().UTC()

	// Parse days remaining from result details for use in event messages.
	var parsed sslDetails
	_ = json.Unmarshal(result.Details, &parsed)

	days := 0
	if parsed.DaysRemaining != nil {
		days = *parsed.DaysRemaining
	}

	domain := extractDomain(check.Target)

	prevStatus := check.LastStatus
	newStatus := result.Status
	if prevStatus != "" && prevStatus != newStatus && check.AppID != "" {
		if evErr := s.createStatusEvent(ctx, check, newStatus, domain, days, now); evErr != nil {
			log.Printf("ssl checker: create event for check %s: %v", check.ID, evErr)
		}
	}

	if updateErr := s.store.Checks.UpdateStatus(ctx, check.ID, newStatus, string(result.Details), now); updateErr != nil {
		return fmt.Errorf("ssl checker: update status for %s: %w", check.ID, updateErr)
	}
	return nil
}

// extractDomain strips the scheme and path from a URL, returning host[:port].
func extractDomain(target string) string {
	d := target
	for _, prefix := range []string{"https://", "http://"} {
		d = strings.TrimPrefix(d, prefix)
	}
	if i := strings.Index(d, "/"); i != -1 {
		d = d[:i]
	}
	return d
}

// createStatusEvent persists a status-change event for an SSL check.
func (s *SSLChecker) createStatusEvent(
	ctx context.Context,
	check *models.MonitorCheck,
	newStatus, domain string,
	daysRemaining int,
	now time.Time,
) error {
	var severity, displayText string
	switch newStatus {
	case "warn":
		severity = "warn"
		displayText = fmt.Sprintf("SSL expiring soon — %s: %d days remaining", domain, daysRemaining)
	case "critical":
		severity = "critical"
		displayText = fmt.Sprintf("SSL expiry critical — %s: %d days remaining", domain, daysRemaining)
	case "down":
		severity = "error"
		displayText = fmt.Sprintf("SSL invalid or expired — %s", domain)
	default: // "up" = recovery
		severity = "info"
		displayText = fmt.Sprintf("SSL renewed — %s: %d days remaining", domain, daysRemaining)
	}

	event := &models.Event{
		ID:          uuid.New().String(),
		AppID:       check.AppID,
		ReceivedAt:  now,
		Severity:    severity,
		DisplayText: displayText,
		RawPayload:  "{}",
		Fields:      `{"source":"monitor","check_id":"` + check.ID + `","type":"ssl"}`,
	}
	return s.store.Events.Create(ctx, event)
}
