package repo

import (
	"testing"
	"testing/fstest"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func openTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestRunMigrations_HappyPath(t *testing.T) {
	db := openTestDB(t)

	migrations := fstest.MapFS{
		"001_test.sql": &fstest.MapFile{
			Data: []byte(`CREATE TABLE test_table (id TEXT PRIMARY KEY, name TEXT NOT NULL);`),
		},
	}

	if err := runMigrations(db, migrations); err != nil {
		t.Fatalf("runMigrations failed: %v", err)
	}

	// Verify schema_migrations row was recorded.
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE name = '001_test.sql'").Scan(&count); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 schema_migrations row, got %d", count)
	}

	// Verify the table was actually created.
	if _, err := db.Exec("INSERT INTO test_table (id, name) VALUES ('1', 'hello')"); err != nil {
		t.Errorf("test_table not created: %v", err)
	}
}

func TestRunMigrations_Idempotent(t *testing.T) {
	db := openTestDB(t)

	migrations := fstest.MapFS{
		"001_test.sql": &fstest.MapFile{
			Data: []byte(`CREATE TABLE idempotent_table (id TEXT PRIMARY KEY);`),
		},
	}

	// First run — should apply the migration.
	if err := runMigrations(db, migrations); err != nil {
		t.Fatalf("first runMigrations failed: %v", err)
	}

	// Second run — should skip (idempotent), NOT return an error.
	if err := runMigrations(db, migrations); err != nil {
		t.Fatalf("second runMigrations failed: %v", err)
	}

	// Should still be exactly one row in schema_migrations.
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE name = '001_test.sql'").Scan(&count); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 schema_migrations row after idempotent run, got %d", count)
	}
}

func TestRunMigrations_OrderedApplication(t *testing.T) {
	db := openTestDB(t)

	// 002 depends on 001 — if order is wrong the FK will fail and the test panics.
	migrations := fstest.MapFS{
		"001_create_parent.sql": &fstest.MapFile{
			Data: []byte(`CREATE TABLE parent (id TEXT PRIMARY KEY);`),
		},
		"002_create_child.sql": &fstest.MapFile{
			Data: []byte(`CREATE TABLE child (id TEXT PRIMARY KEY, parent_id TEXT REFERENCES parent(id));`),
		},
	}

	if err := runMigrations(db, migrations); err != nil {
		t.Fatalf("runMigrations failed: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 applied migrations, got %d", count)
	}
}
