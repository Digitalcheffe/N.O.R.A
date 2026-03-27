package jobs

import (
	"context"
	"log"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// RunMetricsCollection computes per-app event metrics for the previous
// complete hour and upserts the results into the metrics table.
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

// StartMetricsCollection runs RunMetricsCollection every hour until ctx is cancelled.
func StartMetricsCollection(ctx context.Context, store *repo.Store) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	log.Printf("metrics: hourly collection job started")
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := RunMetricsCollection(ctx, store); err != nil && ctx.Err() == nil {
				log.Printf("metrics: hourly collection error: %v", err)
			}
		}
	}
}
