package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/digitalcheffe/nora/internal/infra"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/internal/scanner"
)

// TraefikMetricsScanner collects Traefik overview counts and fires events on
// router error count transitions every MetricsInterval.  One instance serves
// all traefik infrastructure components.
type TraefikMetricsScanner struct {
	store        *repo.Store
	routerErrors sync.Map // componentID → int (last known routers_errors count)
}

// NewTraefikMetricsScanner returns a TraefikMetricsScanner backed by store.
func NewTraefikMetricsScanner(store *repo.Store) *TraefikMetricsScanner {
	return &TraefikMetricsScanner{store: store}
}

// CollectMetrics pings the Traefik API, fetches /api/overview, upserts the
// traefik_overview row, fires router-error transition events, and writes
// resource readings for router/service counts.
func (s *TraefikMetricsScanner) CollectMetrics(ctx context.Context, entityID, entityType string) (*scanner.MetricsResult, error) {
	c, err := s.store.InfraComponents.Get(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("get component %s: %w", entityID, err)
	}

	apiURL, apiKey := infra.ResolveTraefikCreds(c.IP, c.Credentials)
	client := infra.NewTraefikClient(apiURL, apiKey)

	if err := client.Ping(ctx); err != nil {
		polledAt := time.Now().UTC().Format(time.RFC3339Nano)
		_ = s.store.InfraComponents.UpdateStatus(ctx, entityID, "offline", polledAt)
		return nil, fmt.Errorf("ping: %w", err)
	}

	raw, err := client.FetchOverview(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch overview: %w", err)
	}

	now := time.Now().UTC()

	// Persist overview as JSON into traefik_meta on infrastructure_components.
	metaBytes, _ := json.Marshal(map[string]interface{}{
		"version":           raw.Version,
		"routers_total":     raw.HTTP.Routers.Total,
		"routers_errors":    raw.HTTP.Routers.Errors,
		"routers_warnings":  raw.HTTP.Routers.Warnings,
		"services_total":    raw.HTTP.Services.Total,
		"services_errors":   raw.HTTP.Services.Errors,
		"middlewares_total": raw.HTTP.Middlewares.Total,
		"updated_at":        now.Format(time.RFC3339),
	})
	if err := s.store.InfraComponents.UpdateMeta(ctx, entityID, string(metaBytes)); err != nil {
		log.Printf("traefik metrics: update meta for %s: %v", c.Name, err)
	}

	// Router error count transitions — fire events only on 0→N and N→0.
	prev, _ := s.routerErrors.Swap(entityID, raw.HTTP.Routers.Errors)
	prevErrors := 0
	if prev != nil {
		prevErrors = prev.(int)
	}
	switch {
	case prevErrors == 0 && raw.HTTP.Routers.Errors > 0:
		writeMetricsEvent(ctx, s.store, entityID, c.Name, "physical_host", "warn",
			fmt.Sprintf("[metrics] Traefik: %d router error(s) on %s", raw.HTTP.Routers.Errors, c.Name))
	case prevErrors > 0 && raw.HTTP.Routers.Errors == 0:
		writeMetricsEvent(ctx, s.store, entityID, c.Name, "physical_host", "info",
			fmt.Sprintf("[metrics] Traefik: all router errors resolved on %s", c.Name))
	}

	// Mark component online.
	polledAt := now.Format(time.RFC3339Nano)
	if err := s.store.InfraComponents.UpdateStatus(ctx, entityID, "online", polledAt); err != nil {
		log.Printf("traefik metrics: update status for %s: %v", c.Name, err)
	}

	log.Printf("traefik metrics: %s: %d routers (%d errors), %d services",
		c.Name, raw.HTTP.Routers.Total, raw.HTTP.Routers.Errors, raw.HTTP.Services.Total)

	return &scanner.MetricsResult{
		EntityID:   entityID,
		EntityType: entityType,
	}, nil
}

// compile-time check that TraefikMetricsScanner satisfies the interface.
var _ scanner.MetricsScanner = (*TraefikMetricsScanner)(nil)
