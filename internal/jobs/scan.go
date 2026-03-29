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

// ScanOneComponent immediately runs the appropriate poller for a single
// infrastructure component. It returns the resulting status string and any
// error encountered. This is the backend for the "Scan Now" API endpoint.
func ScanOneComponent(ctx context.Context, store *repo.Store, c *models.InfrastructureComponent) (string, error) {
	if !c.Enabled {
		return c.LastStatus, fmt.Errorf("component is disabled")
	}

	log.Printf("scan: starting manual scan of %s (%s) [%s]", c.Name, c.ID, c.CollectionMethod)

	var pollErr error

	switch c.CollectionMethod {
	case "proxmox_api":
		if c.Credentials == nil || *c.Credentials == "" {
			return "offline", fmt.Errorf("no credentials configured — edit the component and save credentials first")
		}
		poller, err := infra.NewProxmoxPoller(c.ID, *c.Credentials)
		if err != nil {
			return "offline", fmt.Errorf("invalid credentials: %w", err)
		}
		pollErr = poller.Poll(ctx, store)

	case "synology_api":
		if c.Credentials == nil || *c.Credentials == "" {
			return "offline", fmt.Errorf("no credentials configured — edit the component and save credentials first")
		}
		poller, err := infra.NewSynologyPoller(c.ID, *c.Credentials)
		if err != nil {
			return "offline", fmt.Errorf("invalid credentials: %w", err)
		}
		pollErr = poller.Poll(ctx, store)

	case "snmp":
		if c.SNMPConfig == nil || *c.SNMPConfig == "" {
			return "offline", fmt.Errorf("no SNMP config — edit the component and save SNMP settings first")
		}
		poller, err := infra.NewSNMPPoller(c.ID, c.IP, *c.SNMPConfig)
		if err != nil {
			return "offline", fmt.Errorf("invalid SNMP config: %w", err)
		}
		pollErr = poller.Poll(ctx, store)

	case "traefik_api":
		if c.Credentials == nil || *c.Credentials == "" {
			return "offline", fmt.Errorf("no credentials configured — edit the component and save credentials first")
		}
		var creds traefikComponentCredentials
		if err := json.Unmarshal([]byte(*c.Credentials), &creds); err != nil {
			return "offline", fmt.Errorf("invalid credentials JSON: %w", err)
		}
		if creds.APIURL == "" {
			return "offline", fmt.Errorf("api_url is empty — edit the component and set the Traefik API URL")
		}
		pollErr = pollTraefikComponent(ctx, store, *c, creds)

	case "docker_socket":
		return c.LastStatus, fmt.Errorf("docker components are monitored automatically via the Docker socket")

	default:
		return c.LastStatus, fmt.Errorf("collection method %q does not support on-demand scanning", c.CollectionMethod)
	}

	polledAt := time.Now().UTC().Format(time.RFC3339Nano)
	if pollErr != nil {
		log.Printf("scan: %s (%s) failed: %v", c.Name, c.ID, pollErr)
		_ = store.InfraComponents.UpdateStatus(ctx, c.ID, "offline", polledAt)
		return "offline", pollErr
	}

	log.Printf("scan: %s (%s) complete", c.Name, c.ID)

	// Re-fetch to return the status the poller wrote (may be "online" or "degraded").
	updated, err := store.InfraComponents.Get(ctx, c.ID)
	if err != nil {
		return "online", nil
	}
	return updated.LastStatus, nil
}
