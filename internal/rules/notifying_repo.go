package rules

import (
	"context"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// notifyingEventRepo wraps an EventRepo and fires the rules engine after every
// successful Create. All other methods delegate to the inner repo unchanged.
type notifyingEventRepo struct {
	inner  repo.EventRepo
	engine *Engine
}

// NewNotifyingEventRepo returns an EventRepo that triggers the rules engine
// asynchronously after every successful event creation.
func NewNotifyingEventRepo(inner repo.EventRepo, engine *Engine) repo.EventRepo {
	return &notifyingEventRepo{inner: inner, engine: engine}
}

func (n *notifyingEventRepo) Create(ctx context.Context, event *models.Event) error {
	if err := n.inner.Create(ctx, event); err != nil {
		return err
	}
	go n.engine.Evaluate(context.Background(), *event)
	return nil
}

func (n *notifyingEventRepo) List(ctx context.Context, f repo.ListFilter) ([]models.Event, int, error) {
	return n.inner.List(ctx, f)
}

func (n *notifyingEventRepo) Get(ctx context.Context, id string) (*models.Event, error) {
	return n.inner.Get(ctx, id)
}

func (n *notifyingEventRepo) Timeseries(ctx context.Context, since, until time.Time, granularity, sourceID, level string) ([]repo.TimeseriesBucket, error) {
	return n.inner.Timeseries(ctx, since, until, granularity, sourceID, level)
}

func (n *notifyingEventRepo) CountForCategory(ctx context.Context, f repo.CategoryFilter) (int, error) {
	return n.inner.CountForCategory(ctx, f)
}

func (n *notifyingEventRepo) LatestPerApp(ctx context.Context, appIDs []string) (map[string]*models.Event, error) {
	return n.inner.LatestPerApp(ctx, appIDs)
}

func (n *notifyingEventRepo) DeleteByLevelBefore(ctx context.Context, level string, before time.Time) (int64, error) {
	return n.inner.DeleteByLevelBefore(ctx, level, before)
}

func (n *notifyingEventRepo) GroupByTypeAndLevel(ctx context.Context, sourceID string, since, until time.Time) ([]repo.EventTypeCount, error) {
	return n.inner.GroupByTypeAndLevel(ctx, sourceID, since, until)
}

func (n *notifyingEventRepo) MetricsForApp(ctx context.Context, appID string, since, until time.Time) (repo.EventMetrics, error) {
	return n.inner.MetricsForApp(ctx, appID, since, until)
}

func (n *notifyingEventRepo) CountPerApp(ctx context.Context, since time.Time) ([]repo.AppEventCount, error) {
	return n.inner.CountPerApp(ctx, since)
}
