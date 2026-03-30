package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/digitalcheffe/nora/internal/infra"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/google/uuid"
)

// ── Transition state (in-memory, reset on restart) ─────────────────────────

// traefikRouterErrors tracks the last known routers_errors count per component.
// key: component_id → int
var traefikRouterErrors sync.Map

// traefikRouterStatus tracks the last known router status per (component, router).
// key: "{component_id}:{router_name}" → string
var traefikRouterStatus sync.Map

// traefikServerState tracks the last known server UP/DOWN state.
// key: "{component_id}:{service_name}:{server_url}" → string ("UP"|"DOWN")
var traefikServerState sync.Map

// ── Overview polling ───────────────────────────────────────────────────────

// pollTraefikOverview fetches /api/overview, persists it, and fires transition events.
func pollTraefikOverview(ctx context.Context, store *repo.Store, c models.InfrastructureComponent, client *infra.TraefikClient) {
	raw, err := client.FetchOverview(ctx)
	if err != nil {
		log.Printf("traefik expanded: overview fetch for %s (%s): %v", c.Name, c.ID, err)
		return
	}

	ov := &models.TraefikOverview{
		ComponentID:      c.ID,
		Version:          raw.Version,
		RoutersTotal:     raw.HTTP.Routers.Total,
		RoutersErrors:    raw.HTTP.Routers.Errors,
		RoutersWarnings:  raw.HTTP.Routers.Warnings,
		ServicesTotal:    raw.HTTP.Services.Total,
		ServicesErrors:   raw.HTTP.Services.Errors,
		MiddlewaresTotal: raw.HTTP.Middlewares.Total,
		UpdatedAt:        time.Now().UTC(),
	}

	if err := store.TraefikOverview.Upsert(ctx, ov); err != nil {
		log.Printf("traefik expanded: upsert overview for %s (%s): %v", c.Name, c.ID, err)
	}

	// ── Transition: routers_errors 0 → N or N → 0 ─────────────────────────
	prev, _ := traefikRouterErrors.Swap(c.ID, raw.HTTP.Routers.Errors)
	prevErrors := 0
	if prev != nil {
		prevErrors = prev.(int)
	}

	switch {
	case prevErrors == 0 && raw.HTTP.Routers.Errors > 0:
		fireTraefikEvent(ctx, store, c, "warn",
			fmt.Sprintf("Traefik: %d router configuration error(s) detected", raw.HTTP.Routers.Errors))
	case prevErrors > 0 && raw.HTTP.Routers.Errors == 0:
		fireTraefikEvent(ctx, store, c, "info",
			"Traefik: all router errors resolved")
	}
}

// ── Router status polling ──────────────────────────────────────────────────

// pollTraefikRouterStatus checks per-router enabled/disabled transitions.
// It must be called after TraefikDiscovery.Run so the route data is already stored,
// but it works off the in-memory slice returned by FetchRouters to avoid an extra DB round-trip.
func pollTraefikRouterStatus(ctx context.Context, store *repo.Store, c models.InfrastructureComponent, routers []infra.TraefikRouter) {
	for _, rr := range routers {
		// Skip internal Traefik routers.
		if rr.ServiceName == "api@internal" || rr.ServiceName == "dashboard@internal" {
			continue
		}

		key := c.ID + ":" + rr.Name
		status := rr.Status
		if status == "" {
			status = "enabled"
		}

		prev, _ := traefikRouterStatus.Swap(key, status)
		if prev == nil {
			// First time we see this router — no transition event yet.
			continue
		}
		prevStatus := prev.(string)
		if prevStatus == status {
			continue
		}

		// Determine the display name (domain if available, else router name).
		label := rr.Name
		if d := extractDomainLabel(rr.Rule); d != "" {
			label = d
		}

		switch status {
		case "disabled":
			fireTraefikEvent(ctx, store, c, "error",
				fmt.Sprintf("Traefik router disabled: %s", label))
		case "enabled":
			fireTraefikEvent(ctx, store, c, "info",
				fmt.Sprintf("Traefik router restored: %s", label))
		}
	}
}

