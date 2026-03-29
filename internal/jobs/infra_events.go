package jobs

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/google/uuid"
)

// emitInfraEvent writes a single informational event to the event log for an
// infrastructure poll cycle.  source is the poller name (e.g. "proxmox"),
// trigger is "scheduled" or "manual", status is "ok" or "failed", and detail
// carries an optional extra message (error text on failure, "" on success).
//
// The event is written with AppID="" so it is not associated with any
// application — it appears in the global event stream as an infra event.
func emitInfraEvent(
	ctx context.Context,
	store *repo.Store,
	componentID, componentName, source, trigger, status, detail string,
) {
	severity := "info"
	var displayText string

	if status == "ok" {
		displayText = fmt.Sprintf("[%s] %s poll completed (%s)", source, componentName, trigger)
	} else {
		severity = "warn"
		if detail != "" {
			displayText = fmt.Sprintf("[%s] %s poll failed (%s): %s", source, componentName, trigger, detail)
		} else {
			displayText = fmt.Sprintf("[%s] %s poll failed (%s)", source, componentName, trigger)
		}
	}

	fields := fmt.Sprintf(
		`{"source":%q,"component_id":%q,"component_name":%q,"trigger":%q,"poll_status":%q}`,
		source, componentID, componentName, trigger, status,
	)

	event := &models.Event{
		ID:          uuid.New().String(),
		AppID:       "",
		ReceivedAt:  time.Now().UTC(),
		Severity:    severity,
		DisplayText: displayText,
		RawPayload:  "{}",
		Fields:      fields,
	}

	if err := store.Events.Create(ctx, event); err != nil {
		log.Printf("infra events: write poll event for %s (%s): %v", componentName, componentID, err)
	}
}
