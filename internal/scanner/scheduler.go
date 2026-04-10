package scanner

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

// GlobalSnapshotJob is a snapshot job that runs at each snapshot tick but
// operates on entities from tables other than infrastructure_components (e.g.
// monitor_checks).  It is invoked once per snapshot cycle after the per-entity
// pass completes.
type GlobalSnapshotJob interface {
	Run(ctx context.Context)
}

// GlobalSnapshotFunc is an adapter so a plain func(context.Context) satisfies
// GlobalSnapshotJob without defining a named type.
type GlobalSnapshotFunc func(ctx context.Context)

// Run implements GlobalSnapshotJob.
func (f GlobalSnapshotFunc) Run(ctx context.Context) { f(ctx) }

// GlobalDiscoveryJob is a discovery job that runs at each discovery tick but
// operates independently of the per-component scanner loop (e.g. the app
// event metrics aggregation which reads the events table rather than polling
// an external system).  It is invoked once per discovery cycle after the
// per-entity pass completes.
type GlobalDiscoveryJob interface {
	Run(ctx context.Context)
}

// GlobalDiscoveryFunc is an adapter so a plain func(context.Context) satisfies
// GlobalDiscoveryJob without defining a named type.
type GlobalDiscoveryFunc func(ctx context.Context)

// Run implements GlobalDiscoveryJob.
func (f GlobalDiscoveryFunc) Run(ctx context.Context) { f(ctx) }

// GlobalMetricsJob is a metrics job that runs at each metrics tick but
// operates independently of the per-component scanner loop (e.g. the Docker
// health poller which polls the local daemon rather than an infra component).
// It is invoked once per metrics cycle after the per-entity pass completes.
type GlobalMetricsJob interface {
	Run(ctx context.Context)
}

// GlobalDailyJob is a job that runs once per day — on startup and then every
// 24 hours. Use for expensive external calls with rate limits (e.g. container
// registry digest lookups).
type GlobalDailyJob interface {
	Run(ctx context.Context)
}

// GlobalDailyFunc is an adapter so a plain func(context.Context) satisfies
// GlobalDailyJob without defining a named type.
type GlobalDailyFunc func(ctx context.Context)

// Run implements GlobalDailyJob.
func (f GlobalDailyFunc) Run(ctx context.Context) { f(ctx) }

// ScanScheduler runs the three scan buckets — Discovery, Metrics, and
// Snapshots — on their canonical intervals. Each tick fans out to all enabled
// infrastructure components concurrently with a per-entity timeout.
//
// Concrete scanner implementations are registered via the Register* methods
// before Start is called. Entity types with no registered scanner are skipped
// silently, which lets REFACTOR-06/07/08 add implementations incrementally
// without requiring changes here.
//
// Discovery scanners can be registered by either entity type (e.g.
// "proxmox_node") or by collection_method (e.g. "snmp").  The scheduler
// checks type first, then falls back to collection_method, so integrations
// that share a generic host type but differ in collection method (SNMP) are
// handled correctly.
type ScanScheduler struct {
	store                    *repo.Store
	discovery                map[string]DiscoveryScanner // keyed by entity type
	discoveryByMethod        map[string]DiscoveryScanner // keyed by collection_method
	metrics                  map[string]MetricsScanner   // keyed by entity type
	metricsByMethod          map[string]MetricsScanner   // keyed by collection_method
	snapshots         map[string]SnapshotScanner // keyed by entity type
	snapshotsByMethod map[string]SnapshotScanner // keyed by collection_method
	globalDiscovery []GlobalDiscoveryJob
	globalSnapshots []GlobalSnapshotJob
	globalMetrics   []GlobalMetricsJob
	globalDaily     []GlobalDailyJob
	mu              sync.RWMutex
}

// NewScanScheduler returns a ScanScheduler wired to store with empty scanner
// registries. Register scanners before calling Start.
func NewScanScheduler(store *repo.Store) *ScanScheduler {
	return &ScanScheduler{
		store:             store,
		discovery:         make(map[string]DiscoveryScanner),
		discoveryByMethod: make(map[string]DiscoveryScanner),
		metrics:           make(map[string]MetricsScanner),
		metricsByMethod:   make(map[string]MetricsScanner),
		snapshots:         make(map[string]SnapshotScanner),
		snapshotsByMethod: make(map[string]SnapshotScanner),
	}
}

// RegisterDiscovery registers a DiscoveryScanner for the given entity type.
func (s *ScanScheduler) RegisterDiscovery(entityType string, sc DiscoveryScanner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.discovery[entityType] = sc
}

