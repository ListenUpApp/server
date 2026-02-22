package sqlite

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"fmt"
	"strings"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/genre"
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

// GetGenresByIDs retrieves multiple genres by their IDs.
// Missing or soft-deleted genres are silently skipped.
func (s *Store) GetGenresByIDs(ctx context.Context, ids []string) ([]*domain.Genre, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, gid := range ids {
		placeholders[i] = "?"
		args[i] = gid
	}

	query := `SELECT ` + genreColumns + ` FROM genres
		WHERE id IN (` + strings.Join(placeholders, ",") + `) AND deleted_at IS NULL`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query genres by ids: %w", err)
	}
	defer rows.Close()

	var genres []*domain.Genre
	for rows.Next() {
		g, err := scanGenre(rows)
		if err != nil {
			return nil, fmt.Errorf("scan genre: %w", err)
		}
		genres = append(genres, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	return genres, nil
}

// GetGenreChildren returns direct children of a genre, ordered by sort_order then name.
func (s *Store) GetGenreChildren(ctx context.Context, parentID string) ([]*domain.Genre, error) {
	var rows *sql.Rows
	var err error

	if parentID == "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+genreColumns+` FROM genres
			 WHERE parent_id IS NULL AND deleted_at IS NULL
			 ORDER BY sort_order, name`)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+genreColumns+` FROM genres
			 WHERE parent_id = ? AND deleted_at IS NULL
			 ORDER BY sort_order, name`, parentID)
	}
	if err != nil {
		return nil, fmt.Errorf("query genre children: %w", err)
	}
	defer rows.Close()

	var children []*domain.Genre
	for rows.Next() {
		g, err := scanGenre(rows)
		if err != nil {
			return nil, fmt.Errorf("scan genre child: %w", err)
		}
		children = append(children, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	return children, nil
}

// MoveGenre changes a genre's parent, updating path and depth for the genre and all descendants.
func (s *Store) MoveGenre(ctx context.Context, genreID, newParentID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Get the genre's current path and slug.
	var oldPath, slug string
	var oldDepth int
	err = tx.QueryRowContext(ctx,
		`SELECT path, depth, slug FROM genres WHERE id = ? AND deleted_at IS NULL`, genreID).
		Scan(&oldPath, &oldDepth, &slug)
	if err != nil {
		return fmt.Errorf("get genre: %w", err)
	}

	// Compute new path and depth from the new parent.
	var newPath string
	var newDepth int
	if newParentID == "" {
		newPath = "/" + slug
		newDepth = 0
	} else {
		var parentPath string
		var parentDepth int
		err = tx.QueryRowContext(ctx,
			`SELECT path, depth FROM genres WHERE id = ? AND deleted_at IS NULL`, newParentID).
			Scan(&parentPath, &parentDepth)
		if err != nil {
			return fmt.Errorf("get new parent: %w", err)
		}
		newPath = parentPath + "/" + slug
		newDepth = parentDepth + 1
	}

	now := formatTime(time.Now().UTC())
	depthDelta := newDepth - oldDepth

	// Update the moved genre.
	_, err = tx.ExecContext(ctx, `
		UPDATE genres SET parent_id = ?, path = ?, depth = ?, updated_at = ?
		WHERE id = ? AND deleted_at IS NULL`,
		nullString(newParentID), newPath, newDepth, now, genreID)
	if err != nil {
		return fmt.Errorf("update genre: %w", err)
	}

	// Update all descendants: replace old path prefix with new, adjust depth.
	_, err = tx.ExecContext(ctx, `
		UPDATE genres SET
			path = ? || SUBSTR(path, LENGTH(?) + 1),
			depth = depth + ?,
			updated_at = ?
		WHERE path LIKE ? || '/%' AND deleted_at IS NULL`,
		newPath, oldPath, depthDelta, now, oldPath)
	if err != nil {
		return fmt.Errorf("update descendants: %w", err)
	}

	return tx.Commit()
}

// MergeGenres merges the source genre into the target genre.
// All book_genres associations are moved from source to target, then the source is deleted.
func (s *Store) MergeGenres(ctx context.Context, sourceID, targetID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Move book_genres from source to target, ignoring duplicates.
	_, err = tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO book_genres (book_id, genre_id)
		SELECT book_id, ? FROM book_genres WHERE genre_id = ?`,
		targetID, sourceID)
	if err != nil {
		return fmt.Errorf("move book_genres: %w", err)
	}

	// Delete source book_genres.
	_, err = tx.ExecContext(ctx, `DELETE FROM book_genres WHERE genre_id = ?`, sourceID)
	if err != nil {
		return fmt.Errorf("delete source book_genres: %w", err)
	}

	// Reparent child genres from source to target before deleting source.
	// Look up source and target paths for path rewriting.
	var sourcePath, targetPath string
	err = tx.QueryRowContext(ctx, `SELECT path FROM genres WHERE id = ? AND deleted_at IS NULL`, sourceID).Scan(&sourcePath)
	if err != nil {
		return fmt.Errorf("get source path: %w", err)
	}
	err = tx.QueryRowContext(ctx, `SELECT path FROM genres WHERE id = ? AND deleted_at IS NULL`, targetID).Scan(&targetPath)
	if err != nil {
		return fmt.Errorf("get target path: %w", err)
	}

	now := formatTime(time.Now().UTC())

	// Update direct children: set parent_id and rewrite path prefix.
	_, err = tx.ExecContext(ctx, `
		UPDATE genres SET
			parent_id = ?,
			path = ? || SUBSTR(path, LENGTH(?) + 1),
			updated_at = ?
		WHERE parent_id = ? AND deleted_at IS NULL`,
		targetID, targetPath, sourcePath, now, sourceID)
	if err != nil {
		return fmt.Errorf("reparent child genres: %w", err)
	}

	// Soft-delete the source genre.
	_, err = tx.ExecContext(ctx, `
		UPDATE genres SET deleted_at = ?, updated_at = ?
		WHERE id = ? AND deleted_at IS NULL`,
		now, now, sourceID)
	if err != nil {
		return fmt.Errorf("soft-delete source genre: %w", err)
	}

	return tx.Commit()
}

