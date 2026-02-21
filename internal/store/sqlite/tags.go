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

// GetTagByID retrieves a tag by its ID.
// Returns store.ErrNotFound if the tag does not exist.
func (s *Store) GetTagByID(ctx context.Context, tagID string) (*domain.Tag, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+tagColumns+` FROM tags WHERE id = ?`, tagID)

	t, err := scanTag(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// GetTagBySlug retrieves a tag by its slug.
// Returns store.ErrNotFound if the tag does not exist.
func (s *Store) GetTagBySlug(ctx context.Context, slug string) (*domain.Tag, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+tagColumns+` FROM tags WHERE slug = ?`, slug)

	t, err := scanTag(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// ListTags returns all tags ordered by slug.
func (s *Store) ListTags(ctx context.Context) ([]*domain.Tag, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+tagColumns+` FROM tags ORDER BY slug ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []*domain.Tag
	for rows.Next() {
		t, err := scanTag(rows)
		if err != nil {
			return nil, err
		}
		tags = append(tags, t)
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
