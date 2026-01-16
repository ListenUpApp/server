package abs

import (
	"archive/zip"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestDiagnoseRealBackup(t *testing.T) {
	// Diagnostic test to examine the actual ABS backup structure
	backupPath := "/home/simonh/listenUp/backups/uploads/abs-upload-1768498102773632640.audiobookshelf"

	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Skip("Test backup file not found - skipping diagnostic")
	}

	// Extract and open the SQLite database directly
	zr, err := zip.OpenReader(backupPath)
	if err != nil {
		t.Fatalf("Failed to open backup: %v", err)
	}
	defer zr.Close()

	// Find and extract SQLite DB
	var dbFile *zip.File
	for _, f := range zr.File {
		if f.Name == "absdatabase.sqlite" {
			dbFile = f
			break
		}
	}
	if dbFile == nil {
		t.Fatal("No absdatabase.sqlite found in backup")
	}

	// Extract to temp file
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "absdatabase.sqlite")

	rc, err := dbFile.Open()
	if err != nil {
		t.Fatalf("Failed to open db file: %v", err)
	}

	outFile, err := os.Create(dbPath)
	if err != nil {
		rc.Close()
		t.Fatalf("Failed to create temp file: %v", err)
	}

	_, err = io.Copy(outFile, rc)
	rc.Close()
	outFile.Close()
	if err != nil {
		t.Fatalf("Failed to extract db: %v", err)
	}

	// Open DB
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open db: %v", err)
	}
	defer db.Close()

	// Check schema
	t.Log("=== mediaProgresses schema ===")
	rows, err := db.Query(`PRAGMA table_info(mediaProgresses)`)
	if err != nil {
		t.Fatalf("Failed to get schema: %v", err)
	}
	hasLibraryItemId := false
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt any
		rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk)
		t.Logf("  Column: %s (%s)", name, colType)
		if name == "libraryItemId" {
			hasLibraryItemId = true
		}
	}
	rows.Close()
	t.Logf("hasLibraryItemId column: %v", hasLibraryItemId)

	// Sample libraryItems
	t.Log("\n=== Sample libraryItems (first 5) ===")
	rows, err = db.Query(`SELECT id, mediaId FROM libraryItems LIMIT 5`)
	if err != nil {
		t.Logf("Error querying libraryItems: %v", err)
	} else {
		for rows.Next() {
			var id, mediaId string
			rows.Scan(&id, &mediaId)
			t.Logf("  libraryItems.id=%s  mediaId=%s", id, mediaId)
		}
		rows.Close()
	}

	// Sample mediaProgresses
	t.Log("\n=== Sample mediaProgresses (first 5) ===")
	rows, err = db.Query(`SELECT id, userId, mediaItemId, isFinished FROM mediaProgresses LIMIT 5`)
	if err != nil {
		t.Logf("Error querying mediaProgresses: %v", err)
	} else {
		for rows.Next() {
			var id, userId, mediaItemId string
			var isFinished int
			rows.Scan(&id, &userId, &mediaItemId, &isFinished)
			t.Logf("  progress.mediaItemId=%s  isFinished=%d", mediaItemId, isFinished)
		}
		rows.Close()
	}

	// Check if mediaItemId matches libraryItems.id or libraryItems.mediaId
	t.Log("\n=== Cross-reference check ===")
	rows, err = db.Query(`
		SELECT
			mp.mediaItemId,
			li_by_id.mediaId as resolved_by_id,
			li_by_media.id as matched_by_mediaId
		FROM mediaProgresses mp
		LEFT JOIN libraryItems li_by_id ON mp.mediaItemId = li_by_id.id
		LEFT JOIN libraryItems li_by_media ON mp.mediaItemId = li_by_media.mediaId
		LIMIT 5
	`)
	if err != nil {
		t.Logf("Error in cross-reference: %v", err)
	} else {
		for rows.Next() {
			var mediaItemId string
			var resolvedById, matchedByMediaId sql.NullString
			rows.Scan(&mediaItemId, &resolvedById, &matchedByMediaId)
			t.Logf("  mediaItemId=%s", mediaItemId)
			t.Logf("    -> JOIN on li.id gives mediaId: %v", resolvedById.String)
			t.Logf("    -> JOIN on li.mediaId gives id: %v", matchedByMediaId.String)
		}
		rows.Close()
	}

	// Sample playbackSessions
	t.Log("\n=== Sample playbackSessions (first 5) ===")
	rows, err = db.Query(`SELECT id, mediaItemId FROM playbackSessions LIMIT 5`)
	if err != nil {
		t.Logf("Error querying playbackSessions: %v", err)
	} else {
		for rows.Next() {
			var id, mediaItemId string
			rows.Scan(&id, &mediaItemId)
			t.Logf("  session.mediaItemId=%s", mediaItemId)
		}
		rows.Close()
	}

	// Final check: do sessions and progress share the same mediaItemId?
	t.Log("\n=== Overlap check: sessions vs progress mediaItemId ===")
	rows, err = db.Query(`
		SELECT
			COUNT(DISTINCT ps.mediaItemId) as session_media_ids,
			COUNT(DISTINCT mp.mediaItemId) as progress_media_ids,
			COUNT(DISTINCT CASE WHEN ps.mediaItemId = mp.mediaItemId THEN ps.mediaItemId END) as matching_ids
		FROM playbackSessions ps, mediaProgresses mp
	`)
	if err != nil {
		t.Logf("Error in overlap check: %v", err)
	} else {
		for rows.Next() {
			var sessionIds, progressIds, matching int
			rows.Scan(&sessionIds, &progressIds, &matching)
			t.Logf("  Session mediaItemIds: %d", sessionIds)
			t.Logf("  Progress mediaItemIds: %d", progressIds)
			t.Logf("  Matching: %d", matching)
		}
		rows.Close()
	}

	// Critical check: do libraryItems.mediaId and progress.mediaItemId share IDs?
	// This determines if ABSImportBook and ABSImportProgress will have matching ABSMediaID
	t.Log("\n=== CRITICAL: libraryItems.mediaId vs progress.mediaItemId ===")
	rows, err = db.Query(`
		SELECT
			COUNT(DISTINCT li.mediaId) as book_media_ids,
			COUNT(DISTINCT mp.mediaItemId) as progress_media_ids,
			COUNT(DISTINCT CASE WHEN li.mediaId = mp.mediaItemId THEN li.mediaId END) as matching_ids
		FROM libraryItems li, mediaProgresses mp
	`)
	if err != nil {
		t.Logf("Error: %v", err)
	} else {
		for rows.Next() {
			var bookIds, progressIds, matching int
			rows.Scan(&bookIds, &progressIds, &matching)
			t.Logf("  libraryItems.mediaId count: %d", bookIds)
			t.Logf("  progress.mediaItemId count: %d", progressIds)
			t.Logf("  MATCHING (will be stored with same ABSMediaID): %d", matching)
		}
		rows.Close()
	}

	// Check which finished progress entries have matching libraryItems
	t.Log("\n=== Finished progress entries with libraryItem matches ===")
	rows, err = db.Query(`
		SELECT
			mp.mediaItemId,
			mp.isFinished,
			li.mediaId as lib_media_id
		FROM mediaProgresses mp
		LEFT JOIN libraryItems li ON mp.mediaItemId = li.mediaId
		WHERE mp.isFinished = 1
	`)
	if err != nil {
		t.Logf("Error: %v", err)
	} else {
		count := 0
		for rows.Next() {
			var mediaItemId string
			var isFinished int
			var libMediaId sql.NullString
			rows.Scan(&mediaItemId, &isFinished, &libMediaId)
			count++
			t.Logf("  Finished progress: mediaItemId=%s has_libraryItem=%v", mediaItemId, libMediaId.Valid)
		}
		t.Logf("  Total finished progress entries: %d", count)
		rows.Close()
	}
}

