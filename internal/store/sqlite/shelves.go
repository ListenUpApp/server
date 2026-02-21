package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// shelfColumns is the ordered list of columns selected in shelf queries.
// Must match the scan order in scanShelf.
const shelfColumns = `id, created_at, updated_at, owner_id, name, description, color, icon`

// scanShelf scans a sql.Row (or sql.Rows via its Scan method) into a domain.Lens.
func scanShelf(scanner interface{ Scan(dest ...any) error }) (*domain.Lens, error) {
	var l domain.Lens

	var (
		createdAt   string
		updatedAt   string
		description sql.NullString
		color       sql.NullString
		icon        sql.NullString
	)

	err := scanner.Scan(
		&l.ID,
		&createdAt,
		&updatedAt,
		&l.OwnerID,
		&l.Name,
		&description,
		&color,
		&icon,
	)
	if err != nil {
		return nil, err
	}

	// Parse timestamps.
	l.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	l.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}

	// Optional string fields.
	if description.Valid {
		l.Description = description.String
	}
	if color.Valid {
		l.Color = color.String
	}
	if icon.Valid {
		l.Icon = icon.String
	}

	return &l, nil
}

// loadShelfBookIDs loads the ordered book IDs for a shelf from shelf_books.
func (s *Store) loadShelfBookIDs(ctx context.Context, shelfID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT book_id FROM shelf_books WHERE shelf_id = ? ORDER BY sort_order`, shelfID)
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

// CreateLens inserts a new shelf and its book associations.
// Returns store.ErrAlreadyExists on duplicate ID.
func (s *Store) CreateLens(ctx context.Context, lens *domain.Lens) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO shelves (
			id, created_at, updated_at, owner_id, name, description, color, icon
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		lens.ID,
		formatTime(lens.CreatedAt),
		formatTime(lens.UpdatedAt),
		lens.OwnerID,
		lens.Name,
		nullString(lens.Description),
		nullString(lens.Color),
		nullString(lens.Icon),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return store.ErrAlreadyExists
		}
		return err
	}

	// Insert shelf_books for each BookID with sort_order based on index.
	for i, bookID := range lens.BookIDs {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO shelf_books (shelf_id, book_id, sort_order)
			VALUES (?, ?, ?)`,
			lens.ID, bookID, i,
		)
		if err != nil {
			return fmt.Errorf("insert shelf_book %s: %w", bookID, err)
		}
	}

	return tx.Commit()
}

// GetLens retrieves a lens by its shelf ID, including ordered BookIDs.
// Returns store.ErrNotFound if the shelf does not exist.
func (s *Store) GetLens(ctx context.Context, id string) (*domain.Lens, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+shelfColumns+` FROM shelves WHERE id = ?`, id)

	l, err := scanShelf(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	l.BookIDs, err = s.loadShelfBookIDs(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load shelf book ids: %w", err)
	}

	return l, nil
}

// UpdateLens updates a shelf row and replaces its book associations in a transaction.
// Returns store.ErrNotFound if the shelf does not exist.
func (s *Store) UpdateLens(ctx context.Context, lens *domain.Lens) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		UPDATE shelves SET
			created_at = ?,
			updated_at = ?,
			owner_id = ?,
			name = ?,
			description = ?,
			color = ?,
			icon = ?
		WHERE id = ?`,
		formatTime(lens.CreatedAt),
		formatTime(lens.UpdatedAt),
		lens.OwnerID,
		lens.Name,
		nullString(lens.Description),
		nullString(lens.Color),
		nullString(lens.Icon),
		lens.ID,
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

	// Replace shelf_books: delete existing, then re-insert.
	if _, err := tx.ExecContext(ctx, `DELETE FROM shelf_books WHERE shelf_id = ?`, lens.ID); err != nil {
		return err
	}

	for i, bookID := range lens.BookIDs {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO shelf_books (shelf_id, book_id, sort_order)
			VALUES (?, ?, ?)`,
			lens.ID, bookID, i,
		)
		if err != nil {
			return fmt.Errorf("insert shelf_book %s: %w", bookID, err)
		}
	}

	return tx.Commit()
}

// DeleteLens performs a hard delete on a shelf.
// The ON DELETE CASCADE on shelf_books ensures book associations are removed.
// Returns store.ErrNotFound if the shelf does not exist.
func (s *Store) DeleteLens(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM shelves WHERE id = ?`, id)
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

// ListLensesByOwner returns all lenses owned by a user, ordered by creation time.
// BookIDs are loaded for each lens.
func (s *Store) ListLensesByOwner(ctx context.Context, ownerID string) ([]*domain.Lens, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+shelfColumns+` FROM shelves WHERE owner_id = ? ORDER BY created_at`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lenses []*domain.Lens
	for rows.Next() {
		l, err := scanShelf(rows)
		if err != nil {
			return nil, err
		}
		lenses = append(lenses, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Load BookIDs for each lens.
	for _, l := range lenses {
		l.BookIDs, err = s.loadShelfBookIDs(ctx, l.ID)
		if err != nil {
			return nil, fmt.Errorf("load shelf book ids for %s: %w", l.ID, err)
		}
	}

	return lenses, nil
}

// AddBookToLens appends a book to a shelf's book list.
// Uses INSERT OR IGNORE for idempotency (no error if already present).
func (s *Store) AddBookToLens(ctx context.Context, lensID, bookID string) error {
	// Get the current max sort_order for this shelf.
	var maxOrder sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT MAX(sort_order) FROM shelf_books WHERE shelf_id = ?`, lensID).Scan(&maxOrder)
	if err != nil {
		return err
	}

	nextOrder := 0
	if maxOrder.Valid {
		nextOrder = int(maxOrder.Int64) + 1
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO shelf_books (shelf_id, book_id, sort_order)
		VALUES (?, ?, ?)`,
		lensID, bookID, nextOrder,
	)
	return err
}

// RemoveBookFromLens removes a book from a shelf's book list.
func (s *Store) RemoveBookFromLens(ctx context.Context, lensID, bookID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM shelf_books WHERE shelf_id = ? AND book_id = ?`,
		lensID, bookID,
	)
	return err
}
