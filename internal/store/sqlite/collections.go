package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
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
// The userID parameter is accepted for interface compatibility but not used for access checks.
// Returns store.ErrNotFound if the collection does not exist.
func (s *Store) GetCollection(ctx context.Context, id string, _ string) (*domain.Collection, error) {
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
// The userID parameter is accepted for interface compatibility but not used for access checks.
// Returns store.ErrNotFound if the collection does not exist.
func (s *Store) UpdateCollection(ctx context.Context, coll *domain.Collection, _ string) error {
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
// The userID parameter is accepted for interface compatibility but not used for access checks.
// collection_books are removed via ON DELETE CASCADE.
func (s *Store) DeleteCollection(ctx context.Context, id string, _ string) error {
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
// The userID parameter is accepted for interface compatibility but not used for access checks.
// BookIDs are loaded for each collection.
func (s *Store) ListCollectionsByLibrary(ctx context.Context, libraryID string, _ string) ([]*domain.Collection, error) {
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
// The userID parameter is accepted for interface compatibility but not used for access checks.
func (s *Store) AddBookToCollection(ctx context.Context, bookID, collectionID string, _ string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO collection_books (collection_id, book_id)
		VALUES (?, ?)`,
		collectionID, bookID,
	)
	return err
}

// RemoveBookFromCollection removes a book from a collection.
// The userID parameter is accepted for interface compatibility but not used for access checks.
func (s *Store) RemoveBookFromCollection(ctx context.Context, bookID, collectionID string, _ string) error {
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
// If the share has no ID, one is generated automatically.
// Returns store.ErrAlreadyExists if a non-deleted share already exists for the
// same collection and user, or on duplicate ID.
func (s *Store) CreateShare(ctx context.Context, share *domain.CollectionShare) error {
	if share.ID == "" {
		generated, err := id.Generate("share")
		if err != nil {
			return fmt.Errorf("generate share ID: %w", err)
		}
		share.ID = generated
	}
	if share.CreatedAt.IsZero() {
		share.InitTimestamps()
	}

	// Check for existing active share for the same collection + user.
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM collection_shares
		WHERE collection_id = ? AND shared_with_user_id = ? AND deleted_at IS NULL`,
		share.CollectionID, share.SharedWithUserID).Scan(&count)
	if err != nil {
		return fmt.Errorf("check existing share: %w", err)
	}
	if count > 0 {
		return store.ErrAlreadyExists
	}

	_, err = s.db.ExecContext(ctx, `
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

// GetShareForUserAndCollection finds a specific active share for a user and collection.
// Returns store.ErrNotFound if no active share exists.
func (s *Store) GetShareForUserAndCollection(ctx context.Context, userID, collectionID string) (*domain.CollectionShare, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+shareColumns+` FROM collection_shares
		WHERE shared_with_user_id = ? AND collection_id = ? AND deleted_at IS NULL`,
		userID, collectionID)

	share, err := scanShare(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return share, nil
}

// UpdateShare updates an existing collection share.
// Returns store.ErrNotFound if the share does not exist or is soft-deleted.
func (s *Store) UpdateShare(ctx context.Context, share *domain.CollectionShare) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE collection_shares SET
			updated_at = ?,
			collection_id = ?,
			shared_with_user_id = ?,
			shared_by_user_id = ?,
			permission = ?
		WHERE id = ? AND deleted_at IS NULL`,
		formatTime(share.UpdatedAt),
		share.CollectionID,
		share.SharedWithUserID,
		share.SharedByUserID,
		share.Permission.String(),
		share.ID,
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

// DeleteSharesForCollection soft-deletes all active shares for a given collection.
func (s *Store) DeleteSharesForCollection(ctx context.Context, collectionID string) error {
	now := formatTime(time.Now().UTC())

	_, err := s.db.ExecContext(ctx, `
		UPDATE collection_shares SET deleted_at = ?, updated_at = ?
		WHERE collection_id = ? AND deleted_at IS NULL`,
		now, now, collectionID)
	return err
}

// --- Additional Collection Methods ---

// getCollectionInternal retrieves a collection by ID without access control.
// For internal store use.
func (s *Store) getCollectionInternal(ctx context.Context, id string) (*domain.Collection, error) {
	return s.GetCollection(ctx, id, "")
}

// updateCollectionInternal updates a collection without access control.
// For internal store use.
func (s *Store) updateCollectionInternal(ctx context.Context, coll *domain.Collection) error {
	return s.UpdateCollection(ctx, coll, "")
}

// ListAllCollectionsByLibrary returns ALL collections for a library without access filtering.
// BookIDs are loaded for each collection.
func (s *Store) ListAllCollectionsByLibrary(ctx context.Context, libraryID string) ([]*domain.Collection, error) {
	return s.ListCollectionsByLibrary(ctx, libraryID, "")
}

// AdminGetCollection retrieves a collection by ID without access control.
// For admin use only.
func (s *Store) AdminGetCollection(ctx context.Context, id string) (*domain.Collection, error) {
	return s.GetCollection(ctx, id, "")
}

// AdminListAllCollections returns ALL collections across all libraries.
// For admin use only.
func (s *Store) AdminListAllCollections(ctx context.Context) ([]*domain.Collection, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+collectionColumns+` FROM collections ORDER BY created_at`)
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

// AdminUpdateCollection updates a collection without access control.
// For admin use only.
func (s *Store) AdminUpdateCollection(ctx context.Context, coll *domain.Collection) error {
	return s.UpdateCollection(ctx, coll, "")
}

// AdminDeleteCollection deletes a collection without access control.
// For admin use only. Cannot delete system inbox collections.
func (s *Store) AdminDeleteCollection(ctx context.Context, id string) error {
	return s.DeleteCollection(ctx, id, "")
}

// AdminAddBookToCollection adds a book to a collection without access control.
// For admin use only.
func (s *Store) AdminAddBookToCollection(ctx context.Context, bookID, collectionID string) error {
	return s.AddBookToCollection(ctx, bookID, collectionID, "")
}

// AdminRemoveBookFromCollection removes a book from a collection without access control.
// For admin use only.
func (s *Store) AdminRemoveBookFromCollection(ctx context.Context, bookID, collectionID string) error {
	return s.RemoveBookFromCollection(ctx, bookID, collectionID, "")
}

// EnsureGlobalAccessCollection ensures a global access collection exists for the library.
// Creates one if it doesn't exist, owned by the given owner.
func (s *Store) EnsureGlobalAccessCollection(ctx context.Context, libraryID, ownerID string) (*domain.Collection, error) {
	// Check if one already exists.
	collections, err := s.ListAllCollectionsByLibrary(ctx, libraryID)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}

	for _, coll := range collections {
		if coll.IsGlobalAccess {
			return coll, nil
		}
	}

	// Create one.
	now := time.Now()
	coll := &domain.Collection{
		ID:             fmt.Sprintf("coll-%d", now.UnixNano()),
		LibraryID:      libraryID,
		OwnerID:        ownerID,
		Name:           "Full Library Access",
		BookIDs:        []string{},
		IsInbox:        false,
		IsGlobalAccess: true,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.CreateCollection(ctx, coll); err != nil {
		return nil, fmt.Errorf("create global access collection: %w", err)
	}

	return coll, nil
}

// GetInboxForLibrary returns the Inbox collection for a library.
// If no inbox exists, one is created automatically.
func (s *Store) GetInboxForLibrary(ctx context.Context, libraryID string) (*domain.Collection, error) {
	collections, err := s.ListAllCollectionsByLibrary(ctx, libraryID)
	if err != nil {
		return nil, err
	}

	// Find the one with IsInbox = true.
	for _, coll := range collections {
		if coll.IsInbox {
			return coll, nil
		}
	}

	// No inbox exists - create one (backward compatibility for existing databases).
	library, err := s.GetLibrary(ctx, libraryID)
	if err != nil {
		return nil, fmt.Errorf("get library for inbox creation: %w", err)
	}

	now := time.Now()
	inbox := &domain.Collection{
		ID:        fmt.Sprintf("coll-inbox-%d", now.UnixNano()),
		LibraryID: libraryID,
		OwnerID:   library.OwnerID,
		Name:      "Inbox",
		IsInbox:   true,
		BookIDs:   []string{},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.CreateCollection(ctx, inbox); err != nil {
		return nil, fmt.Errorf("create inbox collection: %w", err)
	}

	return inbox, nil
}
