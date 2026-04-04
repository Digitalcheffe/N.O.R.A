package discovery

import (
	"context"
	"log"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// RunMetricsCollection computes per-app event throughput metrics (events/hour,
// avg payload bytes, peak/minute) for the previous complete hour and upserts
// the results into the metrics table for sparkline charts.
// Called as a GlobalDiscoveryJob on the scan scheduler's 1-hour discovery tick.
func RunMetricsCollection(ctx context.Context, store *repo.Store) error {
	now := time.Now().UTC()
	hourEnd := now.Truncate(time.Hour)
	hourStart := hourEnd.Add(-time.Hour)

	apps, err := store.Apps.List(ctx)
	if err != nil {
		return err
	}

	for _, app := range apps {
		m, err := store.Events.MetricsForApp(ctx, app.ID, hourStart, hourEnd)
		if err != nil {
			return err
		}
		metric := &models.Metric{
			AppID:           app.ID,
			Period:          hourStart,
			EventsPerHour:   m.EventsPerHour,
			AvgPayloadBytes: m.AvgPayloadBytes,
			PeakPerMinute:   m.PeakPerMinute,
		}
		if err := store.Metrics.Upsert(ctx, metric); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}

	log.Printf("metrics: collected hourly metrics for %d apps at %s", len(apps), hourStart.Format("2006-01-02T15:04"))
	return nil
}
