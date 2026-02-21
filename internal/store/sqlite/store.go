package sqlite

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/dto"
	"github.com/listenupapp/listenup-server/internal/store"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// Store provides SQLite-backed persistence for the ListenUp server.
type Store struct {
	db     *sql.DB
	logger *slog.Logger

	enricher         *dto.Enricher
	emitter          store.EventEmitter
	searchIndexer    store.SearchIndexer
	transcodeDeleter store.TranscodeDeleter

	mu       sync.RWMutex
	bulkMode bool
}

// Open creates a new SQLite store at the given path.
// It configures WAL mode, sets pragmas, and runs schema migrations.
func Open(path string, logger *slog.Logger) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Set connection pool to 1 writer (SQLite limitation).
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(time.Hour)

	// Configure pragmas.
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("exec pragma %q: %w", pragma, err)
		}
	}

	// Run schema migration.
	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("exec schema: %w", err)
	}

	s := &Store{
		db:               db,
		logger:           logger,
		emitter:          store.NewNoopEmitter(),
		searchIndexer:    store.NewNoopSearchIndexer(),
		transcodeDeleter: store.NewNoopTranscodeDeleter(),
	}

	// Initialize enricher for SSE event denormalization.
	// The store implements dto.Store interface.
	s.enricher = dto.NewEnricher(s)

	return s, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// SetSearchIndexer sets the search indexer used for maintaining the search index.
func (s *Store) SetSearchIndexer(indexer store.SearchIndexer) {
	s.searchIndexer = indexer
}

// SetTranscodeDeleter sets the transcode deleter used for cleaning up transcoded files.
func (s *Store) SetTranscodeDeleter(deleter store.TranscodeDeleter) {
	s.transcodeDeleter = deleter
}

// SetBulkMode enables or disables bulk mode, which suppresses indexing and events.
func (s *Store) SetBulkMode(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	return time.Parse(time.RFC3339Nano, s)
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
