package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// prefixed column lists for join queries.
var (
	collectionColumnsAliased = "c.id, c.created_at, c.updated_at, c.library_id, c.owner_id, c.name, c.is_inbox, c.is_global_access"
	bookColumnsAliased       = "b.id, b.created_at, b.updated_at, b.deleted_at, b.scanned_at, " +
		"b.isbn, b.title, b.subtitle, b.path, b.description, b.publisher, b.publish_year, " +
		"b.language, b.asin, b.audible_region, " +
		"b.total_duration, b.total_size, b.abridged, " +
		"b.cover_path, b.cover_filename, b.cover_format, b.cover_size, " +
		"b.cover_inode, b.cover_mod_time, b.cover_blur_hash, " +
		"b.staged_collection_ids"
)

// GetCollectionsForUser returns all collections a user has access to,
// either by owning them or by having an active (non-deleted) share.
func (s *Store) GetCollectionsForUser(ctx context.Context, userID string) ([]*domain.Collection, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT `+collectionColumnsAliased+`
		FROM collections c
		LEFT JOIN collection_shares cs
			ON cs.collection_id = c.id AND cs.shared_with_user_id = ? AND cs.deleted_at IS NULL
		WHERE c.owner_id = ? OR cs.id IS NOT NULL`,
		userID, userID)
	if err != nil {
		return nil, fmt.Errorf("query collections for user: %w", err)
	}
	defer rows.Close()

	var collections []*domain.Collection
	for rows.Next() {
		coll, err := scanCollection(rows)
		if err != nil {
			return nil, fmt.Errorf("scan collection: %w", err)
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

// GetCollectionsContainingBook returns all collections that contain the given book
// via the collection_books join table.
func (s *Store) GetCollectionsContainingBook(ctx context.Context, bookID string) ([]*domain.Collection, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+collectionColumnsAliased+`
		FROM collections c
		JOIN collection_books cb ON cb.collection_id = c.id
		WHERE cb.book_id = ?`, bookID)
	if err != nil {
		return nil, fmt.Errorf("query collections containing book: %w", err)
	}
	defer rows.Close()

	var collections []*domain.Collection
	for rows.Next() {
		coll, err := scanCollection(rows)
		if err != nil {
			return nil, fmt.Errorf("scan collection: %w", err)
		}
		collections = append(collections, coll)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, coll := range collections {
		coll.BookIDs, err = s.loadBookIDs(ctx, coll.ID)
		if err != nil {
			return nil, fmt.Errorf("load book IDs for %s: %w", coll.ID, err)
		}
	}

	return collections, nil
}

// GetBooksForUser returns all non-deleted books that the user can access.
// A book is accessible if:
//   - It is not in any collection (accessible to all), or
//   - It belongs to a global-access collection, or
//   - It belongs to a collection the user owns or has been shared.
func (s *Store) GetBooksForUser(ctx context.Context, userID string) ([]*domain.Book, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT `+bookColumnsAliased+`
		FROM books b
		WHERE b.deleted_at IS NULL AND (
			-- Books not in any collection are accessible to all.
			NOT EXISTS (SELECT 1 FROM collection_books cb2 WHERE cb2.book_id = b.id)
			OR EXISTS (
				SELECT 1 FROM collection_books cb
				JOIN collections c ON c.id = cb.collection_id
				LEFT JOIN collection_shares cs
					ON cs.collection_id = c.id AND cs.shared_with_user_id = ? AND cs.deleted_at IS NULL
				WHERE cb.book_id = b.id
					AND (c.owner_id = ? OR c.is_global_access = 1 OR cs.id IS NOT NULL)
			)
		)`,
		userID, userID)
	if err != nil {
		return nil, fmt.Errorf("query books for user: %w", err)
	}
	defer rows.Close()

	var books []*domain.Book
	for rows.Next() {
		b, err := scanBook(rows)
		if err != nil {
			return nil, fmt.Errorf("scan book: %w", err)
		}
		books = append(books, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Load audio files and chapters for each book.
	for _, b := range books {
		b.AudioFiles, err = s.loadBookAudioFiles(ctx, s.db, b.ID)
		if err != nil {
			return nil, fmt.Errorf("load audio files for %s: %w", b.ID, err)
		}
		b.Chapters, err = s.loadBookChapters(ctx, s.db, b.ID)
		if err != nil {
			return nil, fmt.Errorf("load chapters for %s: %w", b.ID, err)
		}
	}

	return books, nil
}

// GetBooksForUserUpdatedAfter returns non-deleted books accessible by the user
// that have been updated after the given timestamp.
func (s *Store) GetBooksForUserUpdatedAfter(ctx context.Context, userID string, timestamp time.Time) ([]*domain.Book, error) {
	ts := formatTime(timestamp)

	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT `+bookColumnsAliased+`
		FROM books b
		WHERE b.deleted_at IS NULL
			AND b.updated_at > ?
			AND (
				NOT EXISTS (SELECT 1 FROM collection_books cb2 WHERE cb2.book_id = b.id)
				OR EXISTS (
					SELECT 1 FROM collection_books cb
					JOIN collections c ON c.id = cb.collection_id
					LEFT JOIN collection_shares cs
						ON cs.collection_id = c.id AND cs.shared_with_user_id = ? AND cs.deleted_at IS NULL
					WHERE cb.book_id = b.id
						AND (c.owner_id = ? OR c.is_global_access = 1 OR cs.id IS NOT NULL)
				)
			)
		ORDER BY b.updated_at ASC`,
		ts, userID, userID)
	if err != nil {
		return nil, fmt.Errorf("query books for user updated after: %w", err)
	}
	defer rows.Close()

	var books []*domain.Book
	for rows.Next() {
		b, err := scanBook(rows)
		if err != nil {
			return nil, fmt.Errorf("scan book: %w", err)
		}
		books = append(books, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, b := range books {
		b.AudioFiles, err = s.loadBookAudioFiles(ctx, s.db, b.ID)
		if err != nil {
			return nil, fmt.Errorf("load audio files for %s: %w", b.ID, err)
		}
		b.Chapters, err = s.loadBookChapters(ctx, s.db, b.ID)
		if err != nil {
			return nil, fmt.Errorf("load chapters for %s: %w", b.ID, err)
		}
	}

	return books, nil
}

// CanUserAccessBook checks whether the user can access the given book.
// A user can access a book if:
//   - It is not in any collection (accessible to all), or
//   - It belongs to a global-access collection, or
//   - It belongs to a collection the user owns or has been shared.
func (s *Store) CanUserAccessBook(ctx context.Context, userID, bookID string) (bool, error) {
	// First verify the book exists.
	var exists int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM books WHERE id = ? AND deleted_at IS NULL`,
		bookID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check book exists: %w", err)
	}
	if exists == 0 {
		return false, store.ErrBookNotFound
	}

	// If book is not in any collection, it's accessible to all.
	var count int
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM collection_books WHERE book_id = ?`,
		bookID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check book in collections: %w", err)
	}
	if count == 0 {
		return true, nil
	}

	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM collection_books cb
		JOIN collections c ON c.id = cb.collection_id
		LEFT JOIN collection_shares cs
			ON cs.collection_id = c.id AND cs.shared_with_user_id = ? AND cs.deleted_at IS NULL
		WHERE cb.book_id = ?
			AND (c.owner_id = ? OR c.is_global_access = 1 OR cs.id IS NOT NULL)`,
		userID, bookID, userID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check user access to book: %w", err)
	}
	return count > 0, nil
}

