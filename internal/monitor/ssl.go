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
// When check.SSLSource is "traefik", expiry is read from the traefik_certs
// cache — no outbound TLS connection is made. Otherwise the existing standalone
// mode dials the target and reads the cert off the TLS handshake.
//
// On a status transition an event is always created. app_id is optional.
func (s *SSLChecker) Run(ctx context.Context, check *models.MonitorCheck) error {
	if check.SSLSource != nil && *check.SSLSource == "traefik" {
		return s.runTraefikSSL(ctx, check)
	}
	return s.runStandaloneSSL(ctx, check)
}

// runTraefikSSL reads cert expiry from the traefik_certs cache.
// No network connection is made.
func (s *SSLChecker) runTraefikSSL(ctx context.Context, check *models.MonitorCheck) error {
	warnDays := check.SSLWarnDays
	if warnDays == 0 {
		warnDays = 30
	}
	critDays := check.SSLCritDays
	if critDays == 0 {
		critDays = 7
	}

	now := time.Now().UTC()
	domain := check.Target // Traefik checks store the bare domain as target

	cert, err := s.store.Infra.GetCertByDomain(ctx, domain)
	if err != nil {
		errStr := fmt.Sprintf("cert not found in Traefik cache for domain %q", domain)
		if err != repo.ErrNotFound {
			errStr = err.Error()
		}
		details, _ := json.Marshal(sslDetails{Error: &errStr})
		if updateErr := s.store.Checks.UpdateStatus(ctx, check.ID, "down", string(details), now); updateErr != nil {
			return fmt.Errorf("ssl checker (traefik): update status for %s: %w", check.ID, updateErr)
		}
		return nil
	}

	if cert.ExpiresAt == nil {
		errStr := "no expiry date in Traefik cert cache"
		details, _ := json.Marshal(sslDetails{Error: &errStr})
		if updateErr := s.store.Checks.UpdateStatus(ctx, check.ID, "down", string(details), now); updateErr != nil {
			return fmt.Errorf("ssl checker (traefik): update status for %s: %w", check.ID, updateErr)
		}
		return nil
	}

	daysRemaining := int(time.Until(*cert.ExpiresAt).Hours() / 24)

	var issuerStr, subjectStr string
	if cert.Issuer != nil {
		issuerStr = *cert.Issuer
	}
	subjectStr = cert.Domain
	expiresAt := *cert.ExpiresAt

	details, _ := json.Marshal(sslDetails{
		DaysRemaining: &daysRemaining,
		ExpiresAt:     &expiresAt,
		Issuer:        &issuerStr,
		Subject:       &subjectStr,
	})

	newStatus := "up"
	switch {
	case daysRemaining <= 0:
		newStatus = "down"
	case daysRemaining <= critDays:
		newStatus = "critical"
	case daysRemaining <= warnDays:
		newStatus = "warn"
	}

	prevStatus := check.LastStatus
	if prevStatus != "" && prevStatus != newStatus {
		if evErr := s.createStatusEvent(ctx, check, newStatus, domain, daysRemaining, now); evErr != nil {
			log.Printf("ssl checker (traefik): create event for check %s: %v", check.ID, evErr)
		}
	}

	if updateErr := s.store.Checks.UpdateStatus(ctx, check.ID, newStatus, string(details), now); updateErr != nil {
		return fmt.Errorf("ssl checker (traefik): update status for %s: %w", check.ID, updateErr)
	}
	return nil
}

// runStandaloneSSL dials the target over TLS and reads the leaf certificate.
// This is the pre-T-34 behaviour, unchanged.
func (s *SSLChecker) runStandaloneSSL(ctx context.Context, check *models.MonitorCheck) error {
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

	var parsed sslDetails
	_ = json.Unmarshal(result.Details, &parsed)

	days := 0
	if parsed.DaysRemaining != nil {
		days = *parsed.DaysRemaining
	}

	domain := extractDomain(check.Target)
	prevStatus := check.LastStatus
	newStatus := result.Status

	if prevStatus != "" && prevStatus != newStatus {
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
		ID:         uuid.New().String(),
		Level:      severity,
		SourceName: check.Name,
		SourceType: "monitor_check",
		SourceID:   check.ID,
		Title:      displayText,
		Payload:    `{"type":"ssl"}`,
		CreatedAt:  now,
	}
	return s.store.Events.Create(ctx, event)
}
