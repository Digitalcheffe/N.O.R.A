package snapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/digitalcheffe/nora/internal/infra"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/internal/scanner"
)

// SynologySnapshotScanner captures volume utilisation, disk health, DSM version,
// and available update state for a Synology NAS every SnapshotInterval.
type SynologySnapshotScanner struct {
	store   *repo.Store
	pollers sync.Map // componentID → *infra.SynologyPoller
}

// NewSynologySnapshotScanner returns a SynologySnapshotScanner backed by store.
func NewSynologySnapshotScanner(store *repo.Store) *SynologySnapshotScanner {
	return &SynologySnapshotScanner{store: store}
}

// TakeSnapshot fetches Synology condition data and writes snapshot rows. Events
// are fired on condition changes only.
func (s *SynologySnapshotScanner) TakeSnapshot(ctx context.Context, entityID string, entityType string) (*scanner.SnapshotResult, error) {
	c, err := s.store.InfraComponents.Get(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("get component %s: %w", entityID, err)
	}
	if c.Credentials == nil || *c.Credentials == "" {
		return nil, fmt.Errorf("no credentials configured for %s", c.Name)
	}

	// Prefer the cached SynologyMeta (written by the legacy poller) to avoid
	// an extra API call when the poller is already running.
	if c.Meta != nil && *c.Meta != "" {
		return s.snapshotFromMeta(ctx, entityID, entityType, c.Name, *c.Meta)
	}

	// No cached meta: fall back to a direct API call.
	poller, err := s.getOrCreatePoller(entityID, *c.Credentials)
	if err != nil {
		return nil, fmt.Errorf("create synology poller: %w", err)
	}
	snap, err := poller.CollectMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("collect synology metrics: %w", err)
	}

	now := time.Now().UTC()
	changed := false

	// Volume utilisation
	for _, vol := range snap.VolumeStats {
		pctStr := fmt.Sprintf("%.2f", vol.DiskPercent)
		_, ch := captureSnapshot(ctx, s.store, "physical_host", entityID,
			"volume_pct_"+vol.Key, pctStr, now)
		if ch {
			changed = true
			newCond := storageCondition(vol.DiskPercent)
			if newCond != "ok" {
				level, title := storageEventTitle(c.Name, vol.Key, newCond, vol.DiskPercent)
				writeSnapshotEvent(ctx, s.store, entityID, c.Name, "physical_host", level, title)
			}
		}
	}

	writeDebugEvent(ctx, s.store, entityID, c.Name, "physical_host")
	log.Printf("synology snapshot: %s: done via direct API (changed=%v)", c.Name, changed)

	return &scanner.SnapshotResult{
		EntityID:   entityID,
		EntityType: entityType,
		Changed:    changed,
	}, nil
}

