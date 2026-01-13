package abs

import (
	"archive/zip"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Parser errors.
var (
	ErrNotABSBackup    = errors.New("not an audiobookshelf backup")
	ErrMissingMetadata = errors.New("backup missing metadata")
	ErrInvalidFormat   = errors.New("invalid backup format")
)

// Parse reads an Audiobookshelf backup file and extracts all data.
//
// ABS backups are .audiobookshelf files. Modern versions (2.x+) use ZIP with SQLite:
//
//	backup.audiobookshelf (ZIP)
//	├── details              (version info)
//	├── absdatabase.sqlite   (all data)
//	├── metadata-items/      (per-item JSON)
//	└── metadata-authors/    (author images)
func Parse(path string) (*Backup, error) {
	start := time.Now()
	slog.Info("parsing ABS backup", "path", path)

	// Try to open as ZIP first (modern ABS format)
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("%w: not a valid ZIP archive: %v", ErrNotABSBackup, err)
	}
	defer zr.Close()

	// Look for the SQLite database
	var dbFile *zip.File
	for _, f := range zr.File {
		if f.Name == "absdatabase.sqlite" {
			dbFile = f
			break
		}
	}

	if dbFile == nil {
		return nil, fmt.Errorf("%w: missing absdatabase.sqlite", ErrNotABSBackup)
	}

	slog.Info("found database in archive", "size", dbFile.UncompressedSize64)

	// Extract SQLite to temp file
	tmpFile, err := os.CreateTemp("", "abs-import-*.sqlite")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	rc, err := dbFile.Open()
	if err != nil {
		tmpFile.Close()
		return nil, fmt.Errorf("open database in archive: %w", err)
	}

	_, err = io.Copy(tmpFile, rc)
	rc.Close()
	tmpFile.Close()
	if err != nil {
		return nil, fmt.Errorf("extract database: %w", err)
	}

	slog.Info("extracted database", "duration", time.Since(start))

	// Open SQLite database (using modernc.org/sqlite - pure Go, no CGO)
	db, err := sql.Open("sqlite", tmpPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	// Parse all data from SQLite
	backup := &Backup{Path: path}

	if err := parseUsers(db, backup); err != nil {
		return nil, fmt.Errorf("parse users: %w", err)
	}
	slog.Info("parsed users", "count", len(backup.Users), "duration", time.Since(start))

	if err := parseLibraries(db, backup); err != nil {
		return nil, fmt.Errorf("parse libraries: %w", err)
	}

	if err := parseLibraryItems(db, backup); err != nil {
		return nil, fmt.Errorf("parse library items: %w", err)
	}
	slog.Info("parsed library items", "count", len(backup.Items), "duration", time.Since(start))

	if err := parseAuthors(db, backup); err != nil {
		return nil, fmt.Errorf("parse authors: %w", err)
	}

	if err := parseSeries(db, backup); err != nil {
		return nil, fmt.Errorf("parse series: %w", err)
	}

	if err := parseSessions(db, backup); err != nil {
		return nil, fmt.Errorf("parse sessions: %w", err)
	}
	slog.Info("parsed sessions", "count", len(backup.Sessions), "duration", time.Since(start))

	if err := parseMediaProgress(db, backup); err != nil {
		return nil, fmt.Errorf("parse media progress: %w", err)
	}

	slog.Info("ABS backup parsed successfully",
		"users", len(backup.Users),
		"items", len(backup.Items),
		"sessions", len(backup.Sessions),
		"authors", len(backup.Authors),
		"series", len(backup.Series),
		"duration", time.Since(start),
	)

	return backup, nil
}

func parseUsers(db *sql.DB, backup *Backup) error {
	rows, err := db.Query(`SELECT id, username, COALESCE(email, ''), COALESCE(type, 'user') FROM users`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.Type); err != nil {
			return err
		}
		backup.Users = append(backup.Users, u)
	}
	return rows.Err()
}

func parseLibraries(db *sql.DB, backup *Backup) error {
	rows, err := db.Query(`SELECT id, COALESCE(name, ''), COALESCE(mediaType, 'book') FROM libraries`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var l Library
		if err := rows.Scan(&l.ID, &l.Name, &l.MediaType); err != nil {
			return err
		}
		backup.Libraries = append(backup.Libraries, l)
	}
	return rows.Err()
}

