package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	noraappprofiles "github.com/digitalcheffe/nora/appprofiles"
	"github.com/digitalcheffe/nora/internal/api"
	"github.com/digitalcheffe/nora/internal/apptemplate"
	"github.com/digitalcheffe/nora/internal/auth"
	"github.com/digitalcheffe/nora/internal/config"
	"github.com/digitalcheffe/nora/internal/docker"
	"github.com/digitalcheffe/nora/internal/frontend"
	"github.com/digitalcheffe/nora/internal/ingest"
	"github.com/digitalcheffe/nora/internal/infra"
	"github.com/digitalcheffe/nora/internal/jobs"
	"github.com/digitalcheffe/nora/internal/monitor"
	"github.com/digitalcheffe/nora/internal/push"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/internal/rules"
	"github.com/digitalcheffe/nora/internal/scanner"
	"github.com/digitalcheffe/nora/internal/scanner/discovery"
	noraMetrics "github.com/digitalcheffe/nora/internal/scanner/metrics"
	noraSnapshot "github.com/digitalcheffe/nora/internal/scanner/snapshot"
	"github.com/digitalcheffe/nora/migrations"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	cfg := config.Load()
	startTime := time.Now()

	// File logging — write to NORA_LOG_PATH alongside stdout so logs persist.
	if logPath := os.Getenv("NORA_LOG_PATH"); logPath != "" {
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			log.Printf("warning: could not open log file %s: %v", logPath, err)
		} else {
			defer logFile.Close()
			log.SetOutput(io.MultiWriter(os.Stdout, logFile))
			log.SetFlags(log.LstdFlags | log.Lmicroseconds)
			log.Printf("nora: logging to %s", logPath)
		}
	}

	if err := push.EnsureVAPIDKeys(cfg); err != nil {
		log.Fatalf("VAPID key init failed: %v", err)
	}

	db, err := repo.Open(cfg, migrations.Files)
	if err != nil {
		log.Fatalf("database init failed: %v", err)
	}
	defer db.Close()

	log.Printf("NORA database ready at %s", cfg.DBPath)

	// Repositories
	appRepo := repo.NewAppRepo(db)
	eventRepo := repo.NewEventRepo(db)
	checkRepo := repo.NewCheckRepo(db)
	rollupRepo := repo.NewRollupRepo(db)
	resourceRepo := repo.NewResourceReadingRepo(db)
	resourceRollupRepo := repo.NewResourceRollupRepo(db)
	infraComponentRepo := repo.NewInfraComponentRepo(db)
	dockerEngineRepo := repo.NewDockerEngineRepo(db)
	customProfileRepo := repo.NewCustomProfileRepo(db)
	infraRepo := repo.NewInfraRepo(db)
	settingsRepo := repo.NewSettingsRepo(db)
	metricsRepo := repo.NewMetricsRepo(db)
	userRepo := repo.NewUserRepo(db)
	traefikComponentRepo := repo.NewTraefikComponentRepo(db)
	traefikOverviewRepo := repo.NewTraefikOverviewRepo(db)
	traefikServiceRepo := repo.NewTraefikServiceRepo(db)
	discoveredContainerRepo := repo.NewDiscoveredContainerRepo(db)
	discoveredRouteRepo := repo.NewDiscoveredRouteRepo(db)
	webPushSubscriptionRepo := repo.NewWebPushSubscriptionRepo(db)
	snapshotRepo := repo.NewSnapshotRepo(db)
	ruleRepo := repo.NewRuleRepo(db)
	store := repo.NewStore(
		appRepo, eventRepo, checkRepo,
		rollupRepo, resourceRepo, resourceRollupRepo,
		infraComponentRepo, dockerEngineRepo,
		infraRepo, settingsRepo, metricsRepo, userRepo,
		traefikComponentRepo, traefikOverviewRepo, traefikServiceRepo,
		discoveredContainerRepo, discoveredRouteRepo,
		webPushSubscriptionRepo,
		snapshotRepo,
		ruleRepo,
	)

	// Startup event — written once so users can see when NORA last started.
	jobs.EmitStartupEvent(context.Background(), store)

	// Push notification sender
	pushSender := push.NewSender(cfg, store)

	// Rules engine — evaluates every incoming event against enabled rules.
	// Must be wired before the notifying event repo wrapper below.
	rulesEngine := rules.NewEngine(store, pushSender, cfg)
	// Wrap the event repo so every successful Create fires the rules engine
	// asynchronously. All existing call sites (ingest, docker, monitor, etc.)
	// automatically trigger rule evaluation with no per-callsite changes.
	store.Events = rules.NewNotifyingEventRepo(store.Events, rulesEngine)

	// App template registry — load all bundled YAML app templates
	registry, err := apptemplate.NewRegistry(noraappprofiles.Files)
	if err != nil {
		log.Fatalf("app template registry init failed: %v", err)
	}
	log.Printf("loaded %d app templates", len(registry.List()))

	limiter := ingest.NewRateLimiter()

	// Monitor scheduler — runs all enabled checks on their configured intervals.
	schedCtx, schedCancel := context.WithCancel(context.Background())
	defer schedCancel()
	go monitor.NewScheduler(store).Start(schedCtx)

	// Scan scheduler — Discovery (1h), Metrics (2m), Snapshots (30m).
	// Discovery scanners are registered by entity type (and collection method
	// for SNMP).  Metrics scanners are registered here (REFACTOR-07).
	// Snapshot scanners are added in REFACTOR-08.
	//
	// Note: the Docker MetricsScanner is wired to the ResourcePoller created
	// below so they share the same poller instance; the poller's standalone
	// Start() ticker is therefore NOT started — the scan scheduler drives it.
	scanCtx, scanCancel := context.WithCancel(context.Background())
	defer scanCancel()
	scanScheduler := scanner.NewScanScheduler(store)

	// Discovery
	scanScheduler.RegisterDiscovery("proxmox_node", discovery.NewProxmoxDiscoveryScanner(store))
	scanScheduler.RegisterDiscovery("docker_engine", discovery.NewDockerDiscoveryScanner(store))
	scanScheduler.RegisterDiscovery("synology", discovery.NewSynologyDiscoveryScanner(store))
	scanScheduler.RegisterDiscovery("opnsense", discovery.NewOPNsenseDiscoveryScanner(store))
	scanScheduler.RegisterDiscoveryByMethod("snmp", discovery.NewSNMPDiscoveryScanner(store))

	// Metrics (REFACTOR-07)
	scanScheduler.RegisterMetrics("proxmox_node", noraMetrics.NewProxmoxMetricsScanner(store))
	scanScheduler.RegisterMetrics("synology", noraMetrics.NewSynologyMetricsScanner(store))
	scanScheduler.RegisterMetrics("opnsense", noraMetrics.NewOPNsenseMetricsScanner(store))
	scanScheduler.RegisterMetricsByMethod("snmp", noraMetrics.NewSNMPMetricsScanner(store))
	// Docker MetricsScanner is wired after the ResourcePoller is created below.

	// Snapshots (REFACTOR-08) — 30-minute condition polling.
	// Infrastructure component scanners are registered by entity type / collection method.
	// SSL snapshot scanning runs as a standalone job (iterates monitor_checks, not
	// infrastructure_components) and is started on its own 30-minute ticker below.
	scanScheduler.RegisterSnapshot("proxmox_node", noraSnapshot.NewProxmoxSnapshotScanner(store))
	scanScheduler.RegisterSnapshot("synology", noraSnapshot.NewSynologySnapshotScanner(store))
	scanScheduler.RegisterSnapshot("opnsense", noraSnapshot.NewOPNsenseSnapshotScanner(store))
	scanScheduler.RegisterSnapshotByMethod("snmp", noraSnapshot.NewSNMPSnapshotScanner(store))

	go scanScheduler.Start(scanCtx)

	// SSL snapshot job — runs every SnapshotInterval independently of the scan
	// scheduler because it iterates monitor_checks rather than infrastructure_components.
	sslSnapshotCtx, sslSnapshotCancel := context.WithCancel(context.Background())
	defer sslSnapshotCancel()
	go func() {
		sslJob := noraSnapshot.NewSSLSnapshotJob(store)
		ticker := time.NewTicker(scanner.SnapshotInterval)
		defer ticker.Stop()
		for {
			select {
			case <-sslSnapshotCtx.Done():
				return
			case <-ticker.C:
				sslJob.Run(sslSnapshotCtx)
			}
		}
	}()

	// Resource rollup jobs — hourly aggregation and daily rollup + retention purge.
	rollupCtx, rollupCancel := context.WithCancel(context.Background())
	defer rollupCancel()
	go jobs.StartHourlyRollup(rollupCtx, store)
	go jobs.StartDailyRollup(rollupCtx, store)

	// Event jobs — monthly rollup (midnight on 1st), nightly retention (02:00), hourly metrics.
	eventJobCtx, eventJobCancel := context.WithCancel(context.Background())
	defer eventJobCancel()
	go jobs.StartMonthlyRollup(eventJobCtx, store)
	go jobs.StartEventRetention(eventJobCtx, store)
	go jobs.StartMetricsCollection(eventJobCtx, store)

	// Digest job — fires at 08:00 daily; checks stored schedule before sending.
	digestJob := jobs.NewDigestJob(store, cfg)
	digestCtx, digestCancel := context.WithCancel(context.Background())
	defer digestCancel()
	go jobs.StartDigestJob(digestCtx, digestJob)

	// Traefik sync worker — polls all enabled Traefik integrations every 60 s.
	infraCtx, infraCancel := context.WithCancel(context.Background())
	defer infraCancel()
	syncWorker := infra.NewSyncWorker(store)
	go syncWorker.Start(infraCtx)

	// Proxmox pollers — polls all enabled proxmox_node components every 5 minutes.
	proxmoxCtx, proxmoxCancel := context.WithCancel(context.Background())
	defer proxmoxCancel()
	go jobs.StartProxmoxPollers(proxmoxCtx, store)

	// Synology pollers — polls all enabled synology components every 5 minutes.
	synologyCtx, synologyCancel := context.WithCancel(context.Background())
	defer synologyCancel()
	go jobs.StartSynologyPollers(synologyCtx, store)

	// SNMP pollers — polls all enabled snmp components every 5 minutes.
	snmpCtx, snmpCancel := context.WithCancel(context.Background())
	defer snmpCancel()
	go jobs.StartSNMPPollers(snmpCtx, store)

	// Traefik component pollers — polls all enabled traefik components every 5 minutes.
	traefikCtx, traefikCancel := context.WithCancel(context.Background())
	defer traefikCancel()
	go jobs.StartTraefikComponentPollers(traefikCtx, store)

	// Docker socket watcher and resource poller — optional; skipped if the socket is not available.
	dockerCtx, dockerCancel := context.WithCancel(context.Background())
	defer dockerCancel()

	// Ensure a local docker engine infrastructure component exists so discovered
	// containers can be associated with it. This is idempotent — it reuses the
	// existing record if one with type="docker_engine" is already present.
	localEngineID, err := docker.EnsureLocalInfraComponent(context.Background(), store)
	if err != nil {
		log.Printf("docker discovery: could not ensure local engine record: %v", err)
	}

	if watcher, err := docker.NewWatcher(store); err != nil {
		log.Printf("docker watcher: socket not available, skipping (%v)", err)
	} else {
		// Wire up health poller so start events trigger an immediate health check.
		if healthPoller, err := docker.NewHealthPoller(store); err != nil {
			log.Printf("docker health poller: socket not available, skipping (%v)", err)
		} else {
			watcher.SetContainerStartHook(healthPoller.CheckContainer)
			go healthPoller.Start(dockerCtx)
		}

		// Wire up the discovery worker to upsert containers and run profile matching.
		if localEngineID != "" {
			if discoverer, err := docker.NewDiscoverer(store, registry, localEngineID); err != nil {
				log.Printf("docker discoverer: %v", err)
			} else {
				watcher.SetDiscoveryHook(discoverer.HandleEvent)
				go discoverer.ScanAll(dockerCtx)
			}
		}

		go watcher.Start(dockerCtx)
	}

	// Image update poller — checks container registries daily at 02:00 UTC for
	// newer image versions.  Skipped if the Docker socket is not available.
	imagePollerCtx, imagePollerCancel := context.WithCancel(context.Background())
	defer imagePollerCancel()
	if imagePoller, err := docker.NewImageUpdatePoller(store); err != nil {
		log.Printf("image update poller: socket not available, skipping (%v)", err)
	} else {
		go imagePoller.Start(imagePollerCtx)
	}

	// Docker ResourcePoller — metrics collection is driven by the scan scheduler
	// (every 2 minutes via DockerMetricsScanner) rather than a standalone ticker.
	// The poller is registered with the scheduler so PollAll is called on the
	// MetricsInterval instead of the legacy 60-second loop.
	if resourcePoller, err := docker.NewResourcePoller(store, localEngineID); err != nil {
		log.Printf("resource poller: socket not available, skipping (%v)", err)
	} else {
		scanScheduler.RegisterMetrics("docker_engine",
			noraMetrics.NewDockerMetricsScanner(store, localEngineID, resourcePoller))
	}

	// Router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Public routes — no session auth
	r.Post("/api/v1/ingest/{token}", api.HandleIngest(store, registry, limiter))
	pushHandler := api.NewPushHandler(cfg, store, pushSender)
	pushHandler.RegisterPublicRoutes(r)

	// API v1 — protected by auth middleware
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(auth.RequireAuth(cfg.DevMode))
		api.NewAppsHandler(appRepo).Routes(r)
		api.NewEventsHandler(eventRepo).Routes(r)
		api.NewChecksHandler(checkRepo, eventRepo).Routes(r)
		api.NewDashboardHandler(appRepo, eventRepo, checkRepo, rollupRepo, registry).Routes(r)
		api.NewTopologyHandler(infraComponentRepo, dockerEngineRepo, appRepo, resourceRollupRepo).Routes(r)
		api.NewInfraComponentHandler(infraComponentRepo, resourceRollupRepo, checkRepo, eventRepo, store).Routes(r)
		api.NewProfilesHandler(registry, customProfileRepo).Routes(r)
		api.NewInfraHandler(infraRepo, syncWorker).Routes(r)
		api.NewDockerDiscoveryHandler(store, registry).Routes(r)
		api.NewDigestHandler(store, digestJob).Routes(r)
		api.NewSettingsHandler(store).Routes(r)
		api.NewIntegrationDriversHandler(settingsRepo).Routes(r)
		api.NewMetricsHandler(eventRepo, appRepo, metricsRepo, cfg.DBPath, startTime).Routes(r)
		api.NewUsersHandler(userRepo).Routes(r)
		api.NewProxmoxDetailHandler(infraComponentRepo).Routes(r)
		pushHandler.Routes(r)
		api.NewRulesHandler(store, rulesEngine).Routes(r)
	})

	// Frontend — serve embedded React app, SPA fallback to index.html
	distFS, err := fs.Sub(frontend.Dist, "dist")
	if err != nil {
		log.Fatalf("frontend embed: %v", err)
	}
	// Only register the static handler if index.html was actually embedded.
	if _, err := distFS.Open("index.html"); err == nil {
		fileServer := http.FileServer(http.FS(distFS))
		r.Get("/*", spaHandler(distFS, fileServer))
		log.Printf("serving frontend from embedded dist")
	} else {
		log.Printf("no embedded frontend found — API-only mode")
	}

	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("NORA listening on %s (dev_mode=%v)", addr, cfg.DevMode)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// spaHandler serves static files when they exist, and falls back to index.html
// for all other paths so React Router can handle client-side navigation.
func spaHandler(distFS fs.FS, fileServer http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Strip the leading slash for FS lookups.
		path := r.URL.Path
		if len(path) > 0 && path[0] == '/' {
			path = path[1:]
		}
		if path == "" {
			path = "index.html"
		}

		// If the file exists in the embedded FS, serve it directly.
		if f, err := distFS.Open(path); err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// Otherwise fall back to index.html for SPA routing.
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	}
}