// extractDomainLabel parses the Host() rule and returns the domain, or "" on failure.
func extractDomainLabel(rule string) string {
	if d := infra.ParseHostFromRule(rule); d != nil {
		return *d
	}
	return ""
}

// ── Service health polling ─────────────────────────────────────────────────

// pollTraefikServices fetches /api/http/services, persists them, and fires
// server UP/DOWN transition events.
func pollTraefikServices(ctx context.Context, store *repo.Store, c models.InfrastructureComponent, client *infra.TraefikClient) {
	svcs, err := client.FetchServices(ctx)
	if err != nil {
		log.Printf("traefik expanded: services fetch for %s (%s): %v", c.Name, c.ID, err)
		return
	}

	now := time.Now().UTC()
	presentNames := make([]string, 0, len(svcs))

	for _, svc := range svcs {
		// Skip internal Traefik services.
		if strings.HasSuffix(svc.Name, "@internal") {
			continue
		}

		presentNames = append(presentNames, svc.Name)

		up, down := 0, 0
		for _, state := range svc.ServerStatus {
			if strings.EqualFold(state, "UP") {
				up++
			} else {
				down++
			}
		}

		// Serialise server_status map.
		ssJSON := "{}"
		if b, err := json.Marshal(svc.ServerStatus); err == nil {
			ssJSON = string(b)
		}

		row := &models.TraefikService{
			ID:               c.ID + ":" + svc.Name,
			ComponentID:      c.ID,
			ServiceName:      svc.Name,
			ServiceType:      svc.Type,
			Status:           svc.Status,
			ServerCount:      len(svc.ServerStatus),
			ServersUp:        up,
			ServersDown:      down,
			ServerStatusJSON: ssJSON,
			LastSeen:         now,
		}

		if err := store.TraefikServices.Upsert(ctx, row); err != nil {
			log.Printf("traefik expanded: upsert service %s for %s: %v", svc.Name, c.ID, err)
		}

		// ── Server UP/DOWN transitions ────────────────────────────────────
		for serverURL, state := range svc.ServerStatus {
			stateKey := c.ID + ":" + svc.Name + ":" + serverURL
			prev, _ := traefikServerState.Swap(stateKey, state)
			if prev == nil {
				// First observation — no transition event.
				continue
			}
			prevState := prev.(string)
			if strings.EqualFold(prevState, state) {
				continue
			}
			if strings.EqualFold(state, "DOWN") {
				fireTraefikEvent(ctx, store, c, "error",
					fmt.Sprintf("Traefik backend down: %s → %s", svc.Name, serverURL))
			} else if strings.EqualFold(state, "UP") {
				fireTraefikEvent(ctx, store, c, "info",
					fmt.Sprintf("Traefik backend recovered: %s → %s", svc.Name, serverURL))
			}
		}
	}

	// Remove services that are no longer present in Traefik.
	if err := store.TraefikServices.DeleteAbsent(ctx, c.ID, presentNames); err != nil {
		log.Printf("traefik expanded: delete absent services for %s: %v", c.ID, err)
	}
}

// ── Event helper ───────────────────────────────────────────────────────────

func fireTraefikEvent(ctx context.Context, store *repo.Store, c models.InfrastructureComponent, severity, text string) {
	fields := fmt.Sprintf(`{"source":"traefik","component_id":%q,"component_name":%q}`,
		c.ID, c.Name)
	ev := &models.Event{
		ID:         uuid.New().String(),
		Level:      severity,
		SourceName: c.Name,
		SourceType: "physical_host",
		SourceID:   c.ID,
		Title:      text,
		Payload:    fields,
		CreatedAt:  time.Now().UTC(),
	}
	if err := store.Events.Create(ctx, ev); err != nil {
		log.Printf("traefik expanded: write event for %s: %v", c.Name, err)
	}
}
