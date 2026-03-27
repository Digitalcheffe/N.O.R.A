package main

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"

	"github.com/digitalcheffe/nora/internal/api"
	"github.com/digitalcheffe/nora/internal/auth"
	"github.com/digitalcheffe/nora/internal/config"
	"github.com/digitalcheffe/nora/internal/frontend"
	"github.com/digitalcheffe/nora/internal/ingest"
	"github.com/digitalcheffe/nora/internal/profile"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/migrations"
	noraprofiles "github.com/digitalcheffe/nora/profiles"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	cfg := config.Load()

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
	store := repo.NewStore(appRepo, eventRepo, checkRepo, rollupRepo)

	// Profile registry — load all bundled YAML profiles
	registry, err := profile.NewRegistry(noraprofiles.Files)
	if err != nil {
		log.Fatalf("profile registry init failed: %v", err)
	}
	log.Printf("loaded %d profiles", len(registry.List()))

	limiter := ingest.NewRateLimiter()

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
