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

// genreColumns is the ordered list of columns selected in genre queries.
// Must match the scan order in scanGenre.
const genreColumns = `id, created_at, updated_at, deleted_at, name, slug, description,
	parent_id, path, depth, sort_order, color, icon, is_system`

// scanGenre scans a sql.Row (or sql.Rows via its Scan method) into a domain.Genre.
func scanGenre(scanner interface{ Scan(dest ...any) error }) (*domain.Genre, error) {
	var g domain.Genre

	var (
		createdAt   string
		updatedAt   string
		deletedAt   sql.NullString
		description sql.NullString
		parentID    sql.NullString
		color       sql.NullString
		icon        sql.NullString
		isSystem    int
	)

	err := scanner.Scan(
		&g.ID,
		&createdAt,
		&updatedAt,
		&deletedAt,
		&g.Name,
		&g.Slug,
		&description,
		&parentID,
		&g.Path,
		&g.Depth,
		&g.SortOrder,
		&color,
		&icon,
		&isSystem,
	)
	if err != nil {
		return nil, err
	}

	// Parse timestamps.
	g.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	g.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	g.DeletedAt, err = parseNullableTime(deletedAt)
	if err != nil {
		return nil, err
	}

	// Optional strings.
	if description.Valid {
		g.Description = description.String
	}
	if parentID.Valid {
		g.ParentID = parentID.String
	}
	if color.Valid {
		g.Color = color.String
	}
	if icon.Valid {
		g.Icon = icon.String
	}

	// Boolean fields.
	g.IsSystem = isSystem != 0

	return &g, nil
}

// CreateGenre inserts a new genre into the database.
// Returns store.ErrAlreadyExists if the genre ID or slug already exists.
func (s *Store) CreateGenre(ctx context.Context, g *domain.Genre) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO genres (
			id, created_at, updated_at, deleted_at, name, slug, description,
			parent_id, path, depth, sort_order, color, icon, is_system
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		g.ID,
		formatTime(g.CreatedAt),
		formatTime(g.UpdatedAt),
		nullTimeString(g.DeletedAt),
		g.Name,
		g.Slug,
		nullString(g.Description),
		nullString(g.ParentID),
		g.Path,
		g.Depth,
		g.SortOrder,
		nullString(g.Color),
		nullString(g.Icon),
		boolToInt(g.IsSystem),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return store.ErrAlreadyExists
		}
		return err
	}
	return nil
}

// GetGenre retrieves a genre by ID, excluding soft-deleted records.
// Returns store.ErrNotFound if the genre does not exist.
func (s *Store) GetGenre(ctx context.Context, id string) (*domain.Genre, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+genreColumns+` FROM genres WHERE id = ? AND deleted_at IS NULL`, id)

	g, err := scanGenre(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return g, nil
}

// GetGenreBySlug retrieves a genre by slug, excluding soft-deleted records.
// Returns store.ErrNotFound if the genre does not exist.
func (s *Store) GetGenreBySlug(ctx context.Context, slug string) (*domain.Genre, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+genreColumns+` FROM genres WHERE slug = ? AND deleted_at IS NULL`, slug)

	g, err := scanGenre(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return g, nil
}

// GetOrCreateGenreBySlug retrieves an existing genre by slug or creates a new one.
// When creating, it generates an ID, computes path/depth from the parent, and initializes timestamps.
func (s *Store) GetOrCreateGenreBySlug(ctx context.Context, slug, name, parentID string) (*domain.Genre, error) {
	// Try to find existing.
	existing, err := s.GetGenreBySlug(ctx, slug)
	if err == nil {
		return existing, nil
	}
	if err != store.ErrNotFound {
		return nil, err
	}

	// Generate a new ID.
	genreID, err := id.Generate("genre")
	if err != nil {
		return nil, fmt.Errorf("generate genre ID: %w", err)
	}

	// Build path and depth from parent.
	var path string
	var depth int
	if parentID != "" {
		parent, err := s.GetGenre(ctx, parentID)
		if err != nil {
			return nil, fmt.Errorf("get parent genre: %w", err)
		}
		path = parent.Path + "/" + slug
		depth = parent.Depth + 1
	} else {
		path = "/" + slug
		depth = 0
	}

	g := &domain.Genre{
		Syncable: domain.Syncable{ID: genreID},
		Name:     name,
		Slug:     slug,
		ParentID: parentID,
		Path:     path,
		Depth:    depth,
	}
	g.InitTimestamps()

	if err := s.CreateGenre(ctx, g); err != nil {
		return nil, err
	}

	return g, nil
}

// ListGenres returns all non-deleted genres sorted by path.
func (s *Store) ListGenres(ctx context.Context) ([]*domain.Genre, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+genreColumns+` FROM genres WHERE deleted_at IS NULL ORDER BY path ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var genres []*domain.Genre
	for rows.Next() {
		g, err := scanGenre(rows)
		if err != nil {
			return nil, err
		}
		genres = append(genres, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return genres, nil
}

// UpdateGenre performs a full row update on an existing genre.
// Returns store.ErrNotFound if the genre does not exist or is soft-deleted.
func (s *Store) UpdateGenre(ctx context.Context, g *domain.Genre) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE genres SET
			created_at = ?,
			updated_at = ?,
			name = ?,
			slug = ?,
			description = ?,
			parent_id = ?,
			path = ?,
			depth = ?,
			sort_order = ?,
			color = ?,
			icon = ?,
			is_system = ?
		WHERE id = ? AND deleted_at IS NULL`,
		formatTime(g.CreatedAt),
		formatTime(g.UpdatedAt),
		g.Name,
		g.Slug,
		nullString(g.Description),
		nullString(g.ParentID),
		g.Path,
		g.Depth,
		g.SortOrder,
		nullString(g.Color),
		nullString(g.Icon),
		boolToInt(g.IsSystem),
		g.ID,
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

// DeleteGenre performs a soft delete by setting deleted_at and updated_at.
// Returns store.ErrNotFound if the genre does not exist or is already deleted.
func (s *Store) DeleteGenre(ctx context.Context, id string) error {
	now := formatTime(time.Now().UTC())

	result, err := s.db.ExecContext(ctx, `
		UPDATE genres SET deleted_at = ?, updated_at = ?
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

// SetBookGenres replaces all genre associations for a book in a single transaction.
// It deletes existing book_genres rows for the book, then inserts the new set.
func (s *Store) SetBookGenres(ctx context.Context, bookID string, genreIDs []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Delete existing genre links for this book.
	if _, err := tx.ExecContext(ctx, `DELETE FROM book_genres WHERE book_id = ?`, bookID); err != nil {
		return fmt.Errorf("delete book_genres: %w", err)
	}

	// Insert new genre links.
	for _, genreID := range genreIDs {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO book_genres (book_id, genre_id)
			VALUES (?, ?)`,
			bookID,
			genreID,
		)
		if err != nil {
			return fmt.Errorf("insert book_genres: %w", err)
		}
	}

	return tx.Commit()
}

// GetBookGenres returns all genre IDs associated with a book.
func (s *Store) GetBookGenres(ctx context.Context, bookID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT genre_id
		FROM book_genres
		WHERE book_id = ?`, bookID)
	if err != nil {
		return nil, fmt.Errorf("query book_genres: %w", err)
	}
	defer rows.Close()

	var genreIDs []string
	for rows.Next() {
		var genreID string
		if err := rows.Scan(&genreID); err != nil {
			return nil, fmt.Errorf("scan book_genres: %w", err)
		}
		genreIDs = append(genreIDs, genreID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	return genreIDs, nil
}
