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

// tagColumns is the ordered list of columns selected in tag queries.
// Must match the scan order in scanTag.
const tagColumns = `id, slug, created_at, updated_at`

// scanTag scans a sql.Row (or sql.Rows via its Scan method) into a domain.Tag.
// BookCount is left as 0; the caller can compute it if needed.
func scanTag(scanner interface{ Scan(dest ...any) error }) (*domain.Tag, error) {
	var t domain.Tag

	var (
		createdAt string
		updatedAt string
	)

	err := scanner.Scan(
		&t.ID,
		&t.Slug,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, err
	}

	t.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	t.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}

	return &t, nil
}

// CreateTag inserts a new tag into the database.
// Returns store.ErrAlreadyExists on duplicate slug.
func (s *Store) CreateTag(ctx context.Context, t *domain.Tag) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tags (id, slug, created_at, updated_at)
		VALUES (?, ?, ?, ?)`,
		t.ID,
		t.Slug,
		formatTime(t.CreatedAt),
		formatTime(t.UpdatedAt),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return store.ErrAlreadyExists
		}
		return err
	}
	return nil
}

// GetTagByID retrieves a tag by its ID, including its book count.
// Returns store.ErrNotFound if the tag does not exist.
func (s *Store) GetTagByID(ctx context.Context, tagID string) (*domain.Tag, error) {
	var t domain.Tag
	var createdAt, updatedAt string

	err := s.db.QueryRowContext(ctx, `
		SELECT t.id, t.slug, t.created_at, t.updated_at,
			(SELECT COUNT(*) FROM book_tags bt WHERE bt.tag_id = t.id) AS book_count
		FROM tags t WHERE t.id = ?`, tagID).Scan(
		&t.ID, &t.Slug, &createdAt, &updatedAt, &t.BookCount,
	)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	t.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	t.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// GetTagBySlug retrieves a tag by its slug, including its book count.
// Returns store.ErrNotFound if the tag does not exist.
func (s *Store) GetTagBySlug(ctx context.Context, slug string) (*domain.Tag, error) {
	var t domain.Tag
	var createdAt, updatedAt string

	err := s.db.QueryRowContext(ctx, `
		SELECT t.id, t.slug, t.created_at, t.updated_at,
			(SELECT COUNT(*) FROM book_tags bt WHERE bt.tag_id = t.id) AS book_count
		FROM tags t WHERE t.slug = ?`, slug).Scan(
		&t.ID, &t.Slug, &createdAt, &updatedAt, &t.BookCount,
	)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	t.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	t.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ListTags returns all tags ordered by slug.
func (s *Store) ListTags(ctx context.Context) ([]*domain.Tag, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.slug, t.created_at, t.updated_at,
			(SELECT COUNT(*) FROM book_tags bt WHERE bt.tag_id = t.id) AS book_count
		FROM tags t
		ORDER BY t.slug ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []*domain.Tag
	for rows.Next() {
		var t domain.Tag
		var createdAt, updatedAt string
		err := rows.Scan(&t.ID, &t.Slug, &createdAt, &updatedAt, &t.BookCount)
		if err != nil {
			return nil, err
		}
		t.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		t.UpdatedAt, err = parseTime(updatedAt)
		if err != nil {
			return nil, err
		}
		tags = append(tags, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if tags == nil {
		tags = []*domain.Tag{}
	}

	return tags, nil
}

// FindOrCreateTagBySlug finds an existing tag by slug or creates a new one.
// Returns (tag, created, error) where created is true if a new tag was made.
func (s *Store) FindOrCreateTagBySlug(ctx context.Context, slug string) (*domain.Tag, bool, error) {
	// Try to find existing tag first.
	existing, err := s.GetTagBySlug(ctx, slug)
	if err == nil {
		return existing, false, nil
	}
	if err != store.ErrNotFound {
		return nil, false, err
	}

	// Tag doesn't exist, create it.
	tagID, err := id.Generate("tag")
	if err != nil {
		return nil, false, fmt.Errorf("generate tag id: %w", err)
	}

	now := time.Now().UTC()
	t := &domain.Tag{
		ID:        tagID,
		Slug:      slug,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.CreateTag(ctx, t); err != nil {
		if err == store.ErrAlreadyExists {
			// Race condition: another goroutine created it.
			existing, err := s.GetTagBySlug(ctx, slug)
			if err != nil {
				return nil, false, err
			}
			return existing, false, nil
		}
		return nil, false, err
	}

	return t, true, nil
}

// SetBookTags replaces all tags for a book in a single transaction.
// It deletes existing book_tags rows and inserts the new set.
func (s *Store) SetBookTags(ctx context.Context, bookID string, tagIDs []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Delete existing tags for this book.
	if _, err := tx.ExecContext(ctx, `DELETE FROM book_tags WHERE book_id = ?`, bookID); err != nil {
		return fmt.Errorf("delete book_tags: %w", err)
	}

	// Insert new tag associations.
	now := formatTime(time.Now().UTC())
	for _, tagID := range tagIDs {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO book_tags (book_id, tag_id, created_at)
			VALUES (?, ?, ?)`,
			bookID,
			tagID,
			now,
		)
		if err != nil {
			return fmt.Errorf("insert book_tag: %w", err)
		}
	}

	return tx.Commit()
}

