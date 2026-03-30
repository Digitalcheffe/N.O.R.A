// Package scanner defines the three scan bucket interfaces used by NORA to
// collect data from infrastructure entities.
//
// Discovery — "What exists?" — finds and registers entities, updates topology.
// Metrics   — "What is the current measured state?" — collects numeric readings.
// Snapshots — "What is the current condition?" — captures slowly-changing state.
//
// Concrete implementations are provided by REFACTOR-06 (Discovery),
// REFACTOR-07 (Metrics), and REFACTOR-08 (Snapshots). This package establishes
// the interfaces and result types only.
package scanner

import "context"

// DiscoveryResult is returned by a DiscoveryScanner after a discovery pass.
type DiscoveryResult struct {
	// EntityID is the stable ID of the scanned entity.
	EntityID string
	// EntityType is the type string (e.g. "proxmox_node", "vm", "container").
	EntityType string
	// Found is the number of new or changed entities discovered.
	Found int
	// Disappeared is the number of previously-known entities that were not seen.
	Disappeared int
}

// MetricsResult is returned by a MetricsScanner after one collection pass.
type MetricsResult struct {
	// EntityID is the stable ID of the scanned entity.
	EntityID string
	// EntityType is the type string.
	EntityType string
	// Readings is the number of metric values collected and written.
	Readings int
}

// SnapshotResult is returned by a SnapshotScanner after one snapshot pass.
type SnapshotResult struct {
	// EntityID is the stable ID of the scanned entity.
	EntityID string
	// EntityType is the type string.
	EntityType string
	// Changed indicates whether any snapshot values differ from the previous pass.
	Changed bool
}

// DiscoveryScanner finds and registers entities for a single infrastructure
// target. Implementations must be safe to call concurrently.
type DiscoveryScanner interface {
	Discover(ctx context.Context, entityID string, entityType string) (*DiscoveryResult, error)
}

// MetricsScanner collects current numeric readings for a single infrastructure
// target and writes them to the metrics / resource-readings tables.
// Implementations must be safe to call concurrently.
type MetricsScanner interface {
	CollectMetrics(ctx context.Context, entityID string, entityType string) (*MetricsResult, error)
}

// SnapshotScanner captures the current condition of slowly-changing attributes
// for a single infrastructure target and writes them as point-in-time snapshots.
// Implementations must be safe to call concurrently.
type SnapshotScanner interface {
	TakeSnapshot(ctx context.Context, entityID string, entityType string) (*SnapshotResult, error)
}