// RegisterDiscoveryByMethod registers a DiscoveryScanner keyed by
// collection_method rather than entity type.  This is used for SNMP hosts
// which may have any entity type but share collection_method="snmp".
func (s *ScanScheduler) RegisterDiscoveryByMethod(collectionMethod string, sc DiscoveryScanner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.discoveryByMethod[collectionMethod] = sc
}

// RegisterMetrics registers a MetricsScanner for the given entity type.
func (s *ScanScheduler) RegisterMetrics(entityType string, sc MetricsScanner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metrics[entityType] = sc
}

// RegisterMetricsByMethod registers a MetricsScanner keyed by collection_method
// rather than entity type. This is used for SNMP hosts which may have any entity
// type but share collection_method="snmp".
func (s *ScanScheduler) RegisterMetricsByMethod(collectionMethod string, sc MetricsScanner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metricsByMethod[collectionMethod] = sc
}

// RegisterSnapshot registers a SnapshotScanner for the given entity type.
func (s *ScanScheduler) RegisterSnapshot(entityType string, sc SnapshotScanner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshots[entityType] = sc
}

// RegisterSnapshotByMethod registers a SnapshotScanner keyed by
// collection_method rather than entity type. This is used for SNMP hosts
// which may have any entity type but share collection_method="snmp".
func (s *ScanScheduler) RegisterSnapshotByMethod(collectionMethod string, sc SnapshotScanner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshotsByMethod[collectionMethod] = sc
}

// RegisterGlobalDiscovery registers a GlobalDiscoveryJob that is called once
// per discovery tick after the per-entity pass.  Use this for hourly
// aggregation jobs that read internal tables rather than polling external
// systems (e.g. the app event metrics collection for sparkline charts).
func (s *ScanScheduler) RegisterGlobalDiscovery(job GlobalDiscoveryJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.globalDiscovery = append(s.globalDiscovery, job)
}

// RegisterGlobalSnapshot registers a GlobalSnapshotJob that is called once per
// snapshot tick after the per-entity pass.  Use this for jobs that iterate
// entities from tables other than infrastructure_components (e.g. SSLSnapshotJob
// which iterates monitor_checks).
func (s *ScanScheduler) RegisterGlobalSnapshot(job GlobalSnapshotJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.globalSnapshots = append(s.globalSnapshots, job)
}

// RegisterGlobalMetrics registers a GlobalMetricsJob that is called once per
// metrics tick after the per-entity pass.  Use this for jobs that run
// independently of infrastructure_components (e.g. the Docker health poller).
func (s *ScanScheduler) RegisterGlobalMetrics(job GlobalMetricsJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.globalMetrics = append(s.globalMetrics, job)
}

// RegisterGlobalDaily registers a GlobalDailyJob that runs once on startup
// and then every 24 hours. Use for rate-limited external calls such as
// container registry digest lookups.
func (s *ScanScheduler) RegisterGlobalDaily(job GlobalDailyJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.globalDaily = append(s.globalDaily, job)
}

// Start launches the four ticker loops and blocks until ctx is cancelled.
// Each ticker fires independently; slow discovery scans will not delay metrics
// collection. Daily jobs run once immediately on start, then every 24 hours.
func (s *ScanScheduler) Start(ctx context.Context) {
	log.Printf("scan scheduler: starting (discovery=%s, metrics=%s, snapshots=%s, daily=%s)",
		DiscoveryInterval, MetricsInterval, SnapshotInterval, DailyInterval)

	discoveryTicker := time.NewTicker(DiscoveryInterval)
	metricsTicker := time.NewTicker(MetricsInterval)
	snapshotTicker := time.NewTicker(SnapshotInterval)
	dailyTicker := time.NewTicker(DailyInterval)
	defer discoveryTicker.Stop()
	defer metricsTicker.Stop()
	defer snapshotTicker.Stop()
	defer dailyTicker.Stop()

	// Delay the first daily pass so enrichment workers (Portainer, Docker Engine)
	// have time to complete their initial cycle and populate image_digest before
	// the registry poller reads it.
	go func() {
		select {
		case <-time.After(3 * time.Minute):
			s.runDailyPass(ctx)
		case <-ctx.Done():
		}
	}()

	for {
		select {
		case <-ctx.Done():
			log.Printf("scan scheduler: context cancelled — stopping")
			return
		case <-discoveryTicker.C:
			go s.runDiscoveryPass(ctx)
		case <-metricsTicker.C:
			go s.runMetricsPass(ctx)
		case <-snapshotTicker.C:
			go s.runSnapshotPass(ctx)
		case <-dailyTicker.C:
			go s.runDailyPass(ctx)
		}
	}
}

