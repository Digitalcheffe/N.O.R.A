package scanner

import "time"

// Scan bucket intervals. These are the canonical frequencies for each bucket
// type. The scheduler in Scheduler.Start uses these as its ticker durations.
const (
	DiscoveryInterval = 1 * time.Hour
	MetricsInterval   = 60 * time.Second
	SnapshotInterval  = 30 * time.Minute

	// Per-scan timeouts — individual entity scans are cancelled after these
	// durations so a single stuck target cannot block the scheduler.
	DiscoveryTimeout = 30 * time.Second
	MetricsTimeout   = 10 * time.Second
	SnapshotTimeout  = 15 * time.Second
)
