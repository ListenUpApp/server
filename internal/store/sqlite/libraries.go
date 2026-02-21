package sqlite

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"fmt"
	"slices"
	"strings"
	"time"

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

// GetDefaultLibrary returns the first library in the store.
// Returns store.ErrNotFound if no libraries exist.
func (s *Store) GetDefaultLibrary(ctx context.Context) (*domain.Library, error) {
	libraries, err := s.ListLibraries(ctx)
	if err != nil {
		return nil, err
	}

	if len(libraries) == 0 {
		return nil, store.ErrNotFound
	}

	return libraries[0], nil
}

// DeleteLibrary deletes a library and all its collections in a single transaction.
func (s *Store) DeleteLibrary(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Get all collection IDs for this library.
	rows, err := tx.QueryContext(ctx,
		`SELECT id FROM collections WHERE library_id = ?`, id)
	if err != nil {
		return fmt.Errorf("list collections: %w", err)
	}
	var collIDs []string
	for rows.Next() {
		var cid string
		if err := rows.Scan(&cid); err != nil {
			rows.Close()
			return fmt.Errorf("scan collection id: %w", err)
		}
		collIDs = append(collIDs, cid)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows iteration: %w", err)
	}

	// Delete shares and collections.
	for _, cid := range collIDs {
		if _, err := tx.ExecContext(ctx, `DELETE FROM collection_shares WHERE collection_id = ?`, cid); err != nil {
			return fmt.Errorf("delete shares for collection %s: %w", cid, err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM collection_books WHERE collection_id = ?`, cid); err != nil {
			return fmt.Errorf("delete collection_books for %s: %w", cid, err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM collections WHERE id = ?`, cid); err != nil {
			return fmt.Errorf("delete collection %s: %w", cid, err)
		}
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM libraries WHERE id = ?`, id)
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

	return tx.Commit()
}

// EnsureLibrary ensures a library exists with the given scan path and owner.
// If no library exists, creates one with an inbox collection in a single transaction.
// If a library exists, adds the scan path if not already present.
// Returns the library and its inbox collection.
func (s *Store) EnsureLibrary(ctx context.Context, scanPath string, userID string) (*store.BootstrapResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	result := &store.BootstrapResult{}

	// Try to get existing library.
	row := tx.QueryRowContext(ctx, `SELECT `+libraryColumns+` FROM libraries ORDER BY created_at ASC LIMIT 1`)
	library, err := scanLibrary(row)

	if err == sql.ErrNoRows {
		// No library exists - create everything from scratch.
		now := time.Now()

		libraryID := fmt.Sprintf("lib-%d", now.UnixNano())
		inboxCollID := fmt.Sprintf("coll-%d", now.UnixNano())

		scanPathsJSON, jsonErr := json.Marshal([]string{scanPath})
		if jsonErr != nil {
			return nil, jsonErr
		}

		library = &domain.Library{
			ID:        libraryID,
			OwnerID:   userID,
			Name:      "My Library",
			ScanPaths: []string{scanPath},
			SkipInbox: false,
			CreatedAt: now,
			UpdatedAt: now,
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO libraries (id, created_at, updated_at, owner_id, name, scan_paths, skip_inbox, access_mode)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			library.ID, formatTime(now), formatTime(now),
			userID, "My Library", string(scanPathsJSON), 0, "")
		if err != nil {
			return nil, fmt.Errorf("create library: %w", err)
		}

		result.IsNewLibrary = true

		// Create inbox collection.
		inboxColl := &domain.Collection{
			ID:        inboxCollID,
			LibraryID: library.ID,
			OwnerID:   userID,
			Name:      "Inbox",
			IsInbox:   true,
			BookIDs:   []string{},
			CreatedAt: now,
			UpdatedAt: now,
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO collections (id, created_at, updated_at, library_id, owner_id, name, is_inbox, is_global_access)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			inboxColl.ID, formatTime(now), formatTime(now),
			library.ID, userID, "Inbox", 1, 0)
		if err != nil {
			return nil, fmt.Errorf("create inbox collection: %w", err)
		}

		result.InboxCollection = inboxColl
	} else if err != nil {
		return nil, fmt.Errorf("get default library: %w", err)
	} else {
		// Library exists - ensure scan path is included.
		result.IsNewLibrary = false

		hasPath := slices.Contains(library.ScanPaths, scanPath)

		if !hasPath {
			library.ScanPaths = append(library.ScanPaths, scanPath)
			library.UpdatedAt = time.Now()

			scanPathsJSON, jsonErr := json.Marshal(library.ScanPaths)
			if jsonErr != nil {
				return nil, jsonErr
			}

			_, err = tx.ExecContext(ctx, `
				UPDATE libraries SET scan_paths = ?, updated_at = ? WHERE id = ?`,
				string(scanPathsJSON), formatTime(library.UpdatedAt), library.ID)
			if err != nil {
				return nil, fmt.Errorf("update library: %w", err)
			}
		}

		// Get existing inbox collection.
		inboxRows, err := tx.QueryContext(ctx,
			`SELECT `+collectionColumns+` FROM collections WHERE library_id = ? ORDER BY created_at`, library.ID)
		if err != nil {
			return nil, fmt.Errorf("list collections: %w", err)
		}
		defer inboxRows.Close()

		for inboxRows.Next() {
			coll, err := scanCollection(inboxRows)
			if err != nil {
				return nil, err
			}
			if coll.IsInbox {
				result.InboxCollection = coll
				break
			}
		}
		if err := inboxRows.Err(); err != nil {
			return nil, err
		}

		if result.InboxCollection == nil {
			return nil, fmt.Errorf("inbox collection not found for library %s", library.ID)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	result.Library = library
	return result, nil
}