func parseLibraryItems(db *sql.DB, backup *Backup) error {
	// Join libraryItems with books to get full metadata
	// Important: li.mediaId references books.id - this is what sessions use for matching
	query := `
		SELECT
			li.id,
			COALESCE(li.mediaId, ''),
			COALESCE(li.path, ''),
			COALESCE(li.relPath, ''),
			COALESCE(li.libraryId, ''),
			COALESCE(li.mediaType, 'book'),
			COALESCE(li.isMissing, 0),
			COALESCE(li.isInvalid, 0),
			COALESCE(b.title, li.title, ''),
			COALESCE(b.subtitle, ''),
			COALESCE(b.duration, 0),
			COALESCE(b.asin, ''),
			COALESCE(b.isbn, ''),
			COALESCE(b.publisher, ''),
			COALESCE(b.publishedYear, ''),
			COALESCE(b.description, ''),
			COALESCE(b.narrators, '[]'),
			COALESCE(li.authorNamesFirstLast, '')
		FROM libraryItems li
		LEFT JOIN books b ON li.mediaId = b.id
		WHERE li.mediaType = 'book' OR li.mediaType IS NULL
	`

	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var item LibraryItem
		var authorNames, narratorsJSON string

		err := rows.Scan(
			&item.ID,
			&item.MediaID,
			&item.Path,
			&item.RelPath,
			&item.LibraryID,
			&item.MediaType,
			&item.IsMissing,
			&item.IsInvalid,
			&item.Media.Metadata.Title,
			&item.Media.Metadata.Subtitle,
			&item.Media.Duration,
			&item.Media.Metadata.ASIN,
			&item.Media.Metadata.ISBN,
			&item.Media.Metadata.Publisher,
			&item.Media.Metadata.PublishedYear,
			&item.Media.Metadata.Description,
			&narratorsJSON,
			&authorNames,
		)
		if err != nil {
			return err
		}

		// Parse author names (comma-separated in ABS)
		if authorNames != "" {
			for _, name := range strings.Split(authorNames, ", ") {
				name = strings.TrimSpace(name)
				if name != "" {
					item.Media.Metadata.Authors = append(item.Media.Metadata.Authors, PersonRef{Name: name})
				}
			}
		}

		// Parse narrators JSON array
		item.Media.Metadata.Narrators = parseNarratorsJSON(narratorsJSON)

		if item.MediaType == "" {
			item.MediaType = "book"
		}

		backup.Items = append(backup.Items, item)
	}
	return rows.Err()
}

func parseNarratorsJSON(jsonStr string) []PersonRef {
	// Simple JSON array parsing for ["name1", "name2"] format
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" || jsonStr == "[]" || jsonStr == "null" {
		return nil
	}

	// Remove brackets
	jsonStr = strings.TrimPrefix(jsonStr, "[")
	jsonStr = strings.TrimSuffix(jsonStr, "]")

	var narrators []PersonRef
	for _, part := range strings.Split(jsonStr, ",") {
		name := strings.TrimSpace(part)
		name = strings.Trim(name, `"`)
		if name != "" {
			narrators = append(narrators, PersonRef{Name: name})
		}
	}
	return narrators
}

func parseAuthors(db *sql.DB, backup *Backup) error {
	rows, err := db.Query(`SELECT id, COALESCE(name, ''), COALESCE(asin, ''), COALESCE(description, '') FROM authors`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var a Author
		if err := rows.Scan(&a.ID, &a.Name, &a.ASIN, &a.Description); err != nil {
			return err
		}
		backup.Authors = append(backup.Authors, a)
	}
	return rows.Err()
}

func parseSeries(db *sql.DB, backup *Backup) error {
	rows, err := db.Query(`SELECT id, COALESCE(name, ''), COALESCE(description, '') FROM series`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var s Series
		if err := rows.Scan(&s.ID, &s.Name, &s.Description); err != nil {
			return err
		}
		backup.Series = append(backup.Series, s)
	}
	return rows.Err()
}

func parseSessions(db *sql.DB, backup *Backup) error {
	// Note: playbackSessions uses mediaItemId which references books.id
	// We store this in LibraryItemID for consistency with our matching logic
	// (we match using libraryItems.mediaId which also references books.id)
	query := `
		SELECT
			id,
			COALESCE(userId, ''),
			COALESCE(libraryId, ''),
			COALESCE(mediaItemId, ''),
			COALESCE(mediaItemType, 'book'),
			COALESCE(displayTitle, ''),
			COALESCE(displayAuthor, ''),
			COALESCE(duration, 0),
			COALESCE(timeListening, 0),
			COALESCE(startTime, 0),
			COALESCE(currentTime, 0),
			COALESCE(date, ''),
			COALESCE(dayOfWeek, ''),
			COALESCE(strftime('%s', createdAt) * 1000, 0),
			COALESCE(strftime('%s', updatedAt) * 1000, 0)
		FROM playbackSessions
		WHERE mediaItemType = 'book' OR mediaItemType IS NULL
	`

	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var s Session
		err := rows.Scan(
			&s.ID,
			&s.UserID,
			&s.LibraryID,
			&s.LibraryItemID,
			&s.MediaType,
			&s.DisplayTitle,
			&s.DisplayAuthor,
			&s.Duration,
			&s.TimeListening,
			&s.StartTime,
			&s.CurrentTime,
			&s.Date,
			&s.DayOfWeek,
			&s.StartedAt,
			&s.UpdatedAt,
		)
		if err != nil {
			return err
		}

		if s.MediaType == "" {
			s.MediaType = "book"
		}

		backup.Sessions = append(backup.Sessions, s)
	}
	return rows.Err()
}

