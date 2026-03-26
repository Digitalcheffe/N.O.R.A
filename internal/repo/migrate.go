package repo

import (
	"fmt"
	"io/fs"
	"log"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
)

// runMigrations creates the schema_migrations tracking table if needed, then
// applies every *.sql file from the provided FS in filename order, skipping any
// that have already been recorded. It panics on a failed migration — a broken
// schema at startup is unrecoverable.
func runMigrations(db *sqlx.DB, files fs.FS) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name       TEXT PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return fmt.Errorf("read migrations fs: %w", err)
	}

	// Sort by filename to guarantee 001, 002, … ordering.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		name := entry.Name()

		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE name = ?", name).Scan(&count); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if count > 0 {
			log.Printf("migration: skip %s (already applied)", name)
			continue
		}

		data, err := fs.ReadFile(files, name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		if _, err := db.Exec(string(data)); err != nil {
			panic(fmt.Sprintf("migration %s failed: %v", name, err))
		}

		if _, err := db.Exec("INSERT INTO schema_migrations (name) VALUES (?)", name); err != nil {
			return fmt.Errorf("record migration %s: %w", name, err)
		}

		log.Printf("migration: applied %s", name)
	}

	return nil
}
