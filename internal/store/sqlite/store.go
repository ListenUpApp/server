package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/dto"
	"github.com/listenupapp/listenup-server/internal/store"

	// modernc.org/sqlite is registered as the "sqlite" SQL driver via blank import.
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// Store provides SQLite-backed persistence for the ListenUp server.
type Store struct {
	db     *sql.DB
	logger *slog.Logger

	enricher      *dto.Enricher
	emitter       store.EventEmitter
	searchIndexer store.SearchIndexer
	indexer       *asyncIndexer

	mu       sync.RWMutex
	bulkMode bool
}

// Open creates a new SQLite store at the given path.
// It configures WAL mode, sets pragmas, and runs schema migrations.
func Open(path string, logger *slog.Logger) (*Store, error) {
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(normal)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(time.Hour)

	// Run schema migration. No parent context here — this is startup code.
	if _, err := db.Exec(schemaSQL); err != nil { //nolint:noctx // startup-time migration; no caller context
		_ = db.Close()
		return nil, fmt.Errorf("exec schema: %w", err)
	}

	s := &Store{
		db:            db,
		logger:        logger,
		emitter:       store.NewNoopEmitter(),
		searchIndexer: store.NewNoopSearchIndexer(),
	}

	// Wrap the (initially no-op) indexer in the async queue so that store
	// methods always have a non-nil submission target. SetSearchIndexer
	// will swap in the real indexer once search is wired up.
	s.indexer = newAsyncIndexer(s.searchIndexer, logger)
	s.indexer.Start(context.Background())

	// Initialize enricher for SSE event denormalization.
	// The store implements dto.Store interface.
	s.enricher = dto.NewEnricher(s)

	return s, nil
}

// Close closes the underlying database connection. It first drains the
// async search indexer with a bounded grace period so in-flight index
// updates have a chance to finish before the database goes away.
func (s *Store) Close() error {
	if s.indexer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := s.indexer.Shutdown(ctx); err != nil {
			s.logger.Warn("async indexer shutdown timed out", "error", err)
		}
		cancel()
	}
	return s.db.Close()
}

// SetSearchIndexer sets the search indexer used for maintaining the search
// index. It rebuilds the async queue around the new indexer so that all
// store operations submit work to a single, lifecycle-managed worker pool.
func (s *Store) SetSearchIndexer(indexer store.SearchIndexer) {
	// Drain the previous (typically no-op) async indexer before swapping.
	if s.indexer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := s.indexer.Shutdown(ctx); err != nil {
			s.logger.Warn("previous async indexer shutdown timed out", "error", err)
		}
		cancel()
	}
	s.searchIndexer = indexer
	s.indexer = newAsyncIndexer(indexer, s.logger)
	s.indexer.Start(context.Background())
}

// SetBulkMode enables or disables bulk mode, which suppresses indexing and
// events.
//
// Concurrency contract: bulk mode is a single global flag, not a counter, so
// concurrent scans would trample each other's state — e.g. scan A finishes
// and clears the flag while scan B is still running, re-enabling indexing
// mid-bulk-write. Callers must therefore not invoke SetBulkMode concurrently.
//
// This is acceptable because the only caller is scanner.Scan, and the API
// layer serializes full scans via sse.Manager.IsScanning() (see
// internal/api/library_handlers.go); the watcher path uses ScanFolder, which
// does not touch bulk mode.
//
// To enforce the contract at runtime, SetBulkMode panics if the new value
// matches the current value: SetBulkMode(true) when already true means a
// second scan started before the first cleared the flag, which would
// indicate the upstream serialization guard is broken. If nested or
// genuinely concurrent bulk regions ever become a real requirement, replace
// the bool with a reference-counted bulkDepth and drop the panic.
func (s *Store) SetBulkMode(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bulkMode == enabled {
		panic(fmt.Sprintf("sqlite store: SetBulkMode(%v) called while bulkMode already %v; concurrent scans are not supported", enabled, s.bulkMode))
	}
	s.bulkMode = enabled
}

// IsBulkMode returns whether the store is in bulk mode.
func (s *Store) IsBulkMode() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bulkMode
}

// InvalidateGenreCache is a no-op for the SQLite store.
// The SQLite store does not maintain an in-memory genre cache.
func (s *Store) InvalidateGenreCache() {}

// BroadcastUserPending is a no-op; SSE events will be handled separately.
func (s *Store) BroadcastUserPending(_ *domain.User) {}

// BroadcastUserApproved is a no-op; SSE events will be handled separately.
func (s *Store) BroadcastUserApproved(_ *domain.User) {}

// BroadcastUserDeleted is a no-op; SSE events will be handled separately.
func (s *Store) BroadcastUserDeleted(_, _ string) {}

// formatTime formats a time.Time to RFC3339Nano for storage.
func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// parseTime parses a RFC3339Nano string back to time.Time.
func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		// Attempt to salvage malformed timestamps (e.g. double "ZZ" suffix
		// from earlier ABS import bugs) before giving up.
		cleaned := strings.TrimRight(s, "Z") + "Z"
		t, err2 := time.Parse(time.RFC3339Nano, cleaned)
		if err2 == nil {
			return t, nil
		}
	}
	return t, err
}

// parseNullableTime parses an optional time string.
func parseNullableTime(s sql.NullString) (*time.Time, error) {
	if !s.Valid || s.String == "" {
		return nil, nil
	}
	t, err := parseTime(s.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// nullString returns a sql.NullString from a string pointer or empty string.
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// nullableString returns a sql.NullString from a *string.
func nullableString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

// nullTimeString returns a sql.NullString from a *time.Time.
func nullTimeString(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: formatTime(*t), Valid: true}
}

// nullInt64 returns a sql.NullInt64 from an int64.
func nullInt64(v int64) sql.NullInt64 {
	if v == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: v, Valid: true}
}
