// Package main is the NORA server entry point.
//
// @title           NORA API
// @version         1.0
// @description     Nexus Operations Recon & Alerts — self-hosted homelab monitoring, event capture, and alerting.
//
// @contact.name    NORA Project
// @contact.url     https://github.com/digitalcheffe/nora
//
// @license.name    MIT
//
// @host            localhost:8080
// @BasePath        /api/v1
//
// @securityDefinitions.apikey BearerToken
// @in              header
// @name            Authorization
// @description     Session token. Use dev-mode bypass when NORA_DEV_MODE=true.
package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/digitalcheffe/nora/internal/api"
	"github.com/digitalcheffe/nora/internal/auth"
	"github.com/digitalcheffe/nora/internal/config"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/migrations"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	// Register generated OpenAPI spec with the swag runtime.
	_ "github.com/digitalcheffe/nora/docs/swagger"
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

	// Router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Public — API docs (Scalar UI + OpenAPI spec)
	r.Get("/docs/swagger.json", api.SwaggerJSON)
	r.Get("/docs", api.ScalarUI)
	r.Get("/docs/", api.ScalarUI)

	// API v1 — protected by auth middleware
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(auth.RequireAuth(cfg.DevMode))
		api.NewAppsHandler(appRepo).Routes(r)
	})

	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("NORA listening on %s (dev_mode=%v)", addr, cfg.DevMode)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
