package jobs

import (
	"context"
	"log"
	"time"

	"github.com/digitalcheffe/nora/internal/infra"
	"github.com/digitalcheffe/nora/internal/repo"
)

// RunProxmoxPollers iterates all enabled proxmox_node infrastructure components
// and runs a poll cycle for each. Errors per-component are logged; the process
// never crashes.
func RunProxmoxPollers(ctx context.Context, store *repo.Store) {
	components, err := store.InfraComponents.List(ctx)
	if err != nil {
		log.Printf("proxmox scheduler: list components: %v", err)
		return
	}

	for _, c := range components {
		if c.Type != "proxmox_node" || !c.Enabled {
			continue
		}
		if c.Credentials == nil || *c.Credentials == "" {
			log.Printf("proxmox scheduler: component %s (%s) has no credentials, skipping", c.Name, c.ID)
			continue
		}

		poller, err := infra.NewProxmoxPoller(c.ID, *c.Credentials)
		if err != nil {
			log.Printf("proxmox scheduler: component %s (%s): invalid credentials: %v", c.Name, c.ID, err)
			continue
		}

		log.Printf("proxmox scheduler: polling %s (%s)", c.Name, c.ID)
		if err := poller.Poll(ctx, store); err != nil {
			log.Printf("proxmox scheduler: poll %s (%s): %v", c.Name, c.ID, err)
			emitInfraEvent(ctx, store, c.ID, c.Name, "proxmox", "scheduled", "failed", err.Error())
			// Connection failure → mark offline.
			polledAt := time.Now().UTC().Format(time.RFC3339Nano)
			if updateErr := store.InfraComponents.UpdateStatus(ctx, c.ID, "offline", polledAt); updateErr != nil {
				log.Printf("proxmox scheduler: update status %s: %v", c.ID, updateErr)
			}
		} else {
			log.Printf("proxmox scheduler: poll %s (%s): complete", c.Name, c.ID)
			emitInfraEvent(ctx, store, c.ID, c.Name, "proxmox", "scheduled", "ok", "")
		}
	}
}

// StartProxmoxPollers runs RunProxmoxPollers immediately on startup and then
// every 5 minutes until ctx is cancelled.
func StartProxmoxPollers(ctx context.Context, store *repo.Store) {
	log.Printf("proxmox scheduler: started (interval=5m)")
	RunProxmoxPollers(ctx, store)

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("proxmox scheduler: stopped")
			return
		case <-ticker.C:
			RunProxmoxPollers(ctx, store)
		}
	}
}
