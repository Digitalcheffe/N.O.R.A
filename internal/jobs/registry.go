package jobs

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// JobEntry is a single registered background job.
type JobEntry struct {
	ID          string
	Name        string
	Description string
	Category    string
	RunFn       func(ctx context.Context) error

	mu            sync.Mutex
	lastRunAt     *time.Time
	lastRunStatus *string
}

// LastRunAt returns the time of the last run, or nil if never run.
func (e *JobEntry) LastRunAt() *time.Time {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.lastRunAt == nil {
		return nil
	}
	t := *e.lastRunAt
	return &t
}

// LastRunStatus returns "ok" or "error" for the last run, or nil.
func (e *JobEntry) LastRunStatus() *string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.lastRunStatus == nil {
		return nil
	}
	s := *e.lastRunStatus
	return &s
}

// Registry holds all registered background jobs in insertion order.
type Registry struct {
	mu    sync.RWMutex
	jobs  map[string]*JobEntry
	order []string
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		jobs: make(map[string]*JobEntry),
	}
}

// Register adds a job to the registry. Panics if the ID is already registered.
func (r *Registry) Register(entry *JobEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.jobs[entry.ID]; ok {
		panic(fmt.Sprintf("jobs: duplicate job ID %q", entry.ID))
	}
	r.jobs[entry.ID] = entry
	r.order = append(r.order, entry.ID)
}

// List returns all registered jobs in insertion order.
func (r *Registry) List() []*JobEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*JobEntry, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.jobs[id])
	}
	return out
}

// Get returns the job entry for the given ID, or nil if not found.
func (r *Registry) Get(id string) *JobEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.jobs[id]
}

// Run executes the job with the given ID, enforcing a 60-second timeout.
// Updates last_run_at and last_run_status on the entry after completion.
// Returns an error if the ID is unknown or the job returns an error.
func (r *Registry) Run(ctx context.Context, id string) error {
	r.mu.RLock()
	entry, ok := r.jobs[id]
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("unknown job id: %q", id)
	}

	runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	err := entry.RunFn(runCtx)

	now := time.Now().UTC()
	entry.mu.Lock()
	entry.lastRunAt = &now
	var status string
	if err != nil {
		status = "error"
	} else {
		status = "ok"
	}
	entry.lastRunStatus = &status
	entry.mu.Unlock()

	return err
}
