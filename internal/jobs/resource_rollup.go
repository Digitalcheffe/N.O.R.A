package jobs

import (
	"context"
	"log"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// RunHourlyRollup aggregates raw resource readings from the previous complete hour
// and upserts the results into resource_rollups with period_type="hour".
func RunHourlyRollup(ctx context.Context, store *repo.Store) error {
	now := time.Now().UTC()
	// Previous complete hour: from HH:00 to (HH+1):00
	hourEnd := now.Truncate(time.Hour)
	hourStart := hourEnd.Add(-time.Hour)

	aggs, err := store.ResourceRollups.AggregateReadings(ctx, hourStart, hourEnd)
	if err != nil {
		return err
	}

	for i := range aggs {
		a := &aggs[i]
		rollup := &models.ResourceRollup{
			SourceID:    a.SourceID,
			SourceType:  a.SourceType,
			Metric:      a.Metric,
			PeriodType:  "hour",
			PeriodStart: hourStart,
			Avg:         a.Avg,
			Min:         a.Min,
			Max:         a.Max,
		}
		if err := store.ResourceRollups.Upsert(ctx, rollup); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}

	log.Printf("resource rollup: processed %d sources for hour %s", len(aggs), hourStart.Format("2006-01-02T15:04"))
	return nil
}

// RunDailyRollup aggregates hourly rollups from the previous complete day and
// upserts the results into resource_rollups with period_type="day".
func RunDailyRollup(ctx context.Context, store *repo.Store) error {
	now := time.Now().UTC()
	// Previous complete day: midnight to midnight
	dayEnd := now.Truncate(24 * time.Hour)
	dayStart := dayEnd.Add(-24 * time.Hour)

	aggs, err := store.ResourceRollups.AggregateHourlyRollups(ctx, dayStart, dayEnd)
	if err != nil {
		return err
	}

	for i := range aggs {
		a := &aggs[i]
		rollup := &models.ResourceRollup{
			SourceID:    a.SourceID,
			SourceType:  a.SourceType,
			Metric:      a.Metric,
			PeriodType:  "day",
			PeriodStart: dayStart,
			Avg:         a.Avg,
			Min:         a.Min,
			Max:         a.Max,
		}
		if err := store.ResourceRollups.Upsert(ctx, rollup); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}

	log.Printf("resource rollup: processed %d sources for day %s", len(aggs), dayStart.Format("2006-01-02"))
	return nil
}

// RunRetentionPurge deletes:
//   - resource_readings older than 7 days
//   - hourly resource_rollups older than 90 days
//
// Daily rollups are never deleted.
func RunRetentionPurge(ctx context.Context, store *repo.Store) error {
	now := time.Now().UTC()

	readingCutoff := now.Add(-7 * 24 * time.Hour)
	deletedReadings, err := store.ResourceRollups.PurgeReadings(ctx, readingCutoff)
	if err != nil {
		return err
	}
	log.Printf("resource rollup: purged %d resource_readings older than %s", deletedReadings, readingCutoff.Format("2006-01-02"))

	hourlyCutoff := now.Add(-90 * 24 * time.Hour)
	deletedHourly, err := store.ResourceRollups.PurgeHourlyRollups(ctx, hourlyCutoff)
	if err != nil {
		return err
	}
	log.Printf("resource rollup: purged %d hourly rollups older than %s", deletedHourly, hourlyCutoff.Format("2006-01-02"))

	return nil
}

// durationUntilNextMidnight returns the duration from now until the next UTC midnight.
func durationUntilNextMidnight() time.Duration {
	now := time.Now().UTC()
	next := now.Truncate(24 * time.Hour).Add(24 * time.Hour)
	return next.Sub(now)
}

// StartHourlyRollup runs RunHourlyRollup every hour until ctx is cancelled.
func StartHourlyRollup(ctx context.Context, store *repo.Store) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	log.Printf("resource rollup: hourly job started")
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := RunHourlyRollup(ctx, store); err != nil && ctx.Err() == nil {
				log.Printf("resource rollup: hourly job error: %v", err)
			}
		}
	}
}

// StartDailyRollup waits until the next UTC midnight, then runs RunDailyRollup
// and RunRetentionPurge every 24 hours until ctx is cancelled.
func StartDailyRollup(ctx context.Context, store *repo.Store) {
	delay := durationUntilNextMidnight()
	log.Printf("resource rollup: daily job waiting %s until next midnight", delay.Round(time.Minute))

	select {
	case <-ctx.Done():
		return
	case <-time.After(delay):
	}

	// Run immediately at the first midnight, then every 24 hours.
	runDaily := func() {
		if err := RunDailyRollup(ctx, store); err != nil && ctx.Err() == nil {
			log.Printf("resource rollup: daily job error: %v", err)
		}
		if err := RunRetentionPurge(ctx, store); err != nil && ctx.Err() == nil {
			log.Printf("resource rollup: purge job error: %v", err)
		}
	}

	runDaily()

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runDaily()
		}
	}
}
