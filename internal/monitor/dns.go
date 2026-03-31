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

// DNSChecker executes DNS resolution checks and persists results via the store.
type DNSChecker struct {
	store   *repo.Store
	resolve func(ctx context.Context, target, recordType, expectedValue string) Result
}

// NewDNSChecker returns a DNSChecker backed by store.
func NewDNSChecker(store *repo.Store) *DNSChecker {
	return &DNSChecker{store: store, resolve: RunDNS}
}

// Run executes a DNS check for the given MonitorCheck and persists the result.
// If no baseline is stored yet, the first successful result is captured as the
// baseline so future runs can detect changes. On a status transition an event is created.
func (d *DNSChecker) Run(ctx context.Context, check *models.MonitorCheck) error {
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	result := d.resolve(runCtx, check.Target, check.DNSRecordType, check.DNSExpectedValue)
	cancel()

	now := time.Now().UTC()

	// If no baseline is stored yet and this run resolved successfully, capture it now.
	if check.DNSExpectedValue == "" && result.Status == "up" {
		var det struct {
			Records []string `json:"records"`
		}
		if json.Unmarshal(result.Details, &det) == nil && len(det.Records) > 0 {
			baseline := det.Records[0]
			if err := d.store.Checks.SetDNSBaseline(ctx, check.ID, baseline); err != nil {
				log.Printf("dns checker: set baseline for %s: %v", check.ID, err)
			} else {
				check.DNSExpectedValue = baseline
				log.Printf("dns checker: baseline captured for %q — %s", check.Name, baseline)
			}
		}
	}

	prevStatus := check.LastStatus
	if prevStatus != "" && prevStatus != result.Status {
		if err := d.createStatusEvent(ctx, check, result, now); err != nil {
			log.Printf("dns checker: create event for check %s: %v", check.ID, err)
		}
	}

	detailsStr := string(result.Details)
	if err := d.store.Checks.UpdateStatus(ctx, check.ID, result.Status, detailsStr, now); err != nil {
		return fmt.Errorf("dns checker: update status for %s: %w", check.ID, err)
	}
	return nil
}

func (d *DNSChecker) createStatusEvent(ctx context.Context, check *models.MonitorCheck, result Result, now time.Time) error {
	level := "info"
	var displayText string
	if result.Status == "down" {
		level = "error"
		displayText = fmt.Sprintf("DNS check failed — %s (%s)", check.Name, check.Target)
	} else {
		displayText = fmt.Sprintf("DNS check restored — %s (%s)", check.Name, check.Target)
	}

	var det dnsDetails
	_ = json.Unmarshal(result.Details, &det)

	payload := fmt.Sprintf(`{"type":"dns","target":%q,"record_type":%q,"latency_ms":%d}`,
		check.Target, det.RecordType, det.LatencyMs)

	event := &models.Event{
		ID:         uuid.New().String(),
		Level:      level,
		SourceName: check.Name,
		SourceType: "monitor_check",
		SourceID:   check.ID,
		Title:      displayText,
		Payload:    payload,
		CreatedAt:  now,
	}
	return d.store.Events.Create(ctx, event)
}
