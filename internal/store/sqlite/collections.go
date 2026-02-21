package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// collectionColumns is the ordered list of columns selected in collection queries.
// Must match the scan order in scanCollection.
const collectionColumns = `id, created_at, updated_at, library_id, owner_id, name, is_inbox, is_global_access`

// scanCollection scans a sql.Row (or sql.Rows via its Scan method) into a domain.Collection.
func scanCollection(scanner interface{ Scan(dest ...any) error }) (*domain.Collection, error) {
	var c domain.Collection

	var (
		createdAt      string
		updatedAt      string
		isInbox        int
		isGlobalAccess int
	)

	err := scanner.Scan(
		&c.ID,
		&createdAt,
		&updatedAt,
		&c.LibraryID,
		&c.OwnerID,
		&c.Name,
		&isInbox,
		&isGlobalAccess,
	)
	if err != nil {
		return nil, err
	}

	// Parse timestamps.
	c.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	c.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}

	// Boolean fields.
	c.IsInbox = isInbox != 0
	c.IsGlobalAccess = isGlobalAccess != 0

	return &c, nil
}

// loadBookIDs loads all book IDs associated with a collection from the collection_books table.
func (s *Store) loadBookIDs(ctx context.Context, collectionID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT book_id FROM collection_books WHERE collection_id = ?`, collectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bookIDs []string
	for rows.Next() {
		var bookID string
		if err := rows.Scan(&bookID); err != nil {
			return nil, err
		}
		bookIDs = append(bookIDs, bookID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return bookIDs, nil
}

// CreateCollection inserts a collection and its book associations in a transaction.
// Returns store.ErrAlreadyExists on duplicate ID.
func (s *Store) CreateCollection(ctx context.Context, coll *domain.Collection) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO collections (
			id, created_at, updated_at, library_id, owner_id, name, is_inbox, is_global_access
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		coll.ID,
		formatTime(coll.CreatedAt),
		formatTime(coll.UpdatedAt),
		coll.LibraryID,
		coll.OwnerID,
		coll.Name,
		boolToInt(coll.IsInbox),
		boolToInt(coll.IsGlobalAccess),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return store.ErrAlreadyExists
		}
		return err
	}

	// Insert collection_books for each BookID.
	for _, bookID := range coll.BookIDs {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO collection_books (collection_id, book_id)
			VALUES (?, ?)`,
			coll.ID, bookID,
		)
		if err != nil {
			return fmt.Errorf("insert collection_book %s: %w", bookID, err)
		}
	}

	return tx.Commit()
}

// GetCollection retrieves a collection by ID and loads its BookIDs.
// Returns store.ErrNotFound if the collection does not exist.
func (s *Store) GetCollection(ctx context.Context, id string) (*domain.Collection, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+collectionColumns+` FROM collections WHERE id = ?`, id)

	coll, err := scanCollection(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	coll.BookIDs, err = s.loadBookIDs(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load book IDs: %w", err)
	}

	return coll, nil
}

// UpdateCollection updates a collection row and replaces its book associations in a transaction.
// Returns store.ErrNotFound if the collection does not exist.
func (s *Store) UpdateCollection(ctx context.Context, coll *domain.Collection) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		UPDATE collections SET
			created_at = ?,
			updated_at = ?,
			library_id = ?,
			owner_id = ?,
			name = ?,
			is_inbox = ?,
			is_global_access = ?
		WHERE id = ?`,
		formatTime(coll.CreatedAt),
		formatTime(coll.UpdatedAt),
		coll.LibraryID,
		coll.OwnerID,
		coll.Name,
		boolToInt(coll.IsInbox),
		boolToInt(coll.IsGlobalAccess),
		coll.ID,
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

	// Delete all existing collection_books and re-insert.
	if _, err := tx.ExecContext(ctx, `DELETE FROM collection_books WHERE collection_id = ?`, coll.ID); err != nil {
		return err
	}

	for _, bookID := range coll.BookIDs {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO collection_books (collection_id, book_id)
			VALUES (?, ?)`,
			coll.ID, bookID,
		)
		if err != nil {
			return fmt.Errorf("insert collection_book %s: %w", bookID, err)
		}
	}

	return tx.Commit()
}

// DeleteCollection hard-deletes a collection and its associated shares.
// collection_books are removed via ON DELETE CASCADE.
func (s *Store) DeleteCollection(ctx context.Context, id string) error {
	// Delete associated collection_shares first (they also cascade, but be explicit).
	_, err := s.db.ExecContext(ctx, `DELETE FROM collection_shares WHERE collection_id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete collection shares: %w", err)
	}

	result, err := s.db.ExecContext(ctx, `DELETE FROM collections WHERE id = ?`, id)
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

// ListCollectionsByLibrary returns all collections for a library ordered by creation time.
// BookIDs are loaded for each collection.
func (s *Store) ListCollectionsByLibrary(ctx context.Context, libraryID string) ([]*domain.Collection, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+collectionColumns+` FROM collections WHERE library_id = ? ORDER BY created_at`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var collections []*domain.Collection
	for rows.Next() {
		coll, err := scanCollection(rows)
		if err != nil {
			return nil, err
		}
		collections = append(collections, coll)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Load BookIDs for each collection.
	for _, coll := range collections {
		coll.BookIDs, err = s.loadBookIDs(ctx, coll.ID)
		if err != nil {
			return nil, fmt.Errorf("load book IDs for %s: %w", coll.ID, err)
		}
	}

	return collections, nil
}

// AddBookToCollection adds a book to a collection. Does nothing if the association already exists.
func (s *Store) AddBookToCollection(ctx context.Context, collectionID, bookID string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO collection_books (collection_id, book_id)
		VALUES (?, ?)`,
		collectionID, bookID,
	)
	return err
}