// RunDiscovery runs the discovery pass immediately.
// Used by the job registry for on-demand triggering.
func (s *ScanScheduler) RunDiscovery(ctx context.Context) error {
	s.runDiscoveryPass(ctx)
	return nil
}

// RunMetrics runs the metrics collection pass immediately.
// Used by the job registry for on-demand triggering.
func (s *ScanScheduler) RunMetrics(ctx context.Context) error {
	s.runMetricsPass(ctx)
	return nil
}

// RunSnapshot runs the snapshot pass immediately.
// Used by the job registry for on-demand triggering.
func (s *ScanScheduler) RunSnapshot(ctx context.Context) error {
	s.runSnapshotPass(ctx)
	return nil
}

// runDiscoveryPass iterates all enabled components and calls each registered
// DiscoveryScanner concurrently with DiscoveryTimeout per entity.
// Scanners are looked up by entity type first; if none is registered for the
// type, the scheduler falls back to a lookup by collection_method.
func (s *ScanScheduler) runDiscoveryPass(ctx context.Context) {
	components, err := s.listEnabled(ctx)
	if err != nil {
		log.Printf("scan scheduler: discovery: list components: %v", err)
		return
	}

	s.mu.RLock()
	scanners := copyDiscovery(s.discovery)
	methodScanners := copyDiscovery(s.discoveryByMethod)
	s.mu.RUnlock()

	var wg sync.WaitGroup
	for i := range components {
		c := &components[i]
		sc, ok := scanners[c.Type]
		if !ok {
			sc, ok = methodScanners[c.CollectionMethod]
		}
		if !ok {
			continue
		}
		wg.Add(1)
		go func(c *models.InfrastructureComponent, sc DiscoveryScanner) {
			defer wg.Done()
			tctx, cancel := context.WithTimeout(ctx, DiscoveryTimeout)
			defer cancel()
			result, err := sc.Discover(tctx, c.ID, c.Type)
			if err != nil {
				log.Printf("scan scheduler: discovery: %s (%s): %v", c.Name, c.ID, err)
				s.writeErrorEvent(ctx, c, "discovery", err)
				return
			}
			logDiscovery(c, result)
		}(c, sc)
	}
	wg.Wait()

	// Run global discovery jobs (aggregate internal tables rather than poll external systems).
	s.mu.RLock()
	globals := make([]GlobalDiscoveryJob, len(s.globalDiscovery))
	copy(globals, s.globalDiscovery)
	s.mu.RUnlock()
	for _, job := range globals {
		job.Run(ctx)
	}
}

// runMetricsPass iterates all enabled components and calls each registered
// MetricsScanner concurrently with MetricsTimeout per entity.
// Scanners are looked up by entity type first; if none is registered for the
// type the scheduler falls back to a lookup by collection_method.
func (s *ScanScheduler) runMetricsPass(ctx context.Context) {
	components, err := s.listEnabled(ctx)
	if err != nil {
		log.Printf("scan scheduler: metrics: list components: %v", err)
		return
	}

	s.mu.RLock()
	scanners := copyMetrics(s.metrics)
	methodScanners := copyMetrics(s.metricsByMethod)
	s.mu.RUnlock()

	var wg sync.WaitGroup
	for i := range components {
		c := &components[i]
		sc, ok := scanners[c.Type]
		if !ok {
			sc, ok = methodScanners[c.CollectionMethod]
		}
		if !ok {
			continue
		}
		wg.Add(1)
		go func(c *models.InfrastructureComponent, sc MetricsScanner) {
			defer wg.Done()
			tctx, cancel := context.WithTimeout(ctx, MetricsTimeout)
			defer cancel()
			_, err := sc.CollectMetrics(tctx, c.ID, c.Type)
			if err != nil {
				log.Printf("scan scheduler: metrics: %s (%s): %v", c.Name, c.ID, err)
				s.writeErrorEvent(ctx, c, "metrics", err)
			}
		}(c, sc)
	}
	wg.Wait()

	// Run global metrics jobs (operate independently of infrastructure_components).
	s.mu.RLock()
	globals := make([]GlobalMetricsJob, len(s.globalMetrics))
	copy(globals, s.globalMetrics)
	s.mu.RUnlock()
	for _, job := range globals {
		job.Run(ctx)
	}
}