func TestParseRealBackup(t *testing.T) {
	// Use actual backup file for integration test
	backupPath := "/home/simonh/listenUp/backups/uploads/abs-upload-1768498102773632640.audiobookshelf"

	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Skip("Test backup file not found - skipping integration test")
	}

	start := time.Now()
	t.Logf("Starting parse of %s", backupPath)

	backup, err := Parse(backupPath)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	duration := time.Since(start)
	t.Logf("Parse completed in %v", duration)

	// Verify we got data
	t.Logf("Results:")
	t.Logf("  Users: %d", len(backup.Users))
	t.Logf("  Libraries: %d", len(backup.Libraries))
	t.Logf("  Items: %d", len(backup.Items))
	t.Logf("  Authors: %d", len(backup.Authors))
	t.Logf("  Series: %d", len(backup.Series))
	t.Logf("  Sessions: %d", len(backup.Sessions))

	// Basic sanity checks
	if len(backup.Users) == 0 {
		t.Error("Expected at least one user")
	}
	if len(backup.Items) == 0 {
		t.Error("Expected at least one library item")
	}

	// Performance check - should complete in under 10 seconds for reasonable backup
	if duration > 10*time.Second {
		t.Errorf("Parse took too long: %v (expected < 10s)", duration)
	}

	t.Logf("Summary: %s", backup.Summary())
}