// snapshotFromMeta uses the cached SynologyMeta JSON (written every 5 minutes
// by the legacy poller) so no extra API call is needed for the snapshot pass.
func (s *SynologySnapshotScanner) snapshotFromMeta(
	ctx context.Context,
	entityID, entityType, componentName, metaJSON string,
) (*scanner.SnapshotResult, error) {
	var meta infra.SynologyMeta
	if err := json.Unmarshal([]byte(metaJSON), &meta); err != nil {
		return nil, fmt.Errorf("parse synology_meta: %w", err)
	}

	now := time.Now().UTC()
	changed := false

	// ── Volume utilisation ─────────────────────────────────────────────────────
	for _, vol := range meta.Volumes {
		pctStr := fmt.Sprintf("%.2f", vol.Percent)
		_, ch := captureSnapshot(ctx, s.store, "physical_host", entityID,
			"volume_pct_"+sanitiseVolPath(vol.Path), pctStr, now)
		if ch {
			changed = true
			newCond := storageCondition(vol.Percent)
			if newCond != "ok" {
				level, title := storageEventTitle(componentName, vol.Path, newCond, vol.Percent)
				writeSnapshotEvent(ctx, s.store, entityID, componentName, "physical_host", level, title)
			}
		}

		captureSnapshot(ctx, s.store, "physical_host", entityID,
			"volume_used_bytes_"+sanitiseVolPath(vol.Path),
			fmt.Sprintf("%d", vol.UsedBytes), now)
		captureSnapshot(ctx, s.store, "physical_host", entityID,
			"volume_total_bytes_"+sanitiseVolPath(vol.Path),
			fmt.Sprintf("%d", vol.TotalBytes), now)
	}

	// ── Disk health ────────────────────────────────────────────────────────────
	for _, disk := range meta.Disks {
		slotKey := fmt.Sprintf("disk%d", disk.Slot)
		_, ch := captureSnapshot(ctx, s.store, "physical_host", entityID,
			"disk_health_"+slotKey, disk.Status, now)
		if ch {
			changed = true
			newCond := diskHealthCondition(disk.Status)
			if newCond != "ok" {
				level := "warn"
				if newCond == "error" {
					level = "error"
				}
				writeSnapshotEvent(ctx, s.store, entityID, componentName, "physical_host", level,
					fmt.Sprintf("[snapshot] Disk health %s — %s slot %d (%s): %s",
						disk.Status, componentName, disk.Slot, disk.Model, disk.Status))
			}
		}
	}

	// ── DSM version ────────────────────────────────────────────────────────────
	if meta.DSMVersion != "" {
		_, ch := captureSnapshot(ctx, s.store, "physical_host", entityID,
			"dsm_version", meta.DSMVersion, now)
		if ch {
			changed = true
			writeSnapshotEvent(ctx, s.store, entityID, componentName, "physical_host", "info",
				fmt.Sprintf("[snapshot] DSM updated — %s: %s", componentName, meta.DSMVersion))
		}
	}

	// ── Available updates ──────────────────────────────────────────────────────
	updateVal := "false"
	if meta.Update.Available && meta.Update.Version != "" {
		updateVal = meta.Update.Version
	}
	prevUpd, ch := captureSnapshot(ctx, s.store, "physical_host", entityID,
		"update_available", updateVal, now)
	if ch {
		changed = true
		if meta.Update.Available && prevUpd == "false" {
			writeSnapshotEvent(ctx, s.store, entityID, componentName, "physical_host", "info",
				fmt.Sprintf("[snapshot] DSM update available — %s: %s", componentName, meta.Update.Version))
		} else if !meta.Update.Available && prevUpd != "false" {
			writeSnapshotEvent(ctx, s.store, entityID, componentName, "physical_host", "info",
				fmt.Sprintf("[snapshot] DSM update applied — %s", componentName))
		}
	}

	writeDebugEvent(ctx, s.store, entityID, componentName, "physical_host")
	log.Printf("synology snapshot: %s: done from meta cache (changed=%v)", componentName, changed)

	return &scanner.SnapshotResult{
		EntityID:   entityID,
		EntityType: entityType,
		Changed:    changed,
	}, nil
}

func (s *SynologySnapshotScanner) getOrCreatePoller(componentID, credJSON string) (*infra.SynologyPoller, error) {
	if v, ok := s.pollers.Load(componentID); ok {
		return v.(*infra.SynologyPoller), nil
	}
	p, err := infra.NewSynologyPoller(componentID, credJSON)
	if err != nil {
		return nil, err
	}
	s.pollers.Store(componentID, p)
	return p, nil
}

// sanitiseVolPath strips the leading slash from a volume path for use as a
// metric key suffix, e.g. "/volume1" → "volume1".
func sanitiseVolPath(path string) string {
	if len(path) > 0 && path[0] == '/' {
		return path[1:]
	}
	if path == "" {
		return "unknown"
	}
	return path
}

// compile-time interface check.
var _ scanner.SnapshotScanner = (*SynologySnapshotScanner)(nil)
