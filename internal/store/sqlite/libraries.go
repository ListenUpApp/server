package sqlite

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"strings"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// libraryColumns is the ordered list of columns selected in library queries.
// Must match the scan order in scanLibrary.
const libraryColumns = `id, created_at, updated_at, owner_id, name, scan_paths, skip_inbox, access_mode`

// scanLibrary scans a sql.Row (or sql.Rows via its Scan method) into a domain.Library.
func scanLibrary(scanner interface{ Scan(dest ...any) error }) (*domain.Library, error) {
	var lib domain.Library

	var (
		createdAt  string
		updatedAt  string
		scanPaths  string
		skipInbox  int
		accessMode string
	)

	err := scanner.Scan(
		&lib.ID,
		&createdAt,
		&updatedAt,
		&lib.OwnerID,
		&lib.Name,
		&scanPaths,
		&skipInbox,
		&accessMode,
	)
	if err != nil {
		return nil, err
	}

	// Parse timestamps.
	lib.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	lib.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}

	// Parse scan_paths JSON array.
	if err := json.Unmarshal([]byte(scanPaths), &lib.ScanPaths); err != nil {
		return nil, err
	}

	// Boolean fields.
	lib.SkipInbox = skipInbox != 0

	// Enum fields.
	lib.AccessMode = domain.AccessMode(accessMode)

	return &lib, nil
}

// CreateLibrary inserts a new library into the database.
// Returns store.ErrAlreadyExists on duplicate ID.
func (s *Store) CreateLibrary(ctx context.Context, lib *domain.Library) error {
	scanPathsJSON, err := json.Marshal(lib.ScanPaths)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO libraries (
			id, created_at, updated_at, owner_id, name, scan_paths, skip_inbox, access_mode
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		lib.ID,
		formatTime(lib.CreatedAt),
		formatTime(lib.UpdatedAt),
		lib.OwnerID,
		lib.Name,
		string(scanPathsJSON),
		boolToInt(lib.SkipInbox),
		string(lib.AccessMode),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return store.ErrAlreadyExists
		}
		return err
	}
	return nil
}

// GetLibrary retrieves a library by ID.
// Returns store.ErrNotFound if the library does not exist.
func (s *Store) GetLibrary(ctx context.Context, id string) (*domain.Library, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+libraryColumns+` FROM libraries WHERE id = ?`, id)

	lib, err := scanLibrary(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return lib, nil
}

// ListLibraries returns all libraries ordered by creation time.
func (s *Store) ListLibraries(ctx context.Context) ([]*domain.Library, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+libraryColumns+` FROM libraries ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var libraries []*domain.Library
	for rows.Next() {
		lib, err := scanLibrary(rows)
		if err != nil {
			return nil, err
		}
		libraries = append(libraries, lib)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return libraries, nil
}

// UpdateLibrary performs a full row update on an existing library.
// Returns store.ErrNotFound if the library does not exist.
func (s *Store) UpdateLibrary(ctx context.Context, lib *domain.Library) error {
	scanPathsJSON, err := json.Marshal(lib.ScanPaths)
	if err != nil {
		return err
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE libraries SET
			created_at = ?,
			updated_at = ?,
			owner_id = ?,
			name = ?,
			scan_paths = ?,
			skip_inbox = ?,
			access_mode = ?
		WHERE id = ?`,
		formatTime(lib.CreatedAt),
		formatTime(lib.UpdatedAt),
		lib.OwnerID,
		lib.Name,
		string(scanPathsJSON),
		boolToInt(lib.SkipInbox),
		string(lib.AccessMode),
		lib.ID,
	)
	if err != nil {
		return err
	}

	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}
