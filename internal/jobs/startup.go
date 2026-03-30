package jobs

import (
	"context"
	"log"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/google/uuid"
)

// EmitStartupEvent writes a single info event to the event log recording that
// NORA started successfully. source_type is "system", source_id is NULL.
func EmitStartupEvent(ctx context.Context, store *repo.Store) {
	ev := &models.Event{
		ID:         uuid.New().String(),
		Level:      "info",
		SourceName: "NORA System",
		SourceType: "system",
		SourceID:   "",
		Title:      "NORA started",
		Payload:    `{"event":"startup"}`,
		CreatedAt:  time.Now().UTC(),
	}
	if err := store.Events.Create(ctx, ev); err != nil {
		log.Printf("startup event: %v", err)
	}
}
