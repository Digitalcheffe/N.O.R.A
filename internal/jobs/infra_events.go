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
// infrastructure poll cycle. source is the poller name (e.g. "proxmox"),
// trigger is "scheduled" or "manual", status is "ok" or "failed", and detail
// carries an optional extra message (error text on failure, "" on success).
//
// source_type is set to "system" for all infra poll events. Future tasks will
// update pollers to emit more specific source_type values (e.g. "physical_host").
func emitInfraEvent(
	ctx context.Context,
	store *repo.Store,
	componentID, componentName, source, trigger, status, detail string,
) {
	level := "info"
	var title string

	if status == "ok" {
		title = fmt.Sprintf("[%s] %s poll completed (%s)", source, componentName, trigger)
	} else {
		level = "warn"
		if detail != "" {
			title = fmt.Sprintf("[%s] %s poll failed (%s): %s", source, componentName, trigger, detail)
		} else {
			title = fmt.Sprintf("[%s] %s poll failed (%s)", source, componentName, trigger)
		}
	}

	payload := fmt.Sprintf(
		`{"source":%q,"component_id":%q,"component_name":%q,"trigger":%q,"poll_status":%q}`,
		source, componentID, componentName, trigger, status,
	)

	event := &models.Event{
		ID:         uuid.New().String(),
		Level:      level,
		SourceName: componentName,
		SourceType: "system",
		SourceID:   componentID,
		Title:      title,
		Payload:    payload,
		CreatedAt:  time.Now().UTC(),
	}

	if err := store.Events.Create(ctx, event); err != nil {
		log.Printf("infra events: write poll event for %s (%s): %v", componentName, componentID, err)
	}
}
