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
	"github.com/google/uuid"
)

// traefikComponentCredentials is the JSON shape stored in infrastructure_components.credentials
// for traefik-type components.
type traefikComponentCredentials struct {
	APIURL string `json:"api_url"`
	APIKey string `json:"api_key"`
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
		if c.Credentials == nil || *c.Credentials == "" {
			log.Printf("traefik component scheduler: component %s (%s) has no credentials, skipping", c.Name, c.ID)
			continue
		}

		var creds traefikComponentCredentials
		if err := json.Unmarshal([]byte(*c.Credentials), &creds); err != nil {
			log.Printf("traefik component scheduler: component %s (%s): invalid credentials: %v", c.Name, c.ID, err)
			continue
		}
		if creds.APIURL == "" {
			log.Printf("traefik component scheduler: component %s (%s): api_url is empty, skipping", c.Name, c.ID)
			continue
		}

		log.Printf("traefik component scheduler: polling %s (%s)", c.Name, c.ID)
		if err := pollTraefikComponent(ctx, store, c, creds); err != nil {
			log.Printf("traefik component scheduler: poll %s (%s): %v", c.Name, c.ID, err)
			polledAt := time.Now().UTC().Format(time.RFC3339Nano)
			if updateErr := store.InfraComponents.UpdateStatus(ctx, c.ID, "offline", polledAt); updateErr != nil {
				log.Printf("traefik component scheduler: update status %s: %v", c.ID, updateErr)
			}
		} else {
			log.Printf("traefik component scheduler: poll %s (%s): complete", c.Name, c.ID)
		}
	}
}

func pollTraefikComponent(ctx context.Context, store *repo.Store, c models.InfrastructureComponent, creds traefikComponentCredentials) error {
	client := infra.NewTraefikClient(creds.APIURL, creds.APIKey)

	// ── Fetch certs ──────────────────────────────────────────────────────────

	rawCerts, err := client.FetchCerts(ctx)
	if err != nil {
		return fmt.Errorf("fetch certs: %w", err)
	}

	// Convert TraefikCert → TraefikComponentCert.
	compCerts := make([]*models.TraefikComponentCert, 0, len(rawCerts))
	for _, rc := range rawCerts {
		cc := &models.TraefikComponentCert{
			ComponentID: c.ID,
			Domain:      rc.Domain,
			Issuer:      rc.Issuer,
			ExpiresAt:   rc.ExpiresAt,
			SANs:        rc.SANs,
		}
		compCerts = append(compCerts, cc)
	}

	if err := store.TraefikComponents.UpsertCerts(ctx, c.ID, compCerts); err != nil {
		return fmt.Errorf("upsert certs: %w", err)
	}

	// ── Sync SSL checks ───────────────────────────────────────────────────────

	// Build a map of existing owned checks by target (domain).
	existingChecks, err := store.Checks.ListBySourceComponent(ctx, c.ID)
	if err != nil {
		return fmt.Errorf("list existing checks: %w", err)
	}
	existingByDomain := make(map[string]models.MonitorCheck, len(existingChecks))
	for _, ch := range existingChecks {
		existingByDomain[ch.Target] = ch
	}

	// Track which domains are still present so we can delete stale checks.
	activeDomains := make(map[string]bool, len(compCerts))
	sslSource := "component"
	for _, cc := range compCerts {
		activeDomains[cc.Domain] = true

		var checkID string
		if existing, ok := existingByDomain[cc.Domain]; ok {
			checkID = existing.ID
		} else {
			checkID = uuid.New().String()
		}

		check := &models.MonitorCheck{
			ID:                checkID,
			Name:              c.Name + " — " + cc.Domain,
			Type:              "ssl",
			Target:            cc.Domain,
			IntervalSecs:      300,
			SSLWarnDays:       30,
			SSLCritDays:       7,
			SSLSource:         &sslSource,
			SourceComponentID: &c.ID,
			Enabled:           true,
		}
		if err := store.Checks.UpsertForComponent(ctx, check); err != nil {
			log.Printf("traefik component scheduler: upsert check for %s: %v", cc.Domain, err)
		}
	}

	// Delete checks for domains no longer present.
	for domain, ch := range existingByDomain {
		if !activeDomains[domain] {
			if err := store.Checks.Delete(ctx, ch.ID); err != nil {
				log.Printf("traefik component scheduler: delete stale check %s: %v", ch.ID, err)
			}
		}
	}

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
	}

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
