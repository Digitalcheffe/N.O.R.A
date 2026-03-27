package docker

// ResourcePoller polls CPU, memory, and disk usage from Docker containers.
// Implementation deferred to T-17.
type ResourcePoller struct {
	watcher *Watcher
}
