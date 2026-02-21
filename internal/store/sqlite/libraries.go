package sqlite

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
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

// DeleteLibrary deletes a library and all its collections.
// This is a destructive operation.
func (s *Store) DeleteLibrary(ctx context.Context, id string) error {
	// Get all collections for this library.
	collections, err := s.ListAllCollectionsByLibrary(ctx, id)
	if err != nil {
		return fmt.Errorf("list collections: %w", err)
	}

	// Delete all collections first (including their shares).
	for _, coll := range collections {
		if err := s.DeleteSharesForCollection(ctx, coll.ID); err != nil {
			return fmt.Errorf("delete shares for collection %s: %w", coll.ID, err)
		}
		if err := s.AdminDeleteCollection(ctx, coll.ID); err != nil {
			return fmt.Errorf("delete collection %s: %w", coll.ID, err)
		}
	}

	result, err := s.db.ExecContext(ctx, `DELETE FROM libraries WHERE id = ?`, id)
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

// EnsureLibrary ensures a library exists with the given scan path and owner.
// If no library exists, creates one with an inbox collection.
// If a library exists, adds the scan path if not already present.
// Returns the library and its inbox collection.
func (s *Store) EnsureLibrary(ctx context.Context, scanPath string, userID string) (*store.BootstrapResult, error) {
	result := &store.BootstrapResult{}

	// Try to get existing library.
	library, err := s.GetDefaultLibrary(ctx)

	var storeErr *store.Error
	switch {
	case errors.As(err, &storeErr) && storeErr.Code == store.ErrNotFound.Code:
		// No library exists - create everything from scratch.
		now := time.Now()

		libraryID := fmt.Sprintf("lib-%d", now.UnixNano())
		inboxCollID := fmt.Sprintf("coll-%d", now.UnixNano())

		// Create library.
		library = &domain.Library{
			ID:        libraryID,
			OwnerID:   userID,
			Name:      "My Library",
			ScanPaths: []string{scanPath},
			SkipInbox: false,
			CreatedAt: now,
			UpdatedAt: now,
		}

		if err := s.CreateLibrary(ctx, library); err != nil {
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

		if err := s.CreateCollection(ctx, inboxColl); err != nil {
			return nil, fmt.Errorf("create inbox collection: %w", err)
		}

		result.InboxCollection = inboxColl
	case err != nil:
		return nil, fmt.Errorf("get default library: %w", err)
	default:
		// Library exists - ensure scan path is included.
		result.IsNewLibrary = false

		hasPath := slices.Contains(library.ScanPaths, scanPath)

		if !hasPath {
			library.ScanPaths = append(library.ScanPaths, scanPath)
			library.UpdatedAt = time.Now()

			if err := s.UpdateLibrary(ctx, library); err != nil {
				return nil, fmt.Errorf("update library: %w", err)
			}
		}

		// Get existing inbox collection.
		inboxColl, err := s.GetInboxForLibrary(ctx, library.ID)
		if err != nil {
			return nil, fmt.Errorf("get inbox collection: %w", err)
		}
		result.InboxCollection = inboxColl
	}

	result.Library = library
	return result, nil
}
