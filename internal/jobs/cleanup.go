package jobs

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/digitalcheffe/nora/internal/repo"
)

// cleanupItemCap is the maximum number of rows surfaced in a preview response.
// The preview modal only needs enough detail for the user to recognize what
// will be deleted — if a user has more than a hundred stale rows, truncating
// protects both the API payload and the rendered DOM. Count is still the real
// total so the confirm label stays honest.
const cleanupItemCap = 200

// PreviewInactiveRegistry returns a CleanupPreview listing every
// digest_registry row with active=0. Used by the "Clean Stale Registry
// Entries" job's PreviewFn.
func PreviewInactiveRegistry(ctx context.Context, store *repo.Store) (CleanupPreview, error) {
	entries, err := store.DigestRegistry.ListInactive(ctx)
	if err != nil {
		return CleanupPreview{}, fmt.Errorf("list inactive registry: %w", err)
	}
	out := CleanupPreview{Count: len(entries)}
	limit := len(entries)
	if limit > cleanupItemCap {
		limit = cleanupItemCap
	}
	for _, e := range entries[:limit] {
		label := e.Label
		if label == "" {
			label = e.Name
		}
		out.Items = append(out.Items, CleanupPreviewItem{
			ID:    e.ID,
			Label: label,
			Sub:   fmt.Sprintf("%s · %s", e.ProfileID, e.EntryType),
		})
	}
	return out, nil
}

// RunCleanupInactiveRegistry hard-deletes every digest_registry row with
// active=0 and returns the number of rows removed. Designed to be bound to
// a JobEntry.RunFn via a closure in main.go.
func RunCleanupInactiveRegistry(ctx context.Context, store *repo.Store) error {
	n, err := store.DigestRegistry.DeleteAllInactive(ctx)
	if err != nil {
		return fmt.Errorf("cleanup inactive registry: %w", err)
	}
	return logCleanupResult(ctx, "digest registry", n)
}

// PreviewStoppedContainers returns a CleanupPreview listing every
// discovered_containers row whose status is not "running". Used by the
// "Clean Stopped Containers" job's PreviewFn.
func PreviewStoppedContainers(ctx context.Context, store *repo.Store) (CleanupPreview, error) {
	rows, err := store.DiscoveredContainers.ListStoppedContainers(ctx)
	if err != nil {
		return CleanupPreview{}, fmt.Errorf("list stopped containers: %w", err)
	}
	out := CleanupPreview{Count: len(rows)}
	limit := len(rows)
	if limit > cleanupItemCap {
		limit = cleanupItemCap
	}
	for _, c := range rows[:limit] {
		sub := c.Image
		if !c.LastSeenAt.IsZero() {
			sub = fmt.Sprintf("%s · last seen %s", c.Image, c.LastSeenAt.UTC().Format(time.RFC3339))
		}
		out.Items = append(out.Items, CleanupPreviewItem{
			ID:    c.ID,
			Label: c.ContainerName,
			Sub:   sub,
		})
	}
	return out, nil
}

// RunCleanupStoppedContainers hard-deletes every discovered_containers row
// whose status is not "running".
func RunCleanupStoppedContainers(ctx context.Context, store *repo.Store) error {
	n, err := store.DiscoveredContainers.DeleteAllStoppedContainers(ctx)
	if err != nil {
		return fmt.Errorf("cleanup stopped containers: %w", err)
	}
	return logCleanupResult(ctx, "stopped containers", n)
}

// logCleanupResult is a tiny helper so the two cleanup runners have a
// consistent log line shape. Errors are never returned here — the delete
// already succeeded.
func logCleanupResult(_ context.Context, label string, n int64) error {
	log.Printf("cleanup %s: %d rows removed", label, n)
	return nil
}
