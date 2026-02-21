package sqlite

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	s, err := Open(dbPath, logger)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpen(t *testing.T) {
	s := newTestStore(t)

	// Verify WAL mode is set.
	var journalMode string
	err := s.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("expected wal, got %s", journalMode)
	}

	// Verify foreign keys are enabled.
	var fk int
	err = s.db.QueryRow("PRAGMA foreign_keys").Scan(&fk)
	if err != nil {
		t.Fatalf("query foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("expected foreign_keys=1, got %d", fk)
	}

	// Verify tables exist.
	tables := []string{
		"users", "sessions", "libraries", "books", "book_audio_files", "book_chapters",
		"contributors", "book_contributors", "series", "book_series",
		"genres", "book_genres", "tags", "book_tags",
		"collections", "collection_books", "collection_shares",
		"shelves", "shelf_books",
		"listening_events", "playback_state", "book_preferences",
		"book_reading_sessions", "invites", "instance", "server_settings",
	}
	for _, table := range tables {
		var name string
		err := s.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("table %s not found: %v", table, err)
		}
	}
}

func TestOpenClose(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	s, err := Open(dbPath, logger)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	// Re-open should work (schema is idempotent).
	s2, err := Open(dbPath, logger)
	if err != nil {
		t.Fatalf("re-open store: %v", err)
	}
	defer s2.Close()
}
