package main

import (
	"log"

	"github.com/digitalcheffe/nora/internal/config"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/migrations"
)

func main() {
	cfg := config.Load()

	db, err := repo.Open(cfg, migrations.Files)
	if err != nil {
		log.Fatalf("database init failed: %v", err)
	}
	defer db.Close()

	store := repo.NewStore(db)
	_ = store // TODO: pass to handlers (T-05+)

	log.Printf("NORA database ready at %s", cfg.DBPath)

	// TODO: initialize router and background jobs (T-02 / T-05+)
}
