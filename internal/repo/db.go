package repo

import (
	"fmt"
	"io/fs"

	"github.com/digitalcheffe/nora/internal/config"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

// Open opens the SQLite database at Config.DBPath, enforces WAL journal mode
// and foreign key constraints, then runs all pending migrations from the
// provided FS. It returns the ready-to-use *sqlx.DB or an error.
func Open(cfg *config.Config, migrations fs.FS) (*sqlx.DB, error) {
	db, err := sqlx.Open("sqlite3", cfg.DBPath+"?_journal_mode=WAL&_foreign_keys=ON")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// Also set via PRAGMA in case DSN query params are not honoured by the driver build.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("set journal_mode=WAL: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return nil, fmt.Errorf("set foreign_keys=ON: %w", err)
	}

	if err := runMigrations(db, migrations); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return db, nil
}
