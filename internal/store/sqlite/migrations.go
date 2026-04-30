package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
)

// MigrationsFS holds the SQL migration files for the sqlite store. It is
// exported so cmd/migrate can construct goose providers against the exact
// same migration set that the server applies on boot.
//
// Add new migrations by creating a file named
// migrations/<UTC-timestamp>_<descriptive_name>.sql with `-- +goose Up`
// and `-- +goose Down` sections. The easiest way is `go run ./cmd/migrate
// create <name>`, which generates the timestamp and skeleton.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// newProvider builds a goose Provider against the embedded MigrationsFS.
// It is shared by the in-process boot path (Store.Open) and the cmd/migrate
// CLI so both apply migrations identically.
func newProvider(db *sql.DB) (*goose.Provider, error) {
	migrationsSubFS, err := fs.Sub(MigrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("extract migrations subdir: %w", err)
	}
	p, err := goose.NewProvider(goose.DialectSQLite3, db, migrationsSubFS,
		goose.WithVerbose(false))
	if err != nil {
		return nil, fmt.Errorf("init goose provider: %w", err)
	}
	return p, nil
}

// migrate brings the database schema up to the latest version. Safe to call
// on a fresh database (creates the baseline) or an existing one (applies
// any new migrations). Goose maintains a goose_db_version table to track
// applied versions.
func migrate(ctx context.Context, db *sql.DB) error {
	p, err := newProvider(db)
	if err != nil {
		return err
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
