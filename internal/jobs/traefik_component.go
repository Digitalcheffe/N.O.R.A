package jobs

import (
	"context"
	"encoding/json"
	"fmt"
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

func pollTraefikComponent(ctx context.Context, store *repo.Store, c models.InfrastructureComponent, creds traefikComponentCredentials) error {
	client := infra.NewTraefikClient(creds.APIURL, creds.APIKey)

	// ── Connectivity check ────────────────────────────────────────────────────
	// Fail fast — if Traefik is unreachable the caller marks the component offline.
	if err := client.Ping(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}

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

