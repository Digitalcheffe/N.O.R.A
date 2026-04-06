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
	"github.com/digitalcheffe/nora/internal/frontend"
	"github.com/digitalcheffe/nora/internal/icons"
	"github.com/digitalcheffe/nora/internal/ingest"
	"github.com/digitalcheffe/nora/internal/infra"
	"github.com/digitalcheffe/nora/internal/jobs"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/monitor"
	"github.com/digitalcheffe/nora/internal/profile"
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

const version = "1.1.0"

func main() {
	cfg := config.Load()
	startTime := time.Now()

	log.Printf("================================================")
	log.Printf("  N.O.R.A  v%s", version)
	log.Printf("  Nexus Operations Recon & Alerts")
	log.Printf("================================================")

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

	log.Printf("  database : %s", cfg.DBPath)

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
	digestRegistryRepo := repo.NewDigestRegistryRepo(db)
	appMetricSnapshotRepo := repo.NewAppMetricSnapshotRepo(db)
	componentLinkRepo := repo.NewComponentLinkRepo(db)
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
		digestRegistryRepo,
		appMetricSnapshotRepo,
		componentLinkRepo,
	)

	// Bootstrap admin account — only runs when the users table is empty and
	// NORA_ADMIN_EMAIL + NORA_ADMIN_PASSWORD are both set.
	if cfg.AdminEmail != "" && cfg.AdminPassword != "" {
		if n, err := userRepo.Count(context.Background()); err == nil && n == 0 {
			if u, err := seedAdmin(context.Background(), userRepo, cfg.AdminEmail, cfg.AdminPassword); err != nil {
				log.Printf("admin bootstrap failed: %v", err)
			} else {
				log.Printf("admin bootstrap: created admin account for %s", cfg.AdminEmail)
				// Bootstrap admin is exempt from global MFA enforcement.
				if exemptErr := userRepo.SetTOTPExempt(context.Background(), u.ID, true); exemptErr != nil {
					log.Printf("admin bootstrap: could not set TOTP exempt: %v", exemptErr)
				}
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
	log.Printf("  templates: %d loaded from %s", len(registry.List()), cfg.TemplatesPath)

	// Digest registry reconciliation — runs synchronously at startup.
	// Inserts/updates entries for all profile digest categories and deactivates orphans.
	reconciler := profile.NewRegistryReconciler(digestRegistryRepo, registry)
	if err := reconciler.Reconcile(context.Background()); err != nil {
		log.Printf("digest registry reconcile: %v", err)
	}

	// Icon fetcher — downloads and caches SVG icons from dashboard-icons CDN.
	iconFetcher, err := icons.New(cfg.IconsPath)
	if err != nil {
		log.Printf("icon fetcher init failed: %v — icons disabled", err)
		iconFetcher = nil
	}
	if iconFetcher != nil {
		// Pre-fetch icons for all existing apps in the background.
		// Build a slug override map from the app template registry so the fetcher
		// can try the template's icon: slug before falling back to the profileID.
		existingApps, err := appRepo.List(context.Background())
		if err == nil {
			profileIDs := make([]string, 0, len(existingApps))
			slugOverrides := make(map[string]string, len(existingApps))
			for _, a := range existingApps {
				profileIDs = append(profileIDs, a.ProfileID)
				if t, err := registry.Get(a.ProfileID); err == nil && t != nil && t.Meta.Icon != "" {
					slugOverrides[a.ProfileID] = t.Meta.Icon
				}
			}
			iconFetcher.FetchAll(context.Background(), profileIDs, slugOverrides)
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
	scanScheduler.RegisterDiscovery("traefik", discovery.NewTraefikDiscoveryScanner(store))
	scanScheduler.RegisterDiscoveryByMethod("snmp", discovery.NewSNMPDiscoveryScanner(store))
	scanScheduler.RegisterGlobalDiscovery(scanner.GlobalDiscoveryFunc(func(ctx context.Context) {
		if err := discovery.RunHourlyRollup(ctx, store); err != nil {
			log.Printf("hourly rollup: discovery pass: %v", err)
		}
	}))
	scanScheduler.RegisterGlobalDiscovery(scanner.GlobalDiscoveryFunc(func(ctx context.Context) {
		if err := discovery.RunMetricsCollection(ctx, store); err != nil {
			log.Printf("metrics collection: discovery pass: %v", err)
		}
	}))
	scanScheduler.RegisterGlobalDiscovery(scanner.GlobalDiscoveryFunc(func(ctx context.Context) {
		if err := discovery.RunAPIPolling(ctx, store, registry); err != nil {
			log.Printf("api polling: discovery pass: %v", err)
		}
	}))

	// Metrics (REFACTOR-07)
	scanScheduler.RegisterMetrics("proxmox_node", noraMetrics.NewProxmoxMetricsScanner(store))
	scanScheduler.RegisterMetrics("synology", noraMetrics.NewSynologyMetricsScanner(store))
	scanScheduler.RegisterMetrics("opnsense", noraMetrics.NewOPNsenseMetricsScanner(store))
	scanScheduler.RegisterMetrics("traefik", noraMetrics.NewTraefikMetricsScanner(store))
	scanScheduler.RegisterMetrics("portainer", noraMetrics.NewPortainerMetricsScanner(store))
	scanScheduler.RegisterMetricsByMethod("snmp", noraMetrics.NewSNMPMetricsScanner(store))
	// Docker MetricsScanner is wired after the ResourcePoller is created below.

	// Snapshots (REFACTOR-08) — 30-minute condition polling.
	// Infrastructure component scanners are registered by entity type / collection method.
	// SSL snapshot and monitor-check snapshot run as global snapshot jobs (they iterate
	// monitor_checks rather than infrastructure_components) and are called at the end of
	// each snapshot tick by the scheduler.
	scanScheduler.RegisterSnapshot("proxmox_node", noraSnapshot.NewProxmoxSnapshotScanner(store))
	scanScheduler.RegisterSnapshot("synology", noraSnapshot.NewSynologySnapshotScanner(store))
	scanScheduler.RegisterSnapshot("opnsense", noraSnapshot.NewOPNsenseSnapshotScanner(store))
	scanScheduler.RegisterSnapshot("traefik", noraSnapshot.NewTraefikSnapshotScanner(store))
	scanScheduler.RegisterSnapshotByMethod("snmp", noraSnapshot.NewSNMPSnapshotScanner(store))
	scanScheduler.RegisterGlobalSnapshot(noraSnapshot.NewSSLSnapshotJob(store))

	go scanScheduler.Start(scanCtx)

	// Resource rollup jobs — daily rollup + retention purge.
	// Hourly rollup is driven by the scan scheduler's discovery pass.
	rollupCtx, rollupCancel := context.WithCancel(context.Background())
	defer rollupCancel()
	go jobs.StartDailyRollup(rollupCtx, store)

	// Event jobs — monthly rollup (midnight on 1st), nightly retention (02:00).
	// Hourly metrics collection is driven by the scan scheduler's discovery pass.
	eventJobCtx, eventJobCancel := context.WithCancel(context.Background())
	defer eventJobCancel()
	go jobs.StartMonthlyRollup(eventJobCtx, store)
	go jobs.StartEventRetention(eventJobCtx, store)

	// Digest job — fires at 08:00 daily; checks stored schedule before sending.
	digestJob := jobs.NewDigestJob(store, cfg, registry)
	digestCtx, digestCancel := context.WithCancel(context.Background())
	defer digestCancel()
	go jobs.StartDigestJob(digestCtx, digestJob)

	// Sync worker — kept for InfraHandler manual-sync endpoint; background polling removed.
	syncWorker := infra.NewSyncWorker(store)

	// Portainer enrichment worker — container discovery and image-update detection.
	// Runs as a GlobalSnapshotJob on the 30-minute snapshot interval rather than
	// a standalone ticker.
	portainerWorker := infra.NewPortainerEnrichmentWorker(store)
	scanScheduler.RegisterGlobalSnapshot(scanner.GlobalSnapshotFunc(func(ctx context.Context) {
		if err := portainerWorker.Run(ctx); err != nil {
			log.Printf("portainer enrichment: snapshot run: %v", err)
		}
	}))

	// Docker socket watcher and resource poller — optional; skipped if the socket is not available.
	dockerCtx, dockerCancel := context.WithCancel(context.Background())
	defer dockerCancel()

	// Ensure a local docker engine infrastructure component exists so discovered
	// containers can be associated with it. This is idempotent — it reuses the
	// existing record if one with type="docker_engine" is already present.
	localEngineID, err := infra.EnsureLocalInfraComponent(context.Background(), store)
	if err != nil {
		log.Printf("docker discovery: could not ensure local engine record: %v", err)
	}

	if watcher, err := infra.NewWatcher(store); err != nil {
		log.Printf("docker watcher: socket not available, skipping (%v)", err)
	} else {
		// Wire up health poller — immediate check on start events; periodic polling
		// is driven by the scan scheduler's metrics pass (every 60 s).
		if healthPoller, err := noraMetrics.NewHealthPoller(store); err != nil {
			log.Printf("docker health poller: socket not available, skipping (%v)", err)
		} else {
			watcher.SetContainerStartHook(healthPoller.CheckContainer)
			scanScheduler.RegisterGlobalMetrics(healthPoller)
		}

		// Wire up the discovery worker to upsert containers and run profile matching.
		if localEngineID != "" {
			if discoverer, err := infra.NewDiscoverer(store, registry, localEngineID); err != nil {
				log.Printf("docker discoverer: %v", err)
			} else {
				watcher.SetDiscoveryHook(discoverer.HandleEvent)
				go discoverer.ScanAll(dockerCtx)
			}
		}

		go watcher.Start(dockerCtx)
	}

	// Image update poller — checks container registries for newer image versions.
	// Runs as a GlobalSnapshotJob on the 30-minute snapshot interval.
	// Skipped if the Docker socket is not available.
	var imagePoller *noraSnapshot.ImageUpdatePoller
	if p, err := noraSnapshot.NewImageUpdatePoller(store); err != nil {
		log.Printf("image update poller: socket not available, skipping (%v)", err)
	} else {
		imagePoller = p
		scanScheduler.RegisterGlobalSnapshot(scanner.GlobalSnapshotFunc(func(ctx context.Context) {
			if err := imagePoller.Run(ctx); err != nil {
				log.Printf("image update poller: snapshot run: %v", err)
			}
		}))
	}

	// Docker ResourcePoller — metrics collection is driven by the scan scheduler
	// (every 60 s via DockerMetricsScanner) rather than a standalone ticker.
	// The poller is registered with the scheduler so PollAll is called on the
	// MetricsInterval instead of the legacy 60-second loop.
	if resourcePoller, err := noraMetrics.NewResourcePoller(store, localEngineID); err != nil {
		log.Printf("resource poller: socket not available, skipping (%v)", err)
	} else {
		scanScheduler.RegisterMetrics("docker_engine",
			noraMetrics.NewDockerMetricsScanner(store, localEngineID, resourcePoller))
	}

	// Job registry — every background job is registered here so it can be
	// listed and triggered on-demand via the /api/v1/jobs endpoints.
	jobRegistry := jobs.NewRegistry()

	// MONITOR — single job runs all enabled check types.
	jobRegistry.Register(&jobs.JobEntry{
		ID: "run_monitors", Name: "Run All Monitors", Category: "monitor",
		Description: "Runs all enabled ping, URL, SSL, and DNS checks immediately.",
		RunFn:       monitorScheduler.RunAll,
	})

	// SCAN — one job per scan engine bucket (metrics, snapshots, discovery).
	// Each pass includes all per-entity scanners and any registered global jobs.
	jobRegistry.Register(&jobs.JobEntry{
		ID: "scan_metrics", Name: "Metrics Scan", Category: "scan",
		Description: "Collects CPU, memory, uptime, and container metrics from all enabled infrastructure.",
		RunFn:       scanScheduler.RunMetrics,
	})
	jobRegistry.Register(&jobs.JobEntry{
		ID: "scan_snapshots", Name: "Snapshot Scan", Category: "scan",
		Description: "Polls health, firmware versions, SSL certs, and container image updates.",
		RunFn:       scanScheduler.RunSnapshot,
	})
	jobRegistry.Register(&jobs.JobEntry{
		ID: "scan_discovery", Name: "Discovery Scan", Category: "scan",
		Description: "Discovers resources across all enabled infrastructure and runs hourly rollups.",
		RunFn:       scanScheduler.RunDiscovery,
	})

	// DATA — aggregation and retention jobs.
	jobRegistry.Register(&jobs.JobEntry{
		ID: "daily_resource_rollup", Name: "Daily Resource Rollup", Category: "data",
		Description: "Collapses hourly rollups into daily summaries for long-term trending.",
		RunFn:       func(ctx context.Context) error { return jobs.RunDailyRollup(ctx, store) },
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
		ID: "monthly_digest", Name: "Digest", Category: "data",
		Description: "Sends the digest email for the current period.",
		RunFn: func(ctx context.Context) error {
			return digestJob.Send(ctx, time.Now().UTC().Format("2006-01"))
		},
	})

	// Router
	r := chi.NewRouter()
	if cfg.IsDebug() {
		r.Use(middleware.Logger)
	}
	r.Use(middleware.Recoverer)

	// Health check — public, no auth required
	r.Get("/api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Public routes — no session auth
	r.Post("/api/v1/ingest/{token}", api.HandleIngest(store, registry, limiter))
	pushHandler := api.NewPushHandler(cfg, store, pushSender)
	pushHandler.RegisterPublicRoutes(r)
	authHandler := api.NewAuthHandler(userRepo, store.Settings, cfg.Secret)
	authHandler.RegisterPublicRoutes(r)
	totpHandler := api.NewTOTPHandler(userRepo, cfg.Secret)
	totpHandler.RegisterPublicRoutes(r)

	// API v1 — protected by auth middleware
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(auth.RequireAuth(cfg.Secret))
		api.NewAppsHandler(appRepo, iconFetcher, checkRepo, registry, appMetricSnapshotRepo).Routes(r)
		if iconFetcher != nil {
			api.NewIconsHandler(iconFetcher, registry).Routes(r)
		}
		api.NewEventsHandler(eventRepo).Routes(r)
		api.NewChecksHandler(checkRepo, eventRepo, monitorScheduler).Routes(r)
		api.NewDashboardHandler(appRepo, eventRepo, checkRepo, rollupRepo, registry).Routes(r)
		api.NewTopologyHandler(infraComponentRepo, dockerEngineRepo, appRepo, resourceRollupRepo, componentLinkRepo).Routes(r)
		api.NewInfraComponentHandler(infraComponentRepo, resourceRollupRepo, checkRepo, eventRepo, store).Routes(r)
		api.NewProfilesHandler(registry, customDir).Routes(r)
		api.NewInfraHandler(infraRepo, syncWorker).Routes(r)
		api.NewDockerDiscoveryHandler(store, registry).Routes(r)
			api.NewDockerSummaryHandler(store).Routes(r)
		api.NewDigestHandler(store, digestJob).Routes(r)
		api.NewSettingsHandler(store).Routes(r)
		api.NewIntegrationDriversHandler(settingsRepo).Routes(r)
		api.NewMetricsHandler(eventRepo, appRepo, metricsRepo, db, cfg.DBPath, startTime, version).Routes(r)
		api.NewUsersHandler(userRepo, store.Settings).Routes(r)
		totpHandler.Routes(r)
		api.NewProxmoxDetailHandler(infraComponentRepo).Routes(r)
		pushHandler.Routes(r)
		api.NewRulesHandler(store, rulesEngine).Routes(r)
		api.NewJobsHandler(jobRegistry).Routes(r)
		api.NewDigestRegistryHandler(store).Routes(r)
		api.NewPortainerHandler(store).Routes(r)
		api.NewLinksHandler(store).Routes(r)
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
	log.Printf("  port     : %s", cfg.Port)
	log.Printf("  log level: %s", cfg.LogLevel)
	log.Printf("================================================")

	// Start server in background so we can run a startup health check.
	go func() {
		if err := http.ListenAndServe(addr, r); err != nil {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Startup health check — verify the server is accepting connections.
	healthURL := fmt.Sprintf("http://localhost:%s/api/v1/health", cfg.Port)
	for i := 0; i < 10; i++ {
		time.Sleep(200 * time.Millisecond)
		resp, err := http.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				log.Printf("  health   : OK — ready in %s", time.Since(startTime).Round(time.Millisecond))
				log.Printf("================================================")
				break
			}
		}
		if i == 9 {
			log.Printf("  health   : WARN — server did not respond after startup")
			log.Printf("================================================")
		}
	}

	// Block forever.
	select {}
}

// seedAdmin creates the first admin user from the bootstrap env vars.
// It must only be called when the users table is empty.
func seedAdmin(ctx context.Context, users repo.UserRepo, email, password string) (*models.User, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	u := &models.User{
		ID:    uuid.NewString(),
		Email: email,
		Role:  "admin",
	}
	if err := users.Create(ctx, u, string(hashed)); err != nil {
		return nil, err
	}
	return u, nil
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

		// Service worker must never be cached so updates are picked up immediately.
		if path == "sw.js" {
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
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
