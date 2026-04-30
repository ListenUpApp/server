// Package main is a migration CLI for the listenUp server database.
//
// Usage:
//
//	go run ./cmd/migrate <subcommand> [flags]
//
// Subcommands:
//
//	up      [-db <path>]        Apply all pending migrations
//	down    [-db <path>]        Roll back the most recent migration
//	status  [-db <path>]        Print migration status table
//	redo    [-db <path>]        Roll back then re-apply the latest migration
//	version [-db <path>]        Print the current schema version
//	create  <name>              Generate a new timestamped .sql migration file
//
// The -db flag defaults to $DB_PATH env var, falling back to $HOME/listenUp/db.
// The create subcommand must be run from the repository root.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/listenupapp/listenup-server/internal/store/sqlite"
	"github.com/pressly/goose/v3"

	_ "modernc.org/sqlite"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func defaultDBPath() string {
	if v := os.Getenv("DB_PATH"); v != "" {
		return v
	}
	return os.ExpandEnv("$HOME/listenUp/db")
}

// withDB opens the SQLite database, calls fn, and always closes the DB.
// Any close error is joined with the error returned by fn.
func withDB(dbPath string, fn func(*sql.DB) error) error {
	dsn := dbPath + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	runErr := fn(db)
	closeErr := db.Close()
	return errors.Join(runErr, closeErr)
}