func parseMediaProgress(db *sql.DB, backup *Backup) error {
	// Note: mediaProgresses uses mediaItemId which references books.id
	// We store this in LibraryItemID for consistency with our matching logic
	query := `
		SELECT
			id,
			userId,
			COALESCE(mediaItemId, ''),
			COALESCE(mediaItemType, 'book'),
			COALESCE(duration, 0),
			COALESCE(currentTime, 0),
			COALESCE(isFinished, 0),
			COALESCE(hideFromContinueListening, 0),
			COALESCE(strftime('%s', updatedAt) * 1000, 0),
			COALESCE(strftime('%s', createdAt) * 1000, 0),
			COALESCE(strftime('%s', finishedAt) * 1000, 0)
		FROM mediaProgresses
		WHERE mediaItemType = 'book' OR mediaItemType IS NULL
	`

	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	// Build a map of user ID -> progress items
	progressMap := make(map[string][]MediaProgress)

	for rows.Next() {
		var p MediaProgress
		var userID string
		var isFinished, hideFromContinue int

		err := rows.Scan(
			&p.ID,
			&userID,
			&p.LibraryItemID,
			&p.MediaItemType,
			&p.Duration,
			&p.CurrentTime,
			&isFinished,
			&hideFromContinue,
			&p.LastUpdate,
			&p.StartedAt,
			&p.FinishedAt,
		)
		if err != nil {
			return err
		}

		p.IsFinished = isFinished != 0
		p.HideFromContinue = hideFromContinue != 0

		if p.Duration > 0 {
			p.Progress = p.CurrentTime / p.Duration
		}

		if p.MediaItemType == "" {
			p.MediaItemType = "book"
		}

		progressMap[userID] = append(progressMap[userID], p)
	}

	if err := rows.Err(); err != nil {
		return err
	}

	// Attach progress to users
	for i := range backup.Users {
		if progress, ok := progressMap[backup.Users[i].ID]; ok {
			backup.Users[i].Progress = progress
		}
	}

	return nil
}

// Summary returns a human-readable summary of the backup contents.
func (b *Backup) Summary() string {
	var users, guests int
	for _, u := range b.Users {
		if u.Type == "guest" {
			guests++
		} else {
			users++
		}
	}

	var books, podcasts int
	for _, item := range b.Items {
		if item.IsBook() {
			books++
		} else {
			podcasts++
		}
	}

	var bookSessions, podcastSessions int
	for _, s := range b.Sessions {
		if s.IsBook() {
			bookSessions++
		} else {
			podcastSessions++
		}
	}

	return fmt.Sprintf(
		"ABS Backup: %d users (%d guests), %d libraries, %d books, %d podcasts, %d sessions (books), %d sessions (podcasts), %d authors, %d series",
		users, guests, len(b.Libraries), books, podcasts, bookSessions, podcastSessions, len(b.Authors), len(b.Series),
	)
}

// BookItems returns only the audiobook items (not podcasts).
func (b *Backup) BookItems() []LibraryItem {
	var items []LibraryItem
	for _, item := range b.Items {
		if item.IsValid() {
			items = append(items, item)
		}
	}
	return items
}

// BookSessions returns only audiobook listening sessions.
func (b *Backup) BookSessions() []Session {
	var sessions []Session
	for _, s := range b.Sessions {
		if s.IsBook() {
			sessions = append(sessions, s)
		}
	}
	return sessions
}

// ImportableUsers returns users that should be imported (excludes guests).
func (b *Backup) ImportableUsers() []User {
	var users []User
	for _, u := range b.Users {
		if u.IsImportable() {
			users = append(users, u)
		}
	}
	return users
}

// BookLibraries returns only audiobook libraries (not podcast libraries).
func (b *Backup) BookLibraries() []Library {
	var libs []Library
	for _, l := range b.Libraries {
		if l.IsBookLibrary() {
			libs = append(libs, l)
		}
	}
	return libs
}
