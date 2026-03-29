package jobs

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/digitalcheffe/nora/internal/infra"
	"github.com/digitalcheffe/nora/internal/repo"
)

const snmpPollTimeout = 30 * time.Second

// RunSNMPPollers iterates all enabled infrastructure_components with
// collection_method = "snmp" and polls each concurrently, with a 30-second
// per-component timeout. Errors are logged; the process never crashes.
func RunSNMPPollers(ctx context.Context, store *repo.Store) {
	components, err := store.InfraComponents.List(ctx)
	if err != nil {
		log.Printf("snmp scheduler: list components: %v", err)
		return
	}

	var wg sync.WaitGroup

	for _, c := range components {
		if c.CollectionMethod != "snmp" || !c.Enabled {
			continue
		}
		if c.SNMPConfig == nil || *c.SNMPConfig == "" {
			log.Printf("snmp scheduler: component %s (%s) has no snmp_config, skipping", c.Name, c.ID)
			continue
		}

		wg.Add(1)
		go func(id, name, ip string, cfgJSON string) {
			defer wg.Done()

			pollCtx, cancel := context.WithTimeout(ctx, snmpPollTimeout)
			defer cancel()

			poller, err := infra.NewSNMPPoller(id, ip, cfgJSON)
			if err != nil {
				log.Printf("snmp scheduler: component %s (%s): invalid config: %v", name, id, err)
				return
			}

			log.Printf("snmp scheduler: polling %s (%s)", name, id)
			if err := poller.Poll(pollCtx, store); err != nil {
				log.Printf("snmp scheduler: poll %s (%s): %v", name, id, err)
				polledAt := time.Now().UTC().Format(time.RFC3339Nano)
				if updateErr := store.InfraComponents.UpdateStatus(ctx, id, "offline", polledAt); updateErr != nil {
					log.Printf("snmp scheduler: update status %s: %v", id, updateErr)
				}
			} else {
				log.Printf("snmp scheduler: poll %s (%s): complete", name, id)
			}
		}(c.ID, c.Name, c.IP, *c.SNMPConfig)
	}

	wg.Wait()
}

// StartSNMPPollers runs RunSNMPPollers immediately on startup and then every
// 5 minutes until ctx is cancelled.
func StartSNMPPollers(ctx context.Context, store *repo.Store) {
	log.Printf("snmp scheduler: started (interval=5m)")
	RunSNMPPollers(ctx, store)

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("snmp scheduler: stopped")
			return
		case <-ticker.C:
			RunSNMPPollers(ctx, store)
		}
	}
}
