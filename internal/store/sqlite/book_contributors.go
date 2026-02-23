package sqlite

import (
	"strings"
	"context"
	"database/sql"
	"encoding/json/v2"
	"fmt"

	"github.com/listenupapp/listenup-server/internal/domain"
)

// setBookContributorsInternal replaces all contributors for a book in a single transaction.
// It deletes existing rows and inserts the new set.
func (s *Store) setBookContributorsInternal(ctx context.Context, bookID string, contributors []domain.BookContributor) error {
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

// setBookContributorsTx replaces all contributors for a book within an existing transaction.
func setBookContributorsTx(ctx context.Context, tx *sql.Tx, bookID string, contributors []domain.BookContributor) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM book_contributors WHERE book_id = ?`, bookID); err != nil {
		return fmt.Errorf("delete book_contributors: %w", err)
	}
	for _, c := range contributors {
		rolesJSON, err := json.Marshal(c.Roles)
		if err != nil {
			return fmt.Errorf("marshal roles: %w", err)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO book_contributors (book_id, contributor_id, roles, credited_as) VALUES (?, ?, ?, ?)`,
			bookID, c.ContributorID, string(rolesJSON), nullString(c.CreditedAs),
		)
		if err != nil {
			return fmt.Errorf("insert book_contributor: %w", err)
		}
	}
	return nil
}

// GetBookContributors returns all non-deleted contributors linked to a book.
func (s *Store) GetBookContributors(ctx context.Context, bookID string) ([]domain.BookContributor, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT bc.contributor_id, bc.roles, bc.credited_as
		FROM book_contributors bc
		JOIN contributors c ON c.id = bc.contributor_id
		WHERE bc.book_id = ? AND c.deleted_at IS NULL`, bookID)
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

// GetBookIDsByContributor returns the book IDs linked to a specific contributor.
func (s *Store) GetBookIDsByContributor(ctx context.Context, contributorID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT book_id FROM book_contributors WHERE contributor_id = ?`, contributorID)
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
	return bookIDs, rows.Err()
}

// GetContributorBookIDMap returns a map of contributor ID → list of book IDs.
func (s *Store) GetContributorBookIDMap(ctx context.Context) (map[string][]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT contributor_id, book_id FROM book_contributors`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]string)
	for rows.Next() {
		var contribID, bookID string
		if err := rows.Scan(&contribID, &bookID); err != nil {
			return nil, err
		}
		result[contribID] = append(result[contribID], bookID)
	}
	return result, rows.Err()
}

// GetContributorsByBookIDs returns book contributors for multiple books in one query.
// Returns a map of bookID → []domain.BookContributor.
func (s *Store) GetContributorsByBookIDs(ctx context.Context, bookIDs []string) (map[string][]domain.BookContributor, error) {
	if len(bookIDs) == 0 {
		return map[string][]domain.BookContributor{}, nil
	}

	placeholders := make([]string, len(bookIDs))
	args := make([]any, len(bookIDs))
	for i, id := range bookIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `
		SELECT bc.book_id, bc.contributor_id, bc.roles, bc.credited_as
		FROM book_contributors bc
		JOIN contributors c ON c.id = bc.contributor_id
		WHERE bc.book_id IN (` + strings.Join(placeholders, ",") + `) AND c.deleted_at IS NULL`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query book_contributors batch: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]domain.BookContributor)
	for rows.Next() {
		var (
			bookID    string
			bc        domain.BookContributor
			rolesJSON string
			credited  sql.NullString
		)
		if err := rows.Scan(&bookID, &bc.ContributorID, &rolesJSON, &credited); err != nil {
			return nil, fmt.Errorf("scan book_contributor batch: %w", err)
		}
		if err := json.Unmarshal([]byte(rolesJSON), &bc.Roles); err != nil {
			return nil, fmt.Errorf("unmarshal roles: %w", err)
		}
		if credited.Valid {
			bc.CreditedAs = credited.String
		}
		result[bookID] = append(result[bookID], bc)
	}
	return result, rows.Err()
}
