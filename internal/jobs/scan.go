package jobs

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/digitalcheffe/nora/internal/docker"
	"github.com/digitalcheffe/nora/internal/infra"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// ScanOneComponent immediately runs the appropriate poller for a single
// infrastructure component. It returns the resulting status string and any
// error encountered. This is the backend for the "Discover Now" API endpoint.
func ScanOneComponent(ctx context.Context, store *repo.Store, c *models.InfrastructureComponent) (string, error) {
	if !c.Enabled {
		return c.LastStatus, fmt.Errorf("component is disabled")
	}

	log.Printf("scan: starting manual scan of %s (%s) [%s]", c.Name, c.ID, c.CollectionMethod)

	var pollErr error
	var source string // poller label used in event messages

	switch c.CollectionMethod {
	case "proxmox_api":
		source = "proxmox"
		if c.Credentials == nil || *c.Credentials == "" {
			log.Printf("scan: %s (%s): no credentials configured", c.Name, c.ID)
			return "offline", fmt.Errorf("no credentials configured — edit the component and save credentials first")
		}
		poller, err := infra.NewProxmoxPoller(c.ID, *c.Credentials)
		if err != nil {
			log.Printf("scan: %s (%s): invalid credentials: %v", c.Name, c.ID, err)
			return "offline", fmt.Errorf("invalid credentials: %w", err)
		}
		pollErr = poller.Poll(ctx, store)

	case "synology_api":
		source = "synology"
		if c.Credentials == nil || *c.Credentials == "" {
			log.Printf("scan: %s (%s): no credentials configured", c.Name, c.ID)
			return "offline", fmt.Errorf("no credentials configured — edit the component and save credentials first")
		}
		poller, err := infra.NewSynologyPoller(c.ID, *c.Credentials)
		if err != nil {
			log.Printf("scan: %s (%s): invalid credentials: %v", c.Name, c.ID, err)
			return "offline", fmt.Errorf("invalid credentials: %w", err)
		}
		pollErr = poller.Poll(ctx, store)

	case "snmp":
		source = "snmp"
		if c.SNMPConfig == nil || *c.SNMPConfig == "" {
			log.Printf("scan: %s (%s): no SNMP config", c.Name, c.ID)
			return "offline", fmt.Errorf("no SNMP config — edit the component and save SNMP settings first")
		}
		poller, err := infra.NewSNMPPoller(c.ID, c.IP, *c.SNMPConfig)
		if err != nil {
			log.Printf("scan: %s (%s): invalid SNMP config: %v", c.Name, c.ID, err)
			return "offline", fmt.Errorf("invalid SNMP config: %w", err)
		}
		pollErr = poller.Poll(ctx, store)

	case "traefik_api":
		source = "traefik"
		creds := resolveTraefikCreds(*c)
		log.Printf("scan: %s (%s): traefik poll → %s", c.Name, c.ID, creds.APIURL)
		pollErr = pollTraefikComponent(ctx, store, *c, creds)

	case "docker_socket":
		source = "docker"
		poller, err := docker.NewResourcePoller(store, c.ID)
		if err != nil {
			log.Printf("scan: %s (%s): docker client: %v", c.Name, c.ID, err)
			return "offline", fmt.Errorf("docker client unavailable: %w", err)
		}
		poller.PollAll(ctx)

	default:
		return c.LastStatus, fmt.Errorf("collection method %q does not support on-demand scanning", c.CollectionMethod)
	}

	polledAt := time.Now().UTC().Format(time.RFC3339Nano)
	if pollErr != nil {
		log.Printf("scan: %s (%s) failed: %v", c.Name, c.ID, pollErr)
		emitInfraEvent(ctx, store, c.ID, c.Name, source, "manual", "failed", pollErr.Error())
		_ = store.InfraComponents.UpdateStatus(ctx, c.ID, "offline", polledAt)
		return "offline", pollErr
	}

	log.Printf("scan: %s (%s) complete", c.Name, c.ID)
	emitInfraEvent(ctx, store, c.ID, c.Name, source, "manual", "ok", "")

	// Re-fetch to return the status the poller wrote (may be "online" or "degraded").
	updated, err := store.InfraComponents.Get(ctx, c.ID)
	if err != nil {
		return "online", nil
	}
	return updated.LastStatus, nil
}
