package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/digitalcheffe/nora/internal/infra"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/internal/scanner"
)

// SynologyDiscoveryScanner discovers storage pools, volumes, and disks for
// a Synology DSM infrastructure component.
type SynologyDiscoveryScanner struct {
	store *repo.Store
}

// NewSynologyDiscoveryScanner returns a SynologyDiscoveryScanner backed by store.
func NewSynologyDiscoveryScanner(store *repo.Store) *SynologyDiscoveryScanner {
	return &SynologyDiscoveryScanner{store: store}
}

// Discover runs a full poll of the Synology DSM and reports the current count
// of volumes and disks found.  It compares against the previously stored
// synology_meta to detect changes and writes appropriate discovery events.
func (s *SynologyDiscoveryScanner) Discover(ctx context.Context, entityID string, entityType string) (*scanner.DiscoveryResult, error) {
	c, err := s.store.InfraComponents.Get(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("get component %s: %w", entityID, err)
	}
	if c.Credentials == nil || *c.Credentials == "" {
		return nil, fmt.Errorf("no credentials configured for %s", c.Name)
	}

	// Read previous meta before polling so we can detect first-run vs changes.
	var prevVolumes, prevDisks int
	if c.Meta != nil && *c.Meta != "" {
		var prev infra.SynologyMeta
		if err := json.Unmarshal([]byte(*c.Meta), &prev); err == nil {
			prevVolumes = len(prev.Volumes)
			prevDisks = len(prev.Disks)
		}
	}

	poller, err := infra.NewSynologyPoller(c.ID, *c.Credentials)
	if err != nil {
		return nil, fmt.Errorf("create synology poller: %w", err)
	}
	if err := poller.Poll(ctx, s.store); err != nil {
		return nil, fmt.Errorf("poll failed: %w", err)
	}

	// Re-read the updated meta.
	updated, err := s.store.InfraComponents.Get(ctx, entityID)
	if err != nil {
		log.Printf("synology discovery: re-read component %s: %v", entityID, err)
		// Non-fatal — we still polled successfully.
		writeDiscoveryEvent(ctx, s.store, entityID, c.Name, "physical_host", "debug",
			fmt.Sprintf("[discovery] %s discovery completed — no changes", c.Name))
		return &scanner.DiscoveryResult{EntityID: entityID, EntityType: entityType}, nil
	}

	if updated.Meta == nil || *updated.Meta == "" {
		writeDiscoveryEvent(ctx, s.store, entityID, c.Name, "physical_host", "debug",
			fmt.Sprintf("[discovery] %s discovery completed — no changes", c.Name))
		return &scanner.DiscoveryResult{EntityID: entityID, EntityType: entityType}, nil
	}

	var meta infra.SynologyMeta
	if err := json.Unmarshal([]byte(*updated.Meta), &meta); err != nil {
		return nil, fmt.Errorf("parse synology meta: %w", err)
	}

	curVolumes := len(meta.Volumes)
	curDisks := len(meta.Disks)
	found := 0
	disappeared := 0

	// Compare volumes.
	if curVolumes > prevVolumes {
		diff := curVolumes - prevVolumes
		found += diff
		writeDiscoveryEvent(ctx, s.store, entityID, c.Name, "physical_host", "info",
			fmt.Sprintf("[discovery] %s: %d new volume(s) discovered (%d total)", c.Name, diff, curVolumes))
	} else if curVolumes < prevVolumes {
		diff := prevVolumes - curVolumes
		disappeared += diff
		writeDiscoveryEvent(ctx, s.store, entityID, c.Name, "physical_host", "warn",
			fmt.Sprintf("[discovery] Entity no longer found: %d volume(s) missing on %s (%d total)", diff, c.Name, curVolumes))
	}

	// Compare disks.
	if curDisks > prevDisks {
		diff := curDisks - prevDisks
		found += diff
		writeDiscoveryEvent(ctx, s.store, entityID, c.Name, "physical_host", "info",
			fmt.Sprintf("[discovery] %s: %d new disk(s) discovered (%d total)", c.Name, diff, curDisks))
	} else if curDisks < prevDisks {
		diff := prevDisks - curDisks
		disappeared += diff
		writeDiscoveryEvent(ctx, s.store, entityID, c.Name, "physical_host", "warn",
			fmt.Sprintf("[discovery] Entity no longer found: %d disk(s) missing on %s (%d total)", diff, c.Name, curDisks))
	}

	if found == 0 && disappeared == 0 {
		writeDiscoveryEvent(ctx, s.store, entityID, c.Name, "physical_host", "debug",
			fmt.Sprintf("[discovery] %s discovery completed — no changes (%d volumes, %d disks)",
				c.Name, curVolumes, curDisks))
	}

	return &scanner.DiscoveryResult{
		EntityID:    entityID,
		EntityType:  entityType,
		Found:       found,
		Disappeared: disappeared,
	}, nil
}

// compile-time check.
var _ scanner.DiscoveryScanner = (*SynologyDiscoveryScanner)(nil)
