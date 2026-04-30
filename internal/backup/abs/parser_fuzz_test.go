package abs

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// FuzzParseNarratorsJSON hardens the pure JSON-array parser against malformed input.
func FuzzParseNarratorsJSON(f *testing.F) {
	// Seeds derived from formats seen in real ABS exports.
	f.Add(`["Alice","Bob"]`)
	f.Add(`[]`)
	f.Add(``)
	f.Add(`null`)
	f.Add(`["solo"]`)
	f.Add(`["Smith, John","Doe, Jane"]`)
	f.Add(`[" leading","trailing "]`)
	f.Add(`["with \"quote\"","backslash \\"]`)
	f.Add(`["name"]extra`)
	f.Add(`[[[`)
	f.Add(`"not an array"`)
	f.Add(`{"key":"val"}`)
	f.Add(`[1,2,3]`)
	f.Add("\x00\x01\x02")

	f.Fuzz(func(t *testing.T, jsonStr string) {
		// The function must never panic on any input.
		_ = parseNarratorsJSON(jsonStr)
	})
}

// FuzzParseSessions hardens the session parser end-to-end via an in-memory sqlite DB.
func FuzzParseSessions(f *testing.F) {
	// Seeds: representative valid + edge case row values.
	f.Add("sess1", "user1", "lib1", "media1", "Some Title", "Some Author", 3600.0, 1800.0, 0.0, 1800.0, "2025-01-01")
	f.Add("", "", "", "", "", "", 0.0, 0.0, 0.0, 0.0, "")
	f.Add("sess2", "user", "lib", "media", "🦄 emoji", "Smith, John", 1.5e308, -1.0, 1e9, 1e9, "x")
	f.Add("sess3", "u", "l", "m", "ctrl-bytes\x00\x01\x02", "a", 1.0, 1.0, 0.0, 0.5, "?")
	f.Add("s", "u", "l", "m", "title", "author", 0.0, -1e308, -1e9, -1e9, "2099-12-31")
	f.Add("s", "u", "l", "m", "t", "a", 0.0, 0.0, 0.0, 0.0, "not-a-date")

	f.Fuzz(func(t *testing.T,
		id, userID, libraryID, mediaID, displayTitle, displayAuthor string,
		duration, timeListening, startTime, currentTime float64,
		date string,
	) {
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "fuzz.db")
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Skip("open sqlite:", err)
		}
		defer db.Close()

		ctx := context.Background()

		// Create the minimum schema parseSessions touches.
		schema := `
			CREATE TABLE libraryItems (
				id TEXT PRIMARY KEY,
				mediaId TEXT,
				mediaType TEXT
			);
			CREATE TABLE playbackSessions (
				id TEXT PRIMARY KEY,
				userId TEXT,
				libraryId TEXT,
				mediaItemId TEXT,
				mediaItemType TEXT,
				displayTitle TEXT,
				displayAuthor TEXT,
				duration REAL,
				timeListening REAL,
				startTime REAL,
				currentTime REAL,
				date TEXT,
				dayOfWeek TEXT,
				createdAt TEXT,
				updatedAt TEXT
			);
		`
		if _, err := db.ExecContext(ctx, schema); err != nil {
			t.Skip("create schema:", err)
		}

		// Insert one fuzzed row.
		_, err = db.ExecContext(ctx, `INSERT INTO playbackSessions
			(id, userId, libraryId, mediaItemId, mediaItemType, displayTitle, displayAuthor,
			 duration, timeListening, startTime, currentTime, date, dayOfWeek, createdAt, updatedAt)
			VALUES (?, ?, ?, ?, 'book', ?, ?, ?, ?, ?, ?, ?, '0', '2025-01-01T00:00:00Z', '2025-01-01T00:00:00Z')`,
			id, userID, libraryID, mediaID, displayTitle, displayAuthor,
			duration, timeListening, startTime, currentTime, date)
		if err != nil {
			// SQLite-level rejection is fine; we're testing the parser, not sqlite.
			t.Skip("insert:", err)
		}

		backup := &Backup{}
		// MUST NOT PANIC. An error return is fine; we're hardening against
		// crashes, not requiring acceptance.
		_ = parseSessions(ctx, db, backup)
	})
}

// FuzzParseMediaProgress hardens the media-progress parser end-to-end via in-memory sqlite.
func FuzzParseMediaProgress(f *testing.F) {
	// Seeds: representative valid + edge case row values.
	f.Add("prog1", "user1", "media1", 3600.0, 1800.0, 0, 0, "2025-01-15T10:00:00Z", "2025-01-01T08:00:00Z", "")
	f.Add("", "", "", 0.0, 0.0, 0, 0, "", "", "")
	f.Add("p2", "u", "m", 1.5e308, -1.0, 1, 1, "not-a-timestamp", "x", "2025-06-01T00:00:00Z")
	f.Add("p3", "u", "m", 0.0, -1e308, 0, 0, "", "", "")
	f.Add("p4", "u", "m\x00\x01", 1.0, 0.5, 1, 0, "2025-01-01T00:00:00Z", "2024-12-01T00:00:00Z", "2025-01-02T00:00:00Z")

	f.Fuzz(func(t *testing.T,
		id, userID, mediaItemID string,
		duration, currentTime float64,
		isFinished, hideFromContinue int,
		updatedAt, createdAt, finishedAt string,
	) {
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "fuzz_progress.db")
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Skip("open sqlite:", err)
		}
		defer db.Close()

		ctx := context.Background()

		// Modern schema (no libraryItemId column) — simpler path exercised by default.
		schema := `
			CREATE TABLE libraryItems (
				id TEXT PRIMARY KEY,
				mediaId TEXT
			);
			CREATE TABLE mediaProgresses (
				id TEXT PRIMARY KEY,
				userId TEXT,
				mediaItemId TEXT,
				mediaItemType TEXT DEFAULT 'book',
				duration REAL DEFAULT 0,
				currentTime REAL DEFAULT 0,
				isFinished INTEGER DEFAULT 0,
				hideFromContinueListening INTEGER DEFAULT 0,
				updatedAt TEXT,
				createdAt TEXT,
				finishedAt TEXT
			);
		`
		if _, err := db.ExecContext(ctx, schema); err != nil {
			t.Skip("create schema:", err)
		}

		_, err = db.ExecContext(ctx, `INSERT INTO mediaProgresses
			(id, userId, mediaItemId, mediaItemType, duration, currentTime,
			 isFinished, hideFromContinueListening, updatedAt, createdAt, finishedAt)
			VALUES (?, ?, ?, 'book', ?, ?, ?, ?, ?, ?, ?)`,
			id, userID, mediaItemID,
			duration, currentTime,
			isFinished, hideFromContinue,
			updatedAt, createdAt, finishedAt)
		if err != nil {
			// SQLite-level rejection is fine; we're testing the parser, not sqlite.
			t.Skip("insert:", err)
		}

		// Attach a matching user so progress can be linked (tests the progressMap attachment too).
		backup := &Backup{
			Users: []User{{ID: userID, Username: "fuzzuser"}},
		}
		// MUST NOT PANIC. An error return is fine.
		_ = parseMediaProgress(ctx, db, backup)
	})
}
