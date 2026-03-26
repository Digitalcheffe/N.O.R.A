package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/digitalcheffe/nora/internal/api"
	"github.com/digitalcheffe/nora/internal/auth"
	"github.com/digitalcheffe/nora/internal/config"
	"github.com/digitalcheffe/nora/internal/ingest"
	"github.com/digitalcheffe/nora/internal/profile"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/migrations"
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
	store := repo.NewStore(appRepo, eventRepo)

	// Ingest dependencies
	profiler := &profile.NoopLoader{}
	limiter := ingest.NewRateLimiter()

	// Router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Public routes — no session auth
	r.Post("/api/v1/ingest/{token}", api.HandleIngest(store, profiler, limiter))

	// API v1 — protected by auth middleware
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(auth.RequireAuth(cfg.DevMode))
		api.NewAppsHandler(appRepo).Routes(r)
		api.NewEventsHandler(eventRepo).Routes(r)
	})

	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("NORA listening on %s (dev_mode=%v)", addr, cfg.DevMode)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
