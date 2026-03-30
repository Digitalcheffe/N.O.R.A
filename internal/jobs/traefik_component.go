package jobs

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/digitalcheffe/nora/internal/infra"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// traefikComponentCredentials is the JSON shape stored in infrastructure_components.credentials
// for traefik-type components.
type traefikComponentCredentials struct {
	APIURL string `json:"api_url"`
	APIKey string `json:"api_key"` // optional — Traefik may run without auth
}

// resolveTraefikCreds returns polling credentials for a Traefik component.
// If no credentials are stored (or the JSON is malformed), it falls back to
// http://{component.IP}:8080 — Traefik's default dashboard/API address.
// api_key is always optional; absence never blocks polling.
func resolveTraefikCreds(c models.InfrastructureComponent) traefikComponentCredentials {
	if c.Credentials != nil && *c.Credentials != "" {
		var creds traefikComponentCredentials
		if err := json.Unmarshal([]byte(*c.Credentials), &creds); err == nil && creds.APIURL != "" {
			return creds
		}
	}
	return traefikComponentCredentials{APIURL: "http://" + c.IP + ":8080"}
}

// RunTraefikComponentPollers iterates all enabled traefik infrastructure components,
// syncs certs and routes from the Traefik API, and upserts owned SSL checks.
func RunTraefikComponentPollers(ctx context.Context, store *repo.Store) {
	components, err := store.InfraComponents.List(ctx)
	if err != nil {
		log.Printf("traefik component scheduler: list components: %v", err)
		return
	}

	for _, c := range components {
		if c.Type != "traefik" || !c.Enabled {
			continue
		}
		creds := resolveTraefikCreds(c)
		log.Printf("traefik component scheduler: polling %s (%s) → %s", c.Name, c.ID, creds.APIURL)
		if err := pollTraefikComponent(ctx, store, c, creds); err != nil {
			log.Printf("traefik component scheduler: poll %s (%s): %v", c.Name, c.ID, err)
			emitInfraEvent(ctx, store, c.ID, c.Name, "traefik", "scheduled", "failed", err.Error())
			polledAt := time.Now().UTC().Format(time.RFC3339Nano)
			if updateErr := store.InfraComponents.UpdateStatus(ctx, c.ID, "offline", polledAt); updateErr != nil {
				log.Printf("traefik component scheduler: update status %s: %v", c.ID, updateErr)
			}
		} else {
			log.Printf("traefik component scheduler: poll %s (%s): complete", c.Name, c.ID)
			emitInfraEvent(ctx, store, c.ID, c.Name, "traefik", "scheduled", "ok", "")
		}
	}
}

func pollTraefikComponent(ctx context.Context, store *repo.Store, c models.InfrastructureComponent, creds traefikComponentCredentials) error {
	client := infra.NewTraefikClient(creds.APIURL, creds.APIKey)

	// ── Overview health (Infra-10) ────────────────────────────────────────────
	pollTraefikOverview(ctx, store, c, client)

	// ── Fetch routes ─────────────────────────────────────────────────────────

	rawRouters, err := client.FetchRouters(ctx)
	if err != nil {
		// Routes are non-critical — log and continue.
		log.Printf("traefik component scheduler: fetch routers for %s: %v", c.Name, err)
	} else {
		routes := make([]models.TraefikRoute, 0, len(rawRouters))
		for _, rr := range rawRouters {
			routes = append(routes, models.TraefikRoute{
				ComponentID: c.ID,
				Name:        rr.Name,
				Rule:        rr.Rule,
				Service:     rr.ServiceName,
				Status:      rr.Status,
			})
		}
		if err := store.TraefikComponents.UpsertRoutes(ctx, c.ID, routes); err != nil {
			log.Printf("traefik component scheduler: upsert routes for %s: %v", c.Name, err)
		}
		// Fire router status transition events (Infra-10).
		pollTraefikRouterStatus(ctx, store, c, rawRouters)
	}

	// ── Populate discovered_routes ────────────────────────────────────────────

	discovery := infra.NewTraefikDiscovery(store)
	if err := discovery.Run(ctx, &c); err != nil {
		// Non-critical — log but do not fail the component poll.
		log.Printf("traefik component scheduler: discovery run for %s: %v", c.Name, err)
	}

	// ── Service health (Infra-10) ─────────────────────────────────────────────
	pollTraefikServices(ctx, store, c, client)

	// ── Update component status ───────────────────────────────────────────────

	polledAt := time.Now().UTC().Format(time.RFC3339Nano)
	return store.InfraComponents.UpdateStatus(ctx, c.ID, "online", polledAt)
}

// StartTraefikComponentPollers runs RunTraefikComponentPollers immediately on
// startup and then every 5 minutes until ctx is cancelled.
func StartTraefikComponentPollers(ctx context.Context, store *repo.Store) {
	log.Printf("traefik component scheduler: started (interval=5m)")
	RunTraefikComponentPollers(ctx, store)

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("traefik component scheduler: stopped")
			return
		case <-ticker.C:
			RunTraefikComponentPollers(ctx, store)
		}
	}
}