// newProvider builds a goose provider using the embedded MigrationsFS,
// with verbose output enabled for CLI use.
func newProvider(db *sql.DB) (*goose.Provider, error) {
	migrationsSubFS, err := fs.Sub(sqlite.MigrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("extract migrations subdir: %w", err)
	}
	p, err := goose.NewProvider(goose.DialectSQLite3, db, migrationsSubFS,
		goose.WithVerbose(true))
	if err != nil {
		return nil, fmt.Errorf("init goose provider: %w", err)
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Subcommands
// ---------------------------------------------------------------------------

func runUp(args []string) error {
	fset := flag.NewFlagSet("up", flag.ContinueOnError)
	dbPath := fset.String("db", defaultDBPath(), "path to the SQLite database file")
	if err := fset.Parse(args); err != nil {
		return err
	}
	return withDB(*dbPath, func(db *sql.DB) error {
		p, err := newProvider(db)
		if err != nil {
			return err
		}
		results, err := p.Up(context.Background())
		if err != nil {
			return fmt.Errorf("up: %w", err)
		}
		if len(results) == 0 {
			fmt.Println("no pending migrations")
			return nil
		}
		for _, r := range results {
			fmt.Printf("applied: %s (v%d)\n", filepath.Base(r.Source.Path), r.Source.Version)
		}
		return nil
	})
}

func runDown(args []string) error {
	fset := flag.NewFlagSet("down", flag.ContinueOnError)
	dbPath := fset.String("db", defaultDBPath(), "path to the SQLite database file")
	if err := fset.Parse(args); err != nil {
		return err
	}
	return withDB(*dbPath, func(db *sql.DB) error {
		p, err := newProvider(db)
		if err != nil {
			return err
		}
		result, err := p.Down(context.Background())
		if err != nil {
			return fmt.Errorf("down: %w", err)
		}
		if result == nil {
			fmt.Println("no applied migrations to roll back")
			return nil
		}
		fmt.Printf("rolled back: %s (v%d)\n", filepath.Base(result.Source.Path), result.Source.Version)
		return nil
	})
}

func runStatus(args []string) error {
	fset := flag.NewFlagSet("status", flag.ContinueOnError)
	dbPath := fset.String("db", defaultDBPath(), "path to the SQLite database file")
	if err := fset.Parse(args); err != nil {
		return err
	}
	return withDB(*dbPath, func(db *sql.DB) error {
		p, err := newProvider(db)
		if err != nil {
			return err
		}
		statuses, err := p.Status(context.Background())
		if err != nil {
			return fmt.Errorf("status: %w", err)
		}
		fmt.Printf("%-12s  %-20s  %s\n", "state", "version", "filename")
		fmt.Println(strings.Repeat("-", 60))
		for _, s := range statuses {
			fmt.Printf("%-12s  %-20d  %s\n", s.State, s.Source.Version, filepath.Base(s.Source.Path))
		}
		return nil
	})
}

func runRedo(args []string) error {
	fset := flag.NewFlagSet("redo", flag.ContinueOnError)
	dbPath := fset.String("db", defaultDBPath(), "path to the SQLite database file")
	if err := fset.Parse(args); err != nil {
		return err
	}
	return withDB(*dbPath, func(db *sql.DB) error {
		p, err := newProvider(db)
		if err != nil {
			return err
		}
		ctx := context.Background()
		downResult, err := p.Down(ctx)
		if err != nil {
			return fmt.Errorf("redo down: %w", err)
		}
		if downResult != nil {
			fmt.Printf("rolled back: %s (v%d)\n", filepath.Base(downResult.Source.Path), downResult.Source.Version)
		}
		upResult, err := p.UpByOne(ctx)
		if err != nil {
			return fmt.Errorf("redo up: %w", err)
		}
		if upResult != nil {
			fmt.Printf("applied: %s (v%d)\n", filepath.Base(upResult.Source.Path), upResult.Source.Version)
		}
		return nil
	})
}

func runVersion(args []string) error {
	fset := flag.NewFlagSet("version", flag.ContinueOnError)
	dbPath := fset.String("db", defaultDBPath(), "path to the SQLite database file")
	if err := fset.Parse(args); err != nil {
		return err
	}
	return withDB(*dbPath, func(db *sql.DB) error {
		p, err := newProvider(db)
		if err != nil {
			return err
		}
		ver, err := p.GetDBVersion(context.Background())
		if err != nil {
			return fmt.Errorf("version: %w", err)
		}
		fmt.Printf("current version: %d\n", ver)
		return nil
	})
}

var (
	reInvalidChars    = regexp.MustCompile(`[^a-z0-9_]`)
	reMultiUnderscore = regexp.MustCompile(`_+`)
)

// normalizeName converts a human-readable name into a safe snake_case identifier
// suitable for use in a migration filename.
func normalizeName(name string) string {
	s := strings.ToLower(name)
	s = strings.NewReplacer(" ", "_", "-", "_").Replace(s)
	s = reInvalidChars.ReplaceAllString(s, "")
	s = reMultiUnderscore.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	return s
}

const migrationSkeleton = `-- +goose Up
-- TODO: write the schema change here.

-- +goose Down
-- TODO: write the inverse change here, or 'SELECT 1;' if irreversible.
`

func runCreate(args []string) error {
	if len(args) == 0 {
		return errors.New("create requires a migration name argument")
	}
	name := strings.Join(args, " ")
	normalized := normalizeName(name)
	if normalized == "" {
		return fmt.Errorf("name %q normalizes to an empty string; choose a different name", name)
	}

	timestamp := time.Now().UTC().Format("20060102150405")
	filename := timestamp + "_" + normalized + ".sql"

	migrationsDir := filepath.Join("internal", "store", "sqlite", "migrations")
	info, err := os.Stat(migrationsDir)
	if err != nil || !info.IsDir() {
		return errors.New("internal/store/sqlite/migrations/ not found; run from the repo root")
	}

	destPath := filepath.Join(migrationsDir, filename)
	if _, err := os.Stat(destPath); err == nil {
		return fmt.Errorf("file already exists: %s", destPath)
	}

	if err := os.WriteFile(destPath, []byte(migrationSkeleton), 0o644); err != nil {
		return fmt.Errorf("write migration file: %w", err)
	}

	absPath, err := filepath.Abs(destPath)
	if err != nil {
		absPath = destPath
	}
	fmt.Println(absPath)
	return nil
}

// ---------------------------------------------------------------------------
// Entry point
// ---------------------------------------------------------------------------

func usage() {
	fmt.Fprintln(os.Stderr, `migrate — listenUp database migration CLI

Usage:

  go run ./cmd/migrate <subcommand> [flags]

Subcommands:

  up      [-db <path>]   Apply all pending migrations
  down    [-db <path>]   Roll back the most recent migration
  status  [-db <path>]   Print migration status table
  redo    [-db <path>]   Roll back then re-apply the latest migration
  version [-db <path>]   Print the current schema version
  create  <name>         Generate a new timestamped .sql migration file

The -db flag defaults to $DB_PATH env var, falling back to $HOME/listenUp/db.
The create subcommand must be run from the repository root.`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	cmd := os.Args[1]
	rest := os.Args[2:]

	var err error
	switch cmd {
	case "up":
		err = runUp(rest)
	case "down":
		err = runDown(rest)
	case "status":
		err = runStatus(rest)
	case "redo":
		err = runRedo(rest)
	case "version":
		err = runVersion(rest)
	case "create":
		err = runCreate(rest)
	default:
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate %s: %v\n", cmd, err)
		os.Exit(1)
	}
}
