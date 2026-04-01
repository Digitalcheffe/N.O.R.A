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
	"github.com/digitalcheffe/nora/internal/icons"
	"github.com/digitalcheffe/nora/internal/ingest"
	"github.com/digitalcheffe/nora/internal/infra"
	"github.com/digitalcheffe/nora/internal/jobs"
	"github.com/digitalcheffe/nora/internal/models"
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
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
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

	// Bootstrap admin account — only runs when the users table is empty and
	// NORA_ADMIN_EMAIL + NORA_ADMIN_PASSWORD are both set.
	if cfg.AdminEmail != "" && cfg.AdminPassword != "" {
		if n, err := userRepo.Count(context.Background()); err == nil && n == 0 {
			if err := seedAdmin(context.Background(), userRepo, cfg.AdminEmail, cfg.AdminPassword); err != nil {
				log.Printf("admin bootstrap failed: %v", err)
			} else {
				log.Printf("admin bootstrap: created admin account for %s", cfg.AdminEmail)
			}
		}
	}

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

	// App template registry — export embedded templates to disk then load from disk.
	builtinDir := cfg.TemplatesPath + "/builtin"
	customDir := cfg.TemplatesPath + "/custom"
	if err := os.MkdirAll(customDir, 0755); err != nil {
		log.Fatalf("create custom templates dir: %v", err)
	}
	if err := apptemplate.ExportBuiltins(noraappprofiles.Files, builtinDir); err != nil {
		log.Fatalf("export builtin templates: %v", err)
	}
	registry, err := apptemplate.NewRegistryFromDisk(builtinDir, customDir)
	if err != nil {
		log.Fatalf("app template registry init failed: %v", err)
	}
	log.Printf("loaded %d app templates from %s", len(registry.List()), cfg.TemplatesPath)

	// Icon fetcher — downloads and caches SVG icons from dashboard-icons CDN.
	iconFetcher, err := icons.New(cfg.IconsPath)
	if err != nil {
		log.Printf("icon fetcher init failed: %v — icons disabled", err)
		iconFetcher = nil
	}
	if iconFetcher != nil {
		// Pre-fetch icons for all existing apps in the background.
		existingApps, err := appRepo.List(context.Background())
		if err == nil {
			profileIDs := make([]string, 0, len(existingApps))
			for _, a := range existingApps {
				profileIDs = append(profileIDs, a.ProfileID)
			}
			iconFetcher.FetchAll(context.Background(), profileIDs)
		}
	}

	limiter := ingest.NewRateLimiter()

	// Monitor scheduler — runs all enabled checks on their configured intervals.
	schedCtx, schedCancel := context.WithCancel(context.Background())
	defer schedCancel()
	monitorScheduler := monitor.NewScheduler(store)
	go monitorScheduler.Start(schedCtx)

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
	digestJob := jobs.NewDigestJob(store, cfg, registry)
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

	// Portainer enrichment worker — polls all enabled Portainer components every 15 minutes.
	// Does not require a local Docker Engine component; gate is Portainer-component presence only.
	portainerCtx, portainerCancel := context.WithCancel(context.Background())
	defer portainerCancel()
	portainerWorker := infra.NewPortainerEnrichmentWorker(store)
	go portainerWorker.Start(portainerCtx)

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
	var imagePoller *docker.ImageUpdatePoller
	if p, err := docker.NewImageUpdatePoller(store); err != nil {
		log.Printf("image update poller: socket not available, skipping (%v)", err)
	} else {
		imagePoller = p
		go imagePoller.StartEvery(imagePollerCtx, scanner.DiscoveryInterval)
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

	// Job registry — every background job is registered here so it can be
	// listed and triggered on-demand via the /api/v1/jobs endpoints.
	jobRegistry := jobs.NewRegistry()

	// MONITOR — per-check-type runners.
	jobRegistry.Register(&jobs.JobEntry{
		ID: "ping_checks", Name: "Ping Checks", Category: "monitor",
		Description: "Runs all enabled ping checks immediately.",
		RunFn:       func(ctx context.Context) error { return monitorScheduler.RunAllByType(ctx, "ping") },
	})
	jobRegistry.Register(&jobs.JobEntry{
		ID: "url_checks", Name: "URL Checks", Category: "monitor",
		Description: "Runs all enabled URL checks immediately.",
		RunFn:       func(ctx context.Context) error { return monitorScheduler.RunAllByType(ctx, "url") },
	})
	jobRegistry.Register(&jobs.JobEntry{
		ID: "ssl_checks", Name: "SSL Checks", Category: "monitor",
		Description: "Evaluates certificate expiry for all SSL checks.",
		RunFn:       func(ctx context.Context) error { return monitorScheduler.RunAllByType(ctx, "ssl") },
	})
	jobRegistry.Register(&jobs.JobEntry{
		ID: "dns_checks", Name: "DNS Checks", Category: "monitor",
		Description: "Runs all enabled DNS checks immediately.",
		RunFn:       func(ctx context.Context) error { return monitorScheduler.RunAllByType(ctx, "dns") },
	})

	// DATA — aggregation and retention jobs.
	jobRegistry.Register(&jobs.JobEntry{
		ID: "resource_rollup", Name: "Resource Rollup", Category: "data",
		Description: "Collapses raw resource readings into hourly summary rollups.",
		RunFn:       func(ctx context.Context) error { return jobs.RunHourlyRollup(ctx, store) },
	})
	jobRegistry.Register(&jobs.JobEntry{
		ID: "event_retention", Name: "Event Retention Purge", Category: "data",
		Description: "Deletes expired events per severity retention rules.",
		RunFn:       func(ctx context.Context) error { return jobs.RunEventRetention(ctx, store) },
	})
	jobRegistry.Register(&jobs.JobEntry{
		ID: "monthly_rollup", Name: "Monthly Rollup", Category: "data",
		Description: "Aggregates events from the previous calendar month into rollup counts.",
		RunFn:       func(ctx context.Context) error { return jobs.RunMonthlyRollup(ctx, store) },
	})
	jobRegistry.Register(&jobs.JobEntry{
		ID: "monthly_digest", Name: "Monthly Digest", Category: "data",
		Description: "Sends the digest email for the current period.",
		RunFn: func(ctx context.Context) error {
			return digestJob.Send(ctx, time.Now().UTC().Format("2006-01"))
		},
	})

	// INTEGRATION — infrastructure pollers and scan passes.
	jobRegistry.Register(&jobs.JobEntry{
		ID: "traefik_sync", Name: "Traefik Sync", Category: "integration",
		Description: "Syncs certificate data from all enabled Traefik integrations.",
		RunFn:       syncWorker.RunSync,
	})
	jobRegistry.Register(&jobs.JobEntry{
		ID: "traefik_pollers", Name: "Traefik Component Pollers", Category: "integration",
		Description: "Polls routes and service health from all enabled Traefik components.",
		RunFn: func(ctx context.Context) error {
			jobs.RunTraefikComponentPollers(ctx, store)
			return nil
		},
	})
	jobRegistry.Register(&jobs.JobEntry{
		ID: "proxmox_pollers", Name: "Proxmox Pollers", Category: "integration",
		Description: "Polls status and metrics from all enabled Proxmox nodes.",
		RunFn: func(ctx context.Context) error {
			jobs.RunProxmoxPollers(ctx, store)
			return nil
		},
	})
	jobRegistry.Register(&jobs.JobEntry{
		ID: "synology_pollers", Name: "Synology Pollers", Category: "integration",
		Description: "Polls status and storage info from all enabled Synology components.",
		RunFn: func(ctx context.Context) error {
			jobs.RunSynologyPollers(ctx, store, make(map[string]*infra.SynologyPoller))
			return nil
		},
	})
	jobRegistry.Register(&jobs.JobEntry{
		ID: "snmp_pollers", Name: "SNMP Pollers", Category: "integration",
		Description: "Polls all enabled SNMP targets for metrics and status.",
		RunFn: func(ctx context.Context) error {
			jobs.RunSNMPPollers(ctx, store)
			return nil
		},
	})
	jobRegistry.Register(&jobs.JobEntry{
		ID: "scan_discovery", Name: "Infrastructure Discovery", Category: "integration",
		Description: "Discovers resources across all enabled infrastructure components.",
		RunFn:       scanScheduler.RunDiscovery,
	})
	jobRegistry.Register(&jobs.JobEntry{
		ID: "scan_metrics", Name: "Infrastructure Metrics", Category: "integration",
		Description: "Collects metrics from all enabled infrastructure components.",
		RunFn:       scanScheduler.RunMetrics,
	})
	jobRegistry.Register(&jobs.JobEntry{
		ID: "scan_snapshots", Name: "Infrastructure Snapshots", Category: "integration",
		Description: "Polls health status from all enabled infrastructure components.",
		RunFn:       scanScheduler.RunSnapshot,
	})
	if imagePoller != nil {
		jobRegistry.Register(&jobs.JobEntry{
			ID: "docker_image_scan", Name: "Docker Image Scan", Category: "integration",
			Description: "Checks all running containers for available image updates.",
			RunFn:       imagePoller.Run,
		})
	}
	jobRegistry.Register(&jobs.JobEntry{
		ID: "portainer_enrichment", Name: "Portainer Enrichment", Category: "integration",
		Description: "Matches Portainer containers to NORA-known records and updates image update status.",
		RunFn:       portainerWorker.Run,
	})

	// SYSTEM — instance-level background jobs.
	jobRegistry.Register(&jobs.JobEntry{
		ID: "metrics_collection", Name: "Metrics Collection", Category: "system",
		Description: "Recalculates per-app event throughput metrics.",
		RunFn:       func(ctx context.Context) error { return jobs.RunMetricsCollection(ctx, store) },
	})

	// Router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Public routes — no session auth
	r.Post("/api/v1/ingest/{token}", api.HandleIngest(store, registry, limiter))
	pushHandler := api.NewPushHandler(cfg, store, pushSender)
	pushHandler.RegisterPublicRoutes(r)
	authHandler := api.NewAuthHandler(userRepo, cfg.Secret)
	authHandler.RegisterPublicRoutes(r)

	// API v1 — protected by auth middleware
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(auth.RequireAuth(cfg.Secret))
		api.NewAppsHandler(appRepo, iconFetcher).Routes(r)
		if iconFetcher != nil {
			api.NewIconsHandler(iconFetcher).Routes(r)
		}
		api.NewEventsHandler(eventRepo).Routes(r)
		api.NewChecksHandler(checkRepo, eventRepo).Routes(r)
		api.NewDashboardHandler(appRepo, eventRepo, checkRepo, rollupRepo, registry).Routes(r)
		api.NewTopologyHandler(infraComponentRepo, dockerEngineRepo, appRepo, resourceRollupRepo).Routes(r)
		api.NewInfraComponentHandler(infraComponentRepo, resourceRollupRepo, checkRepo, eventRepo, store).Routes(r)
		api.NewProfilesHandler(registry, customDir).Routes(r)
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
		api.NewJobsHandler(jobRegistry).Routes(r)
		api.NewPortainerHandler(store).Routes(r)
		authHandler.Routes(r)
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
	log.Printf("NORA listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// seedAdmin creates the first admin user from the bootstrap env vars.
// It must only be called when the users table is empty.
func seedAdmin(ctx context.Context, users repo.UserRepo, email, password string) error {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u := &models.User{
		ID:    uuid.NewString(),
		Email: email,
		Role:  "admin",
	}
	return users.Create(ctx, u, string(hashed))
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
