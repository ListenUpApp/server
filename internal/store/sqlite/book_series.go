package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/listenupapp/listenup-server/internal/domain"
)

// SetBookSeries replaces all series links for a book in a single transaction.
// It deletes existing rows and inserts the new set.
func (s *Store) SetBookSeries(ctx context.Context, bookID string, series []domain.BookSeries) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Delete existing series links for this book.
	if _, err := tx.ExecContext(ctx, `DELETE FROM book_series WHERE book_id = ?`, bookID); err != nil {
		return fmt.Errorf("delete book_series: %w", err)
	}

	// Insert new series links.
	for _, bs := range series {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO book_series (book_id, series_id, sequence)
			VALUES (?, ?, ?)`,
			bookID,
			bs.SeriesID,
			nullString(bs.Sequence),
		)
		if err != nil {
			return fmt.Errorf("insert book_series: %w", err)
		}
	}

	return tx.Commit()
}

// GetBookSeries returns all series linked to a book.
func (s *Store) GetBookSeries(ctx context.Context, bookID string) ([]domain.BookSeries, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT series_id, sequence
		FROM book_series
		WHERE book_id = ?`, bookID)
	if err != nil {
		return nil, fmt.Errorf("query book_series: %w", err)
	}
	defer rows.Close()

	var series []domain.BookSeries
	for rows.Next() {
		var (
			bs  domain.BookSeries
			seq sql.NullString
		)

		if err := rows.Scan(&bs.SeriesID, &seq); err != nil {
			return nil, fmt.Errorf("scan book_series: %w", err)
		}

		if seq.Valid {
			bs.Sequence = seq.String
		}

		series = append(series, bs)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	return series, nil
}
