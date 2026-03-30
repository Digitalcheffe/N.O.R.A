// Package discovery provides DiscoveryScanner implementations for each
// infrastructure integration type supported by NORA.
//
// Each scanner is constructed with a *repo.Store and registered with the
// scanner.ScanScheduler in main.go.  The same implementations are also used
// by the Discover Now API endpoint (POST /infrastructure/{id}/discover).
package discovery

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/google/uuid"
)

// writeDiscoveryEvent writes a single event to the event log for a discovery
// action.  level must be one of "debug", "info", "warn", "error".
// sourceType should match the models.Event source_type convention.
func writeDiscoveryEvent(
	ctx context.Context,
	store *repo.Store,
	componentID, componentName, sourceType, level, title string,
) {
	payload := fmt.Sprintf(
		`{"bucket":"discovery","component_id":%q,"component_name":%q}`,
		componentID, componentName,
	)
	ev := &models.Event{
		ID:         uuid.New().String(),
		Level:      level,
		SourceName: componentName,
		SourceType: sourceType,
		SourceID:   componentID,
		Title:      title,
		Payload:    payload,
		CreatedAt:  time.Now().UTC(),
	}
	if err := store.Events.Create(ctx, ev); err != nil {
		log.Printf("discovery: write event for %s (%s): %v", componentName, componentID, err)
	}
}