// GetBookTags returns the tag IDs associated with a book.
func (s *Store) GetBookTags(ctx context.Context, bookID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT tag_id FROM book_tags WHERE book_id = ?`, bookID)
	if err != nil {
		return nil, fmt.Errorf("query book_tags: %w", err)
	}
	defer rows.Close()

	var tagIDs []string
	for rows.Next() {
		var tagID string
		if err := rows.Scan(&tagID); err != nil {
			return nil, fmt.Errorf("scan book_tag: %w", err)
		}
		tagIDs = append(tagIDs, tagID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	return tagIDs, nil
}

// DeleteTag removes a tag by ID. Also removes all book_tags associations.
func (s *Store) DeleteTag(ctx context.Context, tagID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Delete book_tags associations first.
	if _, err := tx.ExecContext(ctx, `DELETE FROM book_tags WHERE tag_id = ?`, tagID); err != nil {
		return fmt.Errorf("delete book_tags: %w", err)
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM tags WHERE id = ?`, tagID)
	if err != nil {
		return fmt.Errorf("delete tag: %w", err)
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

// AddTagToBook adds a tag to a book.
// Uses INSERT OR IGNORE for idempotency.
func (s *Store) AddTagToBook(ctx context.Context, bookID, tagID string) error {
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO book_tags (book_id, tag_id, created_at)
		VALUES (?, ?, ?)`, bookID, tagID, now)
	return err
}

// RemoveTagFromBook removes a tag from a book.
func (s *Store) RemoveTagFromBook(ctx context.Context, bookID, tagID string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM book_tags WHERE book_id = ? AND tag_id = ?`, bookID, tagID)
	return err
}

// GetTagsForBook returns all tags associated with a book (full tag objects via JOIN).
func (s *Store) GetTagsForBook(ctx context.Context, bookID string) ([]*domain.Tag, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.slug, t.created_at, t.updated_at
		FROM tags t
		INNER JOIN book_tags bt ON bt.tag_id = t.id
		WHERE bt.book_id = ?
		ORDER BY t.slug`, bookID)
	if err != nil {
		return nil, fmt.Errorf("query tags for book: %w", err)
	}
	defer rows.Close()

	var tags []*domain.Tag
	for rows.Next() {
		t, err := scanTag(rows)
		if err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tags = append(tags, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tags, nil
}

// GetTagsForBookIDs returns tags for multiple books in a single batch query.
// The returned map is keyed by book ID.
func (s *Store) GetTagsForBookIDs(ctx context.Context, bookIDs []string) (map[string][]*domain.Tag, error) {
	result := make(map[string][]*domain.Tag, len(bookIDs))
	if len(bookIDs) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(bookIDs))
	args := make([]any, len(bookIDs))
	for i, bid := range bookIDs {
		placeholders[i] = "?"
		args[i] = bid
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT bt.book_id, t.id, t.slug, t.created_at, t.updated_at
		FROM book_tags bt
		INNER JOIN tags t ON t.id = bt.tag_id
		WHERE bt.book_id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY t.slug`, args...)
	if err != nil {
		return nil, fmt.Errorf("query tags for book ids: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			bookID    string
			t         domain.Tag
			createdAt string
			updatedAt string
		)
		if err := rows.Scan(&bookID, &t.ID, &t.Slug, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan tag row: %w", err)
		}

		t.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		t.UpdatedAt, err = parseTime(updatedAt)
		if err != nil {
			return nil, err
		}

		result[bookID] = append(result[bookID], &t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// GetTagIDsForBook returns the tag IDs associated with a book.
func (s *Store) GetTagIDsForBook(ctx context.Context, bookID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT tag_id FROM book_tags WHERE book_id = ?`, bookID)
	if err != nil {
		return nil, fmt.Errorf("query tag ids for book: %w", err)
	}
	defer rows.Close()

	var tagIDs []string
	for rows.Next() {
		var tid string
		if err := rows.Scan(&tid); err != nil {
			return nil, fmt.Errorf("scan tag id: %w", err)
		}
		tagIDs = append(tagIDs, tid)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tagIDs, nil
}

// GetBookIDsForTag returns all book IDs associated with a tag.
func (s *Store) GetBookIDsForTag(ctx context.Context, tagID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT book_id FROM book_tags WHERE tag_id = ?`, tagID)
	if err != nil {
		return nil, fmt.Errorf("query book ids for tag: %w", err)
	}
	defer rows.Close()

	var bookIDs []string
	for rows.Next() {
		var bid string
		if err := rows.Scan(&bid); err != nil {
			return nil, fmt.Errorf("scan book id: %w", err)
		}
		bookIDs = append(bookIDs, bid)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return bookIDs, nil
}

// CleanupTagsForDeletedBook removes all book_tags associations for a deleted book.
func (s *Store) CleanupTagsForDeletedBook(ctx context.Context, bookID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM book_tags WHERE book_id = ?`, bookID)
	return err
}

// RecalculateTagBookCount updates a tag's book_count based on actual book_tags rows.
// Since the tags table does not have a book_count column, this is a no-op
// that returns nil. The BookCount field on domain.Tag is computed at read time.
func (s *Store) RecalculateTagBookCount(ctx context.Context, tagID string) error {
	// The tags table schema does not have a book_count column.
	// BookCount is computed dynamically when needed.
	// This method exists to satisfy the interface.
	return nil
}

// GetTagSlugsForBook returns the slugs of all tags associated with a book.
func (s *Store) GetTagSlugsForBook(ctx context.Context, bookID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.slug
		FROM tags t
		INNER JOIN book_tags bt ON bt.tag_id = t.id
		WHERE bt.book_id = ?
		ORDER BY t.slug`, bookID)
	if err != nil {
		return nil, fmt.Errorf("query tag slugs for book: %w", err)
	}
	defer rows.Close()

	var slugs []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, fmt.Errorf("scan tag slug: %w", err)
		}
		slugs = append(slugs, slug)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return slugs, nil
}
