package jobs

import (
	"context"
	"log"
	"time"

	"github.com/digitalcheffe/nora/internal/repo"
)

// retentionWindows maps each event level to its retention duration.
var retentionWindows = map[string]time.Duration{
	"debug":    24 * time.Hour,
	"info":     7 * 24 * time.Hour,
	"warn":     30 * 24 * time.Hour,
	"error":    90 * 24 * time.Hour,
	"critical": 90 * 24 * time.Hour,
}

// ruleExecutionRetention is the retention window for rule_executions rows.
const ruleExecutionRetention = 30 * 24 * time.Hour

// RunEventRetention purges events whose created_at is older than the
// configured retention window for their level. Rollup rows are never
// touched by this function. Rule execution logs older than 30 days are
// also purged here.
func RunEventRetention(ctx context.Context, store *repo.Store) error {
	now := time.Now().UTC()
	for level, window := range retentionWindows {
		cutoff := now.Add(-window)
		n, err := store.Events.DeleteByLevelBefore(ctx, level, cutoff)
		if err != nil {
			return err
		}
		if n > 0 {
			log.Printf("retention: deleted %d %s events older than %s", n, level, window)
		}
	}

	if store.Rules != nil {
		cutoff := now.Add(-ruleExecutionRetention)
		n, err := store.Rules.DeleteExecutionsBefore(ctx, cutoff)
		if err != nil {
			log.Printf("retention: rule executions cleanup error: %v", err)
		} else if n > 0 {
			log.Printf("retention: deleted %d rule executions older than 30 days", n)
		}
	}

	return nil
}

// durationUntilNext2AM returns the duration from now until the next 02:00 UTC.
func durationUntilNext2AM() time.Duration {
	now := time.Now().UTC()
	next := time.Date(now.Year(), now.Month(), now.Day(), 2, 0, 0, 0, time.UTC)
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next.Sub(now)
}

// StartEventRetention waits until 02:00 UTC, runs RunEventRetention, then
// repeats every 24 hours until ctx is cancelled.
func StartEventRetention(ctx context.Context, store *repo.Store) {
	delay := durationUntilNext2AM()
	log.Printf("retention: event retention job waiting %s until next 02:00 UTC", delay.Round(time.Minute))

	select {
	case <-ctx.Done():
		return
	case <-time.After(delay):
	}

	run := func() {
		if err := RunEventRetention(ctx, store); err != nil && ctx.Err() == nil {
			log.Printf("retention: event retention error: %v", err)
		}
	}

	run()

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