func TestParseMediaProgressModernSchema(t *testing.T) {
	// Test modern ABS schema (2.17+) without libraryItemId column
	// This tests the case where mediaItemId contains libraryItems.id (row UUID)
	// instead of books.id (mediaId), which requires JOIN resolution
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	// Create libraryItems table (needed for JOIN resolution)
	_, err = db.Exec(`
		CREATE TABLE libraryItems (
			id TEXT PRIMARY KEY,
			mediaId TEXT
		)
	`)
	if err != nil {
		t.Fatalf("failed to create libraryItems table: %v", err)
	}

	// Create modern schema without libraryItemId column
	_, err = db.Exec(`
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
		)
	`)
	if err != nil {
		t.Fatalf("failed to create mediaProgresses table: %v", err)
	}

	// Insert libraryItems - these have their own UUID (id) and reference books.id (mediaId)
	_, err = db.Exec(`
		INSERT INTO libraryItems (id, mediaId)
		VALUES
			('li-row-uuid-1', 'book-abc'),
			('li-row-uuid-2', 'book-xyz')
	`)
	if err != nil {
		t.Fatalf("failed to insert libraryItems: %v", err)
	}

	// Insert progress data where mediaItemId contains libraryItems.id (NOT books.id!)
	// This is the problematic case from the user's ABS backup
	_, err = db.Exec(`
		INSERT INTO mediaProgresses (id, userId, mediaItemId, mediaItemType, duration, currentTime, isFinished, updatedAt, createdAt)
		VALUES
			('prog-1', 'user-1', 'li-row-uuid-1', 'book', 3600.5, 1800.25, 0, '2025-01-15 10:00:00', '2025-01-10 08:00:00'),
			('prog-2', 'user-1', 'li-row-uuid-2', 'book', 7200.0, 7200.0, 1, '2025-01-14 15:30:00', '2025-01-01 12:00:00')
	`)
	if err != nil {
		t.Fatalf("failed to insert data: %v", err)
	}

	backup := &Backup{
		Users: []User{{ID: "user-1", Username: "testuser"}},
	}

	err = parseMediaProgress(db, backup)
	if err != nil {
		t.Fatalf("parseMediaProgress failed: %v", err)
	}

	// Verify progress was parsed correctly
	if len(backup.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(backup.Users))
	}

	progress := backup.Users[0].Progress
	if len(progress) != 2 {
		t.Fatalf("expected 2 progress items, got %d", len(progress))
	}

	// Verify first progress item
	if progress[0].LibraryItemID != "book-abc" {
		t.Errorf("expected LibraryItemID 'book-abc', got '%s'", progress[0].LibraryItemID)
	}
	if progress[0].Duration != 3600.5 {
		t.Errorf("expected Duration 3600.5, got %f", progress[0].Duration)
	}
	if progress[0].CurrentTime != 1800.25 {
		t.Errorf("expected CurrentTime 1800.25, got %f", progress[0].CurrentTime)
	}
	if progress[0].IsFinished {
		t.Error("expected IsFinished=false")
	}

	// Verify second progress item (finished)
	if progress[1].LibraryItemID != "book-xyz" {
		t.Errorf("expected LibraryItemID 'book-xyz', got '%s'", progress[1].LibraryItemID)
	}
	if !progress[1].IsFinished {
		t.Error("expected IsFinished=true")
	}
}

func TestParseMediaProgressLegacySchema(t *testing.T) {
	// Test legacy ABS schema with libraryItemId column
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	// Create legacy schema with libraryItemId
	_, err = db.Exec(`
		CREATE TABLE libraryItems (
			id TEXT PRIMARY KEY,
			mediaId TEXT
		);
		CREATE TABLE mediaProgresses (
			id TEXT PRIMARY KEY,
			userId TEXT,
			libraryItemId TEXT,
			mediaItemId TEXT,
			mediaItemType TEXT DEFAULT 'book',
			duration REAL DEFAULT 0,
			currentTime REAL DEFAULT 0,
			isFinished INTEGER DEFAULT 0,
			hideFromContinueListening INTEGER DEFAULT 0,
			updatedAt TEXT,
			createdAt TEXT,
			finishedAt TEXT
		)
	`)
	if err != nil {
		t.Fatalf("failed to create tables: %v", err)
	}

	// Insert test data - with legacy fallback through libraryItems
	_, err = db.Exec(`
		INSERT INTO libraryItems (id, mediaId) VALUES ('li-1', 'media-from-join');
		INSERT INTO mediaProgresses (id, userId, libraryItemId, mediaItemId, mediaItemType, duration, currentTime, updatedAt, createdAt)
		VALUES
			('prog-1', 'user-1', 'li-1', NULL, 'book', 5400.0, 2700.0, '2025-01-15 10:00:00', '2025-01-10 08:00:00')
	`)
	if err != nil {
		t.Fatalf("failed to insert data: %v", err)
	}

	backup := &Backup{
		Users: []User{{ID: "user-1", Username: "testuser"}},
	}

	err = parseMediaProgress(db, backup)
	if err != nil {
		t.Fatalf("parseMediaProgress failed: %v", err)
	}

	progress := backup.Users[0].Progress
	if len(progress) != 1 {
		t.Fatalf("expected 1 progress item, got %d", len(progress))
	}

	// In legacy schema with NULL mediaItemId, should fall back to libraryItems.mediaId
	if progress[0].LibraryItemID != "media-from-join" {
		t.Errorf("expected LibraryItemID 'media-from-join' from JOIN fallback, got '%s'", progress[0].LibraryItemID)
	}
}
