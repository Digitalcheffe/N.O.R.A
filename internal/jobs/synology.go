package jobs

import (
	"context"
	"log"
	"time"

	"github.com/digitalcheffe/nora/internal/infra"
	"github.com/digitalcheffe/nora/internal/repo"
)

// RunSynologyPollers iterates all enabled synology infrastructure components,
// reusing existing poller instances (and their sessions) across calls.
// Errors per-component are logged; the process never crashes.
func RunSynologyPollers(ctx context.Context, store *repo.Store, pollers map[string]*infra.SynologyPoller) {
	components, err := store.InfraComponents.List(ctx)
	if err != nil {
		log.Printf("synology scheduler: list components: %v", err)
		return
	}

	for _, c := range components {
		if c.Type != "synology" || !c.Enabled {
			continue
		}
		if c.Credentials == nil || *c.Credentials == "" {
			log.Printf("synology scheduler: component %s (%s) has no credentials, skipping", c.Name, c.ID)
			continue
		}

		// Get or create the poller for this component.
		poller, ok := pollers[c.ID]
		if !ok {
			var newErr error
			poller, newErr = infra.NewSynologyPoller(c.ID, *c.Credentials)
			if newErr != nil {
				log.Printf("synology scheduler: component %s (%s): invalid credentials: %v", c.Name, c.ID, newErr)
				continue
			}
			pollers[c.ID] = poller
		}

		log.Printf("synology scheduler: polling %s (%s)", c.Name, c.ID)
		if err := poller.Poll(ctx, store); err != nil {
			log.Printf("synology scheduler: poll %s (%s): %v", c.Name, c.ID, err)
			// Connection failure → mark offline.
			polledAt := time.Now().UTC().Format(time.RFC3339Nano)
			if updateErr := store.InfraComponents.UpdateStatus(ctx, c.ID, "offline", polledAt); updateErr != nil {
				log.Printf("synology scheduler: update status %s: %v", c.ID, updateErr)
			}
		} else {
			log.Printf("synology scheduler: poll %s (%s): complete", c.Name, c.ID)
		}
	}
}

// StartSynologyPollers runs RunSynologyPollers immediately on startup and then
// every 5 minutes until ctx is cancelled. Poller instances are retained across
// ticks so sessions are reused. All pollers are logged out on shutdown.
func StartSynologyPollers(ctx context.Context, store *repo.Store) {
	pollers := make(map[string]*infra.SynologyPoller)

	log.Printf("synology scheduler: started (interval=5m)")
	RunSynologyPollers(ctx, store, pollers)

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("synology scheduler: stopped, logging out sessions")
			for _, p := range pollers {
				p.Shutdown(context.Background())
			}
			return
		case <-ticker.C:
			RunSynologyPollers(ctx, store, pollers)
		}
	}
}
