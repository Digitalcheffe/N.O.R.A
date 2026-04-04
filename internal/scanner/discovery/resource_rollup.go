package discovery

import (
	"context"
	"log"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// RunHourlyRollup aggregates raw resource readings from the previous complete
// hour and upserts the results into resource_rollups with period_type="hour".
// Called as a GlobalDiscoveryJob on the scan scheduler's 1-hour discovery tick.
func RunHourlyRollup(ctx context.Context, store *repo.Store) error {
	now := time.Now().UTC()
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
