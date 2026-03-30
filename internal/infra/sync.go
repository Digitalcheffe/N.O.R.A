package infra

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/google/uuid"
)

// SyncWorker periodically syncs all enabled Traefik integrations, populates
// the traefik_certs cache, and fires warn/error events for expiring certs.
type SyncWorker struct {
	store *repo.Store

	// firedToday tracks which (domain, date) pairs have already produced an
	// expiry event today, preventing duplicate events across sync cycles.
	mu         sync.Mutex
	firedToday map[string]string // domain → "YYYY-MM-DD" of last fire
}

// NewSyncWorker returns a SyncWorker wired to store.
func NewSyncWorker(store *repo.Store) *SyncWorker {
	return &SyncWorker{
		store:      store,
		firedToday: make(map[string]string),
	}
}

// Start loads all enabled Traefik integrations, syncs each immediately, then
// re-syncs on a 60-second ticker. It blocks until ctx is cancelled.
func (w *SyncWorker) Start(ctx context.Context) {
	log.Printf("infra sync: starting")

	w.syncAll(ctx)

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("infra sync: stopped")
			return
		case <-ticker.C:
			w.syncAll(ctx)
		}
	}
}

// SyncOne runs a single synchronous sync for integrationID and returns the
// number of certs found. Used by the manual-sync API endpoint.
func (w *SyncWorker) SyncOne(ctx context.Context, integrationID string) (int, error) {
	integration, err := w.store.Infra.Get(ctx, integrationID)
	if err != nil {
		return 0, err
	}
	return w.syncIntegration(ctx, integration)
}

// syncAll loads all enabled integrations and syncs each one.
func (w *SyncWorker) syncAll(ctx context.Context) {
	integrations, err := w.store.Infra.List(ctx)
	if err != nil {
		log.Printf("infra sync: list integrations: %v", err)
		return
	}
	for _, i := range integrations {
		if !i.Enabled || i.Type != "traefik" {
			continue
		}
		if _, err := w.syncIntegration(ctx, i); err != nil {
			log.Printf("infra sync: integration %q (%s): %v", i.Name, i.ID, err)
		}
	}
}

// syncIntegration fetches certs from one Traefik instance, upserts them into
// the cache, updates the integration row, and fires expiry events as needed.
func (w *SyncWorker) syncIntegration(ctx context.Context, i *models.InfraIntegration) (int, error) {
	apiKey := ""
	if i.APIKey != nil {
		apiKey = *i.APIKey
	}
	client := NewTraefikClient(i.APIURL, apiKey)

	certs, err := client.FetchCerts(ctx)
	if err != nil {
		errStr := err.Error()
		statusErr := "error"
		i.LastStatus = &statusErr
		i.LastError = &errStr
		_ = w.store.Infra.Update(ctx, i)
		return 0, err
	}

	if err := w.store.Infra.UpsertCerts(ctx, i.ID, certs); err != nil {
		return 0, err
	}

	now := time.Now().UTC()
	statusOK := "ok"
	i.LastStatus = &statusOK
	i.LastError = nil
	i.LastSyncedAt = &now
	if err := w.store.Infra.Update(ctx, i); err != nil {
		log.Printf("infra sync: update integration %s: %v", i.ID, err)
	}

	for _, cert := range certs {
		w.maybeFireExpiryEvent(ctx, cert)
	}

	return len(certs), nil
}

// maybeFireExpiryEvent creates a warn or error event when a cert is within
// the expiry window. Deduplicates: at most one event per domain per calendar day.
func (w *SyncWorker) maybeFireExpiryEvent(ctx context.Context, cert *models.TraefikCert) {
	if cert.ExpiresAt == nil {
		return
	}
	daysRemaining := int(time.Until(*cert.ExpiresAt).Hours() / 24)

	var severity, displayText string
	switch {
	case daysRemaining <= 7:
		severity = "error"
		displayText = fmt.Sprintf("SSL expiry critical — %s: %d days remaining", cert.Domain, daysRemaining)
	case daysRemaining <= 30:
		severity = "warn"
		displayText = fmt.Sprintf("SSL expiring soon — %s: %d days remaining", cert.Domain, daysRemaining)
	default:
		return
	}

	today := time.Now().UTC().Format("2006-01-02")

	w.mu.Lock()
	last, alreadyFired := w.firedToday[cert.Domain]
	if alreadyFired && last == today {
		w.mu.Unlock()
		return
	}
	w.firedToday[cert.Domain] = today
	w.mu.Unlock()

	event := &models.Event{
		ID:         uuid.New().String(),
		Level:      severity,
		SourceName: cert.Domain,
		SourceType: "system",
		SourceID:   "",
		Title:      displayText,
		Payload:    `{"source":"traefik_cert","domain":"` + cert.Domain + `"}`,
		CreatedAt:  time.Now().UTC(),
	}
	if err := w.store.Events.Create(ctx, event); err != nil {
		log.Printf("infra sync: create expiry event for %s: %v", cert.Domain, err)
	}
}