// CanUserAccessCollection checks whether the user can access the given collection.
// Returns (canAccess, permission, isOwner, error).
// A user can access a collection if they own it or have an active share.
// If the user is the owner, permission is PermissionWrite and isOwner is true.
// If the user has a share, the share's permission is returned and isOwner is false.
func (s *Store) CanUserAccessCollection(ctx context.Context, userID, collectionID string) (bool, domain.SharePermission, bool, error) {
	// First check ownership.
	var ownerID string
	err := s.db.QueryRowContext(ctx,
		`SELECT owner_id FROM collections WHERE id = ?`, collectionID).Scan(&ownerID)
	if err == sql.ErrNoRows {
		return false, domain.PermissionRead, false, nil
	}
	if err != nil {
		return false, domain.PermissionRead, false, fmt.Errorf("check collection owner: %w", err)
	}

	if ownerID == userID {
		return true, domain.PermissionWrite, true, nil
	}

	// Check for an active share.
	var permStr string
	err = s.db.QueryRowContext(ctx, `
		SELECT permission FROM collection_shares
		WHERE collection_id = ? AND shared_with_user_id = ? AND deleted_at IS NULL`,
		collectionID, userID).Scan(&permStr)
	if err == sql.ErrNoRows {
		return false, domain.PermissionRead, false, nil
	}
	if err != nil {
		return false, domain.PermissionRead, false, fmt.Errorf("check collection share: %w", err)
	}

	perm, ok := domain.ParseSharePermission(permStr)
	if !ok {
		return false, domain.PermissionRead, false, fmt.Errorf("unknown share permission: %s", permStr)
	}

	return true, perm, false, nil
}

// GetAccessibleBookIDSet returns the set of book IDs the user can access.
// This is a convenience wrapper around GetBooksForUser that returns just IDs
// as a map for efficient lookups.
func (s *Store) GetAccessibleBookIDSet(ctx context.Context, userID string) (map[string]bool, error) {
	books, err := s.GetBooksForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	ids := make(map[string]bool, len(books))
	for _, b := range books {
		ids[b.ID] = true
	}
	return ids, nil
}
