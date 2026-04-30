package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMigrate_AppliesBaseline(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	if err := migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// goose_db_version should exist after a successful Up.
	var version int64
	if err := db.QueryRowContext(ctx,
		`SELECT MAX(version_id) FROM goose_db_version WHERE is_applied = 1`,
	).Scan(&version); err != nil {
		t.Fatalf("query goose_db_version: %v", err)
	}
	if version == 0 {
		t.Fatalf("expected at least one applied migration, got version 0")
	}

	// One canonical baseline table must exist.
	var name string
	if err := db.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'users'`,
	).Scan(&name); err != nil {
		t.Fatalf("baseline did not create users table: %v", err)
	}
	if name != "users" {
		t.Fatalf("expected users table, got %q", name)
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	if err := migrate(ctx, db); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := migrate(ctx, db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM goose_db_version WHERE is_applied = 1`,
	).Scan(&count); err != nil {
		t.Fatalf("count goose_db_version: %v", err)
	}
	if count < 1 {
		t.Fatalf("expected at least one applied migration, got %d", count)
	}
}