// runSnapshotPass iterates all enabled components and calls each registered
// SnapshotScanner concurrently with SnapshotTimeout per entity.
// Scanners are looked up by entity type first; if none is registered for the
// type the scheduler falls back to a lookup by collection_method.
func (s *ScanScheduler) runSnapshotPass(ctx context.Context) {
	components, err := s.listEnabled(ctx)
	if err != nil {
		log.Printf("scan scheduler: snapshot: list components: %v", err)
		return
	}

	s.mu.RLock()
	scanners := copySnapshots(s.snapshots)
	methodScanners := copySnapshots(s.snapshotsByMethod)
	s.mu.RUnlock()

	var wg sync.WaitGroup
	for i := range components {
		c := &components[i]
		sc, ok := scanners[c.Type]
		if !ok {
			sc, ok = methodScanners[c.CollectionMethod]
		}
		if !ok {
			continue
		}
		wg.Add(1)
		go func(c *models.InfrastructureComponent, sc SnapshotScanner) {
			defer wg.Done()
			tctx, cancel := context.WithTimeout(ctx, SnapshotTimeout)
			defer cancel()
			_, err := sc.TakeSnapshot(tctx, c.ID, c.Type)
			if err != nil {
				log.Printf("scan scheduler: snapshot: %s (%s): %v", c.Name, c.ID, err)
				s.writeErrorEvent(ctx, c, "snapshot", err)
			}
		}(c, sc)
	}
	wg.Wait()

	// Run global snapshot jobs (iterate non-component tables such as monitor_checks).
	s.mu.RLock()
	globals := make([]GlobalSnapshotJob, len(s.globalSnapshots))
	copy(globals, s.globalSnapshots)
	s.mu.RUnlock()
	for _, job := range globals {
		job.Run(ctx)
	}
}

// runDailyPass runs all registered GlobalDailyJobs sequentially.
func (s *ScanScheduler) runDailyPass(ctx context.Context) {
	s.mu.RLock()
	globals := make([]GlobalDailyJob, len(s.globalDaily))
	copy(globals, s.globalDaily)
	s.mu.RUnlock()
	for _, job := range globals {
		job.Run(ctx)
	}
}

// listEnabled returns all enabled infrastructure components from the store.
func (s *ScanScheduler) listEnabled(ctx context.Context) ([]models.InfrastructureComponent, error) {
	all, err := s.store.InfraComponents.List(ctx)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, c := range all {
		if c.Enabled {
			out = append(out, c)
		}
	}
	return out, nil
}

// writeErrorEvent writes an error-level event to the event log for a scan
// failure. The scheduler continues running after a failed scan.
func (s *ScanScheduler) writeErrorEvent(
	ctx context.Context,
	c *models.InfrastructureComponent,
	bucket string,
	scanErr error,
) {
	ev := &models.Event{
		ID:         uuid.New().String(),
		Level:      "error",
		SourceName: c.Name,
		SourceType: "physical_host",
		SourceID:   c.ID,
		Title:      fmt.Sprintf("%s scan failed — %s: %v", bucket, c.Name, scanErr),
		Payload: fmt.Sprintf(
			`{"bucket":%q,"entity_id":%q,"entity_type":%q,"error":%q}`,
			bucket, c.ID, c.Type, scanErr.Error(),
		),
		CreatedAt: time.Now().UTC(),
	}
	if err := s.store.Events.Create(ctx, ev); err != nil {
		log.Printf("scan scheduler: write error event for %s (%s): %v", c.Name, c.ID, err)
	}
}

// logDiscovery emits structured log lines based on what the discovery pass found.
func logDiscovery(c *models.InfrastructureComponent, r *DiscoveryResult) {
	if r.Found == 0 && r.Disappeared == 0 {
		log.Printf("scan scheduler: discovery: %s (%s): no changes", c.Name, c.ID)
		return
	}
	if r.Found > 0 {
		log.Printf("scan scheduler: discovery: %s (%s): %d new/updated entities",
			c.Name, c.ID, r.Found)
	}
	if r.Disappeared > 0 {
		log.Printf("scan scheduler: discovery: %s (%s): %d entities disappeared",
			c.Name, c.ID, r.Disappeared)
	}
}

// copyDiscovery returns a shallow copy of the scanner map so it can be
// iterated without holding the lock.
func copyDiscovery(m map[string]DiscoveryScanner) map[string]DiscoveryScanner {
	out := make(map[string]DiscoveryScanner, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func copyMetrics(m map[string]MetricsScanner) map[string]MetricsScanner {
	out := make(map[string]MetricsScanner, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func copySnapshots(m map[string]SnapshotScanner) map[string]SnapshotScanner {
	out := make(map[string]SnapshotScanner, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
