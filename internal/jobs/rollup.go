package jobs

import (
	"context"
	"log"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// RunMonthlyRollup processes the previous calendar month: for each app it
// groups events by event_type (from the fields JSON column) and severity,
// then upserts the counts into the rollups table. Safe to call multiple times.
func RunMonthlyRollup(ctx context.Context, store *repo.Store) error {
	now := time.Now().UTC()
	// The first day of the current month is the exclusive upper bound.
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	firstOfPrev := firstOfMonth.AddDate(0, -1, 0)

	year := firstOfPrev.Year()
	month := int(firstOfPrev.Month())

	apps, err := store.Apps.List(ctx)
	if err != nil {
		return err
	}

	for _, app := range apps {
		rows, err := store.Events.GroupByTypeAndLevel(ctx, app.ID, firstOfPrev, firstOfMonth)
		if err != nil {
			return err
		}
		for _, row := range rows {
			rollup := &models.Rollup{
				AppID:     app.ID,
				Year:      year,
				Month:     month,
				EventType: row.EventType,
				Severity:  row.Level,
				Count:     row.Count,
			}
			if err := store.Rollups.Upsert(ctx, rollup); err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}

	log.Printf("rollup: processed %d apps for %04d-%02d", len(apps), year, month)
	return nil
}

// StartMonthlyRollup wakes up daily at UTC midnight. On the 1st of each month
// it runs RunMonthlyRollup (which should complete before StartEventRetention
// runs at 02:00) until ctx is cancelled.
func StartMonthlyRollup(ctx context.Context, store *repo.Store) {
	delay := durationUntilNextMidnight()
	log.Printf("rollup: monthly rollup job waiting %s until next midnight", delay.Round(time.Minute))

	select {
	case <-ctx.Done():
		return
	case <-time.After(delay):
	}

	run := func() {
		if time.Now().UTC().Day() == 1 {
			if err := RunMonthlyRollup(ctx, store); err != nil && ctx.Err() == nil {
				log.Printf("rollup: monthly rollup error: %v", err)
			}
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
