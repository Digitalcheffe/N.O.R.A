package snapshot

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
	"github.com/digitalcheffe/nora/internal/scanner"
)

// TraefikSnapshotScanner captures slowly-changing Traefik router and service
// state every SnapshotInterval.  It fires events only on condition changes:
//   - Router enabled → disabled (and vice versa)
//   - Backend server UP → DOWN (and vice versa)
//
// State is held in memory between ticks; the scanner is re-created on restart
// so the first pass after a restart establishes a baseline without firing events.
type TraefikSnapshotScanner struct {
	store        *repo.Store
	routerStatus sync.Map // key: "{entityID}:{routerName}" → string status
	serverState  sync.Map // key: "{entityID}:{serviceName}:{serverURL}" → string ("UP"|"DOWN")
}

// NewTraefikSnapshotScanner returns a TraefikSnapshotScanner backed by store.
func NewTraefikSnapshotScanner(store *repo.Store) *TraefikSnapshotScanner {
	return &TraefikSnapshotScanner{store: store}
}

// TakeSnapshot fetches router and service state from the Traefik API, upserts
// service records, and fires transition events on status changes.
func (s *TraefikSnapshotScanner) TakeSnapshot(ctx context.Context, entityID, entityType string) (*scanner.SnapshotResult, error) {
	c, err := s.store.InfraComponents.Get(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("get component %s: %w", entityID, err)
	}

	apiURL, apiKey := infra.ResolveTraefikCreds(c.IP, c.Credentials)
	client := infra.NewTraefikClient(apiURL, apiKey)

	if err := client.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}

	now := time.Now().UTC()
	changed := false

	// ── Router enabled/disabled transitions ───────────────────────────────────

	routers, err := client.FetchRouters(ctx)
	if err != nil {
		log.Printf("traefik snapshot: fetch routers for %s: %v", c.Name, err)
	} else {
		for _, rr := range routers {
			// Skip internal Traefik routers.
			if rr.ServiceName == "api@internal" || rr.ServiceName == "dashboard@internal" {
				continue
			}
			status := rr.Status
			if status == "" {
				status = "enabled"
			}
			key := entityID + ":" + rr.Name
			prev, _ := s.routerStatus.Swap(key, status)
			if prev == nil {
				continue // first observation — establish baseline, no event
			}
			if prev.(string) == status {
				continue
			}
			changed = true
			label := rr.Name
			if d := infra.ParseHostFromRule(rr.Rule); d != nil {
				label = *d
			}
			switch status {
			case "disabled":
				writeSnapshotEvent(ctx, s.store, entityID, c.Name, "physical_host", "error",
					fmt.Sprintf("[snapshot] Traefik router disabled: %s on %s", label, c.Name))
			default:
				writeSnapshotEvent(ctx, s.store, entityID, c.Name, "physical_host", "info",
					fmt.Sprintf("[snapshot] Traefik router restored: %s on %s", label, c.Name))
			}
		}
	}

	// ── Service backend UP/DOWN transitions ───────────────────────────────────

	svcs, err := client.FetchServices(ctx)
	if err != nil {
		log.Printf("traefik snapshot: fetch services for %s: %v", c.Name, err)
	} else {
		presentNames := make([]string, 0, len(svcs))
		for _, svc := range svcs {
			if strings.HasSuffix(svc.Name, "@internal") {
				continue
			}
			presentNames = append(presentNames, svc.Name)

			// Upsert service row.
			up, down := 0, 0
			for _, state := range svc.ServerStatus {
				if strings.EqualFold(state, "UP") {
					up++
				} else {
					down++
				}
			}
			ssJSON := "{}"
			if b, err := json.Marshal(svc.ServerStatus); err == nil {
				ssJSON = string(b)
			}
			row := &models.TraefikService{
				ID:               entityID + ":" + svc.Name,
				ComponentID:      entityID,
				ServiceName:      svc.Name,
				ServiceType:      svc.Type,
				Status:           svc.Status,
				ServerCount:      len(svc.ServerStatus),
				ServersUp:        up,
				ServersDown:      down,
				ServerStatusJSON: ssJSON,
				LastSeen:         now,
			}
			if err := s.store.TraefikServices.Upsert(ctx, row); err != nil {
				log.Printf("traefik snapshot: upsert service %s for %s: %v", svc.Name, c.Name, err)
			}

			// Per-server state transitions.
			for serverURL, state := range svc.ServerStatus {
				stateKey := entityID + ":" + svc.Name + ":" + serverURL
				prev, _ := s.serverState.Swap(stateKey, state)
				if prev == nil {
					continue // first observation
				}
				if strings.EqualFold(prev.(string), state) {
					continue
				}
				changed = true
				if strings.EqualFold(state, "DOWN") {
					writeSnapshotEvent(ctx, s.store, entityID, c.Name, "physical_host", "error",
						fmt.Sprintf("[snapshot] Traefik backend down: %s → %s on %s", svc.Name, serverURL, c.Name))
				} else if strings.EqualFold(state, "UP") {
					writeSnapshotEvent(ctx, s.store, entityID, c.Name, "physical_host", "info",
						fmt.Sprintf("[snapshot] Traefik backend recovered: %s → %s on %s", svc.Name, serverURL, c.Name))
				}
			}
		}

		// Remove services that no longer appear in Traefik.
		if err := s.store.TraefikServices.DeleteAbsent(ctx, entityID, presentNames); err != nil {
			log.Printf("traefik snapshot: delete absent services for %s: %v", c.Name, err)
		}
	}

	writeDebugEvent(ctx, s.store, entityID, c.Name, "physical_host")

	return &scanner.SnapshotResult{
		EntityID:   entityID,
		EntityType: entityType,
		Changed:    changed,
	}, nil
}

// compile-time check that TraefikSnapshotScanner satisfies the interface.
var _ scanner.SnapshotScanner = (*TraefikSnapshotScanner)(nil)