// AddBookGenre adds a single genre association to a book.
// Uses INSERT OR IGNORE for idempotency.
func (s *Store) AddBookGenre(ctx context.Context, bookID, genreID string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO book_genres (book_id, genre_id)
		VALUES (?, ?)`, bookID, genreID)
	return err
}

// RemoveBookGenre removes a single genre association from a book.
func (s *Store) RemoveBookGenre(ctx context.Context, bookID, genreID string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM book_genres WHERE book_id = ? AND genre_id = ?`,
		bookID, genreID)
	return err
}

// GetGenreIDsForBook returns all genre IDs associated with a book.
func (s *Store) GetGenreIDsForBook(ctx context.Context, bookID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT genre_id FROM book_genres WHERE book_id = ?`, bookID)
	if err != nil {
		return nil, fmt.Errorf("query genre ids for book: %w", err)
	}
	defer rows.Close()

	var genreIDs []string
	for rows.Next() {
		var gid string
		if err := rows.Scan(&gid); err != nil {
			return nil, fmt.Errorf("scan genre id: %w", err)
		}
		genreIDs = append(genreIDs, gid)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return genreIDs, nil
}

// GetBookIDsForGenre returns all book IDs directly in a genre.
func (s *Store) GetBookIDsForGenre(ctx context.Context, genreID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT book_id FROM book_genres WHERE genre_id = ?`, genreID)
	if err != nil {
		return nil, fmt.Errorf("query book ids for genre: %w", err)
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

// GetBookIDsForGenreTree returns book IDs in a genre AND all its descendant genres.
// Uses the materialized path to find all descendants.
func (s *Store) GetBookIDsForGenreTree(ctx context.Context, genreID string) ([]string, error) {
	// First get the genre to know its path.
	g, err := s.GetGenre(ctx, genreID)
	if err != nil {
		return nil, err
	}

	// Find all genres whose path starts with this genre's path.
	rows, err := s.db.QueryContext(ctx, `
		SELECT bg.book_id FROM book_genres bg
		INNER JOIN genres g ON g.id = bg.genre_id
		WHERE (g.path = ? OR g.path LIKE ?)
		  AND g.deleted_at IS NULL`,
		g.Path, g.Path+"/%")
	if err != nil {
		return nil, fmt.Errorf("query book ids for genre tree: %w", err)
	}
	defer rows.Close()

	seen := make(map[string]bool)
	var bookIDs []string
	for rows.Next() {
		var bid string
		if err := rows.Scan(&bid); err != nil {
			return nil, fmt.Errorf("scan book id: %w", err)
		}
		if !seen[bid] {
			seen[bid] = true
			bookIDs = append(bookIDs, bid)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return bookIDs, nil
}

// CreateGenreAlias inserts a new genre alias mapping.
func (s *Store) CreateGenreAlias(ctx context.Context, alias *domain.GenreAlias) error {
	genreIDsJSON, err := json.Marshal(alias.GenreIDs)
	if err != nil {
		return fmt.Errorf("marshal genre ids: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO genre_aliases (id, raw_value, raw_lower, genre_ids, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		alias.ID,
		alias.RawValue,
		strings.ToLower(alias.RawValue),
		string(genreIDsJSON),
		alias.CreatedBy,
		formatTime(alias.CreatedAt),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return store.ErrAlreadyExists
		}
		return fmt.Errorf("insert genre alias: %w", err)
	}
	return nil
}

// GetGenreAliasByRaw looks up a genre alias by its raw metadata string.
// Returns store.ErrNotFound if no alias exists for the given raw string.
func (s *Store) GetGenreAliasByRaw(ctx context.Context, raw string) (*domain.GenreAlias, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, raw_value, genre_ids, created_by, created_at
		FROM genre_aliases WHERE raw_lower = ?`,
		strings.ToLower(raw))

	var (
		alias       domain.GenreAlias
		genreIDsStr string
		createdAt   string
	)

	err := row.Scan(&alias.ID, &alias.RawValue, &genreIDsStr, &alias.CreatedBy, &createdAt)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan genre alias: %w", err)
	}

	if err := json.Unmarshal([]byte(genreIDsStr), &alias.GenreIDs); err != nil {
		return nil, fmt.Errorf("unmarshal genre ids: %w", err)
	}

	alias.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}

	return &alias, nil
}

// TrackUnmappedGenre records a raw genre string that could not be mapped.
// If the raw string was already tracked, increments the book count and adds the book ID.
func (s *Store) TrackUnmappedGenre(ctx context.Context, raw string, bookID string) error {
	slug := genre.Slugify(raw)
	now := formatTime(time.Now().UTC())

	// Try to get existing.
	var existingBookIDs string
	var existingCount int
	err := s.db.QueryRowContext(ctx, `
		SELECT book_count, book_ids FROM unmapped_genres WHERE raw_slug = ?`, slug).
		Scan(&existingCount, &existingBookIDs)

	if err == sql.ErrNoRows {
		// New unmapped genre.
		bookIDsJSON, _ := json.Marshal([]string{bookID})
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO unmapped_genres (raw_value, raw_slug, book_count, first_seen, book_ids)
			VALUES (?, ?, 1, ?, ?)`,
			raw, slug, now, string(bookIDsJSON))
		return err
	}
	if err != nil {
		return fmt.Errorf("query unmapped genre: %w", err)
	}

	// Update existing.
	var bookIDs []string
	if err := json.Unmarshal([]byte(existingBookIDs), &bookIDs); err != nil {
		bookIDs = nil
	}
	if len(bookIDs) < 10 {
		bookIDs = append(bookIDs, bookID)
	}
	bookIDsJSON, _ := json.Marshal(bookIDs)

	_, err = s.db.ExecContext(ctx, `
		UPDATE unmapped_genres SET book_count = ?, book_ids = ? WHERE raw_slug = ?`,
		existingCount+1, string(bookIDsJSON), slug)
	return err
}

// ListUnmappedGenres returns all unmapped genre strings, ordered by book count descending.
func (s *Store) ListUnmappedGenres(ctx context.Context) ([]*domain.UnmappedGenre, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT raw_value, book_count, first_seen, book_ids
		FROM unmapped_genres
		ORDER BY book_count DESC`)
	if err != nil {
		return nil, fmt.Errorf("query unmapped genres: %w", err)
	}
	defer rows.Close()

	var unmapped []*domain.UnmappedGenre
	for rows.Next() {
		var (
			u          domain.UnmappedGenre
			firstSeen  string
			bookIDsStr string
		)

		if err := rows.Scan(&u.RawValue, &u.BookCount, &firstSeen, &bookIDsStr); err != nil {
			return nil, fmt.Errorf("scan unmapped genre: %w", err)
		}

		u.FirstSeen, err = parseTime(firstSeen)
		if err != nil {
			return nil, fmt.Errorf("parse first_seen: %w", err)
		}

		if err := json.Unmarshal([]byte(bookIDsStr), &u.BookIDs); err != nil {
			u.BookIDs = nil
		}

		unmapped = append(unmapped, &u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return unmapped, nil
}

// ResolveUnmappedGenre creates an alias for the raw genre string and removes it from unmapped.
func (s *Store) ResolveUnmappedGenre(ctx context.Context, raw string, genreIDs []string, userID string) error {
	aliasID, err := id.Generate("alias")
	if err != nil {
		return fmt.Errorf("generate alias id: %w", err)
	}

	alias := &domain.GenreAlias{
		ID:        aliasID,
		RawValue:  raw,
		GenreIDs:  genreIDs,
		CreatedBy: userID,
		CreatedAt: time.Now().UTC(),
	}

	if err := s.CreateGenreAlias(ctx, alias); err != nil {
		return fmt.Errorf("create genre alias: %w", err)
	}

	// Remove from unmapped.
	slug := genre.Slugify(raw)
	_, err = s.db.ExecContext(ctx, `DELETE FROM unmapped_genres WHERE raw_slug = ?`, slug)
	return err
}

// SeedDefaultGenres creates the default genre hierarchy if no genres exist.
func (s *Store) SeedDefaultGenres(ctx context.Context) error {
	// Check if already seeded.
	genres, err := s.ListGenres(ctx)
	if err != nil {
		return err
	}
	if len(genres) > 0 {
		return nil // Already seeded.
	}

	return s.seedGenreTree(ctx, genre.DefaultGenres, "", "")
}

// seedGenreTree recursively creates genres from a seed list.
func (s *Store) seedGenreTree(ctx context.Context, seeds []genre.GenreSeed, parentID, parentPath string) error {
	for i, seed := range seeds {
		genreID, err := id.Generate("genre")
		if err != nil {
			return fmt.Errorf("generate genre id: %w", err)
		}

		path := "/" + seed.Slug
		depth := 0
		if parentPath != "" {
			path = parentPath + "/" + seed.Slug
			depth = strings.Count(path, "/") - 1
		}

		g := &domain.Genre{
			Syncable:  domain.Syncable{ID: genreID},
			Name:      seed.Name,
			Slug:      seed.Slug,
			ParentID:  parentID,
			Path:      path,
			Depth:     depth,
			SortOrder: i,
			IsSystem:  true,
		}
		g.InitTimestamps()

		if err := s.CreateGenre(ctx, g); err != nil {
			return fmt.Errorf("create genre %s: %w", seed.Name, err)
		}

		if len(seed.Children) > 0 {
			if err := s.seedGenreTree(ctx, seed.Children, genreID, path); err != nil {
				return err
			}
		}
	}

	return nil
}
