package infra

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/digitalcheffe/nora/internal/apptemplate"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/google/uuid"
)

// EnrichAppOnLink wires up monitor checks, SSL checks, and historical resource
// readings when a discovered container or route is linked to an app.
//
// containerID is the UUID of the discovered_containers row (not the Docker ID).
// routeID is the UUID of the discovered_routes row.
//
// All enrichment steps are best-effort: errors are logged but do not prevent
// the link from completing. The function returns an error only if the app
// itself cannot be loaded.
func EnrichAppOnLink(
	ctx context.Context,
	store *repo.Store,
	profiles apptemplate.Loader,
	appID string,
	containerID *string,
	routeID *string,
) error {
	app, err := store.Apps.Get(ctx, appID)
	if err != nil {
		return fmt.Errorf("enrichment: load app %s: %w", appID, err)
	}

	// Step 1 — SSL check from discovered route domain.
	if routeID != nil {
		enrichSSLCheck(ctx, store, *routeID)
	}

	// Step 3 — backfill historical resource readings.
	if containerID != nil {
		enrichResourceReadings(ctx, store, app, *containerID)
	}

	return nil
}

// enrichSSLCheck creates an SSL monitor check for the route's domain if one
// does not already exist.
func enrichSSLCheck(ctx context.Context, store *repo.Store, routeID string) {
	route, err := store.DiscoveredRoutes.GetDiscoveredRoute(ctx, routeID)
	if err != nil {
		log.Printf("enrichment: load route %q: %v", routeID, err)
		return
	}
	if route.Domain == nil || *route.Domain == "" {
		return
	}
	domain := *route.Domain
	exists, err := store.Checks.ExistsForTypeAndTarget(ctx, "ssl", domain)
	if err != nil {
		log.Printf("enrichment: ssl check existence for domain %q: %v", domain, err)
		return
	}
	if exists {
		return
	}
	check := &models.MonitorCheck{
		ID:           uuid.New().String(),
		Name:         "SSL — " + domain,
		Type:         "ssl",
		Target:       domain,
		IntervalSecs: 3600,
		SSLWarnDays:  30,
		SSLCritDays:  7,
		Enabled:      true,
		CreatedAt:    time.Now().UTC(),
	}
	if err := store.Checks.Create(ctx, check); err != nil {
		log.Printf("enrichment: create SSL check for domain %q: %v", domain, err)
		return
	}
	log.Printf("enrichment: created SSL check for domain %q", domain)
}

// enrichResourceReadings backfills app_id on resource_readings for the docker
// container linked via discoveredContainerUUID.
func enrichResourceReadings(ctx context.Context, store *repo.Store, app *models.App, discoveredContainerUUID string) {
	container, err := store.DiscoveredContainers.GetDiscoveredContainer(ctx, discoveredContainerUUID)
	if err != nil {
		log.Printf("enrichment: load discovered container %q: %v", discoveredContainerUUID, err)
		return
	}
	n, err := store.Resources.BackfillAppID(ctx, container.ContainerID, app.ID)
	if err != nil {
		log.Printf("enrichment: backfill resource readings for app %q: %v", app.Name, err)
		return
	}
	if n > 0 {
		log.Printf("enrichment: backfilled %d resource readings for app %q", n, app.Name)
	}
}

// substituteBaseURL replaces {base_url} in tmplURL with the base_url value
// from the app config JSON. Returns an error when the placeholder is present
// but base_url is missing or empty — this prevents an unresolved template from
// being stored as a check target.
func substituteBaseURL(tmplURL string, cfg models.ConfigJSON) (string, error) {
	return cfg.ResolveTemplateVars(tmplURL)
}

// parseIntervalSecs converts a duration string (e.g. "5m", "1h", "30s") to
// seconds. Returns fallback when s is empty or cannot be parsed.
func parseIntervalSecs(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return fallback
	}
	return int(d.Seconds())
}
