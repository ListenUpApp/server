package sqlite

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"fmt"

	"github.com/listenupapp/listenup-server/internal/domain"
)

// SetBookContributors replaces all contributors for a book in a single transaction.
// It deletes existing rows and inserts the new set.
func (s *Store) SetBookContributors(ctx context.Context, bookID string, contributors []domain.BookContributor) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Delete existing contributors for this book.
	if _, err := tx.ExecContext(ctx, `DELETE FROM book_contributors WHERE book_id = ?`, bookID); err != nil {
		return fmt.Errorf("delete book_contributors: %w", err)
	}

	// Insert new contributors.
	for _, c := range contributors {
		rolesJSON, err := json.Marshal(c.Roles)
		if err != nil {
			return fmt.Errorf("marshal roles: %w", err)
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO book_contributors (book_id, contributor_id, roles, credited_as)
			VALUES (?, ?, ?, ?)`,
			bookID,
			c.ContributorID,
			string(rolesJSON),
			nullString(c.CreditedAs),
		)
		if err != nil {
			return fmt.Errorf("insert book_contributor: %w", err)
		}
	}

	return tx.Commit()
}

// GetBookContributors returns all contributors linked to a book.
func (s *Store) GetBookContributors(ctx context.Context, bookID string) ([]domain.BookContributor, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT contributor_id, roles, credited_as
		FROM book_contributors
		WHERE book_id = ?`, bookID)
	if err != nil {
		return nil, fmt.Errorf("query book_contributors: %w", err)
	}
	defer rows.Close()

	var contributors []domain.BookContributor
	for rows.Next() {
		var (
			bc        domain.BookContributor
			rolesJSON string
			credited  sql.NullString
		)

		if err := rows.Scan(&bc.ContributorID, &rolesJSON, &credited); err != nil {
			return nil, fmt.Errorf("scan book_contributor: %w", err)
		}

		if err := json.Unmarshal([]byte(rolesJSON), &bc.Roles); err != nil {
			return nil, fmt.Errorf("unmarshal roles: %w", err)
		}

		if credited.Valid {
			bc.CreditedAs = credited.String
		}

		contributors = append(contributors, bc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	return contributors, nil
}
