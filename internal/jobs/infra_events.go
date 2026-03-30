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

// emitInfraEvent writes a diagnostic event to the event log for an
// infrastructure poll cycle. source is the poller name (e.g. "proxmox"),
// trigger is "scheduled" or "manual", status is "ok" or "failed", and detail
// carries an optional extra message (error text on failure, "" on success).
//
// Level is "debug" on success and "error" on failure.
// source_type is "physical_host" for all infra poll events.
func emitInfraEvent(
	ctx context.Context,
	store *repo.Store,
	componentID, componentName, source, trigger, status, detail string,
) {
	level := "debug"
	var title string

	if status == "ok" {
		title = fmt.Sprintf("%s poll completed — %s", source, componentName)
	} else {
		level = "error"
		if detail != "" {
			title = fmt.Sprintf("%s poll failed — %s: %s", source, componentName, detail)
		} else {
			title = fmt.Sprintf("%s poll failed — %s", source, componentName)
		}
	}

	payload := fmt.Sprintf(
		`{"source":%q,"component_id":%q,"component_name":%q,"trigger":%q,"poll_status":%q,"error":%q}`,
		source, componentID, componentName, trigger, status, detail,
	)

	event := &models.Event{
		ID:         uuid.New().String(),
		Level:      level,
		SourceName: componentName,
		SourceType: "physical_host",
		SourceID:   componentID,
		Title:      title,
		Payload:    payload,
		CreatedAt:  time.Now().UTC(),
	}

	if err := store.Events.Create(ctx, event); err != nil {
		log.Printf("infra events: write poll event for %s (%s): %v", componentName, componentID, err)
	}
}