// RemoveBookFromCollection removes a book from a collection.
func (s *Store) RemoveBookFromCollection(ctx context.Context, collectionID, bookID string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM collection_books
		WHERE collection_id = ? AND book_id = ?`,
		collectionID, bookID,
	)
	return err
}

// GetCollectionsForBook returns all collections containing a specific book.
// BookIDs are loaded for each returned collection.
func (s *Store) GetCollectionsForBook(ctx context.Context, bookID string) ([]*domain.Collection, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.created_at, c.updated_at, c.library_id, c.owner_id, c.name, c.is_inbox, c.is_global_access
		FROM collections c
		JOIN collection_books cb ON cb.collection_id = c.id
		WHERE cb.book_id = ?`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var collections []*domain.Collection
	for rows.Next() {
		coll, err := scanCollection(rows)
		if err != nil {
			return nil, err
		}
		collections = append(collections, coll)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Load BookIDs for each collection.
	for _, coll := range collections {
		coll.BookIDs, err = s.loadBookIDs(ctx, coll.ID)
		if err != nil {
			return nil, fmt.Errorf("load book IDs for %s: %w", coll.ID, err)
		}
	}

	return collections, nil
}

// --- Collection Shares ---

// shareColumns is the ordered list of columns selected in collection_shares queries.
// Must match the scan order in scanShare.
const shareColumns = `id, created_at, updated_at, deleted_at, collection_id, shared_with_user_id, shared_by_user_id, permission`

// scanShare scans a sql.Row (or sql.Rows via its Scan method) into a domain.CollectionShare.
func scanShare(scanner interface{ Scan(dest ...any) error }) (*domain.CollectionShare, error) {
	var s domain.CollectionShare

	var (
		createdAt string
		updatedAt string
		deletedAt sql.NullString
		permStr   string
	)

	err := scanner.Scan(
		&s.ID,
		&createdAt,
		&updatedAt,
		&deletedAt,
		&s.CollectionID,
		&s.SharedWithUserID,
		&s.SharedByUserID,
		&permStr,
	)
	if err != nil {
		return nil, err
	}

	// Parse timestamps.
	s.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	s.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	s.DeletedAt, err = parseNullableTime(deletedAt)
	if err != nil {
		return nil, err
	}

	// Parse permission.
	perm, ok := domain.ParseSharePermission(permStr)
	if !ok {
		return nil, fmt.Errorf("unknown share permission: %s", permStr)
	}
	s.Permission = perm

	return &s, nil
}

// CreateShare inserts a new collection share.
// Returns store.ErrAlreadyExists on duplicate ID.
func (s *Store) CreateShare(ctx context.Context, share *domain.CollectionShare) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO collection_shares (
			id, created_at, updated_at, deleted_at,
			collection_id, shared_with_user_id, shared_by_user_id, permission
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		share.ID,
		formatTime(share.CreatedAt),
		formatTime(share.UpdatedAt),
		nullTimeString(share.DeletedAt),
		share.CollectionID,
		share.SharedWithUserID,
		share.SharedByUserID,
		share.Permission.String(),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return store.ErrAlreadyExists
		}
		return err
	}
	return nil
}

// GetShare retrieves a share by ID, excluding soft-deleted records.
// Returns store.ErrNotFound if the share does not exist or is soft-deleted.
func (s *Store) GetShare(ctx context.Context, id string) (*domain.CollectionShare, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+shareColumns+` FROM collection_shares WHERE id = ? AND deleted_at IS NULL`, id)

	share, err := scanShare(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return share, nil
}

// GetSharesForUser returns all active shares for a given user.
func (s *Store) GetSharesForUser(ctx context.Context, userID string) ([]*domain.CollectionShare, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+shareColumns+` FROM collection_shares
		WHERE shared_with_user_id = ? AND deleted_at IS NULL`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var shares []*domain.CollectionShare
	for rows.Next() {
		share, err := scanShare(rows)
		if err != nil {
			return nil, err
		}
		shares = append(shares, share)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return shares, nil
}

// GetSharesForCollection returns all active shares for a given collection.
func (s *Store) GetSharesForCollection(ctx context.Context, collectionID string) ([]*domain.CollectionShare, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+shareColumns+` FROM collection_shares
		WHERE collection_id = ? AND deleted_at IS NULL`, collectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var shares []*domain.CollectionShare
	for rows.Next() {
		share, err := scanShare(rows)
		if err != nil {
			return nil, err
		}
		shares = append(shares, share)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return shares, nil
}

// DeleteShare performs a soft delete by setting deleted_at and updated_at.
// Returns store.ErrNotFound if the share does not exist or is already soft-deleted.
func (s *Store) DeleteShare(ctx context.Context, id string) error {
	now := formatTime(time.Now().UTC())

	result, err := s.db.ExecContext(ctx, `
		UPDATE collection_shares SET deleted_at = ?, updated_at = ?
		WHERE id = ? AND deleted_at IS NULL`,
		now, now, id)
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
