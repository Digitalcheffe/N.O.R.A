package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
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
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/migrations"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	cfg := config.Load()
	startTime := time.Now()

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
	discoveredContainerRepo := repo.NewDiscoveredContainerRepo(db)
	discoveredRouteRepo := repo.NewDiscoveredRouteRepo(db)
	store := repo.NewStore(
		appRepo, eventRepo, checkRepo,
		rollupRepo, resourceRepo, resourceRollupRepo,
		infraComponentRepo, dockerEngineRepo,
		infraRepo, settingsRepo, metricsRepo, userRepo,
		traefikComponentRepo,
		discoveredContainerRepo, discoveredRouteRepo,
	)

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
		go watcher.Start(dockerCtx)
	}

	if poller, err := docker.NewResourcePoller(store); err != nil {
		log.Printf("resource poller: socket not available, skipping (%v)", err)
	} else {
		go poller.Start(dockerCtx)
	}

	// Router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Public routes — no session auth
	api.RegisterDocsRoutes(r)
	r.Post("/api/v1/ingest/{token}", api.HandleIngest(store, registry, limiter))

	// API v1 — protected by auth middleware
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(auth.RequireAuth(cfg.DevMode))
		api.NewAppsHandler(appRepo).Routes(r)
		api.NewEventsHandler(eventRepo).Routes(r)
		api.NewChecksHandler(checkRepo, eventRepo).Routes(r)
		api.NewDashboardHandler(appRepo, eventRepo, checkRepo, rollupRepo, registry).Routes(r)
		api.NewTopologyHandler(infraComponentRepo, dockerEngineRepo, appRepo, resourceRollupRepo).Routes(r)
		api.NewInfraComponentHandler(infraComponentRepo, resourceRollupRepo, checkRepo, traefikComponentRepo).Routes(r)
		api.NewProfilesHandler(registry, customProfileRepo).Routes(r)
		api.NewInfraHandler(infraRepo, syncWorker).Routes(r)
		api.NewDigestHandler(store, digestJob).Routes(r)
		api.NewSettingsHandler(store).Routes(r)
		api.NewIntegrationDriversHandler(settingsRepo).Routes(r)
		api.NewMetricsHandler(eventRepo, appRepo, metricsRepo, cfg.DBPath, startTime).Routes(r)
		api.NewUsersHandler(userRepo).Routes(r)
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
