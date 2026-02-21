package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// GetLibraryCheckpoint returns the most recent updated_at timestamp across
// all non-deleted books, contributors, and series. If no data exists, it
// returns a zero time.Time.
func (s *Store) GetLibraryCheckpoint(ctx context.Context) (time.Time, error) {
	var maxUpdated sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT MAX(updated_at) FROM (
			SELECT updated_at FROM books WHERE deleted_at IS NULL
			UNION ALL
			SELECT updated_at FROM contributors WHERE deleted_at IS NULL
			UNION ALL
			SELECT updated_at FROM series WHERE deleted_at IS NULL
		)`).Scan(&maxUpdated)
	if err != nil {
		return time.Time{}, fmt.Errorf("query library checkpoint: %w", err)
	}

	if !maxUpdated.Valid || maxUpdated.String == "" {
		return time.Time{}, nil
	}

	t, err := parseTime(maxUpdated.String)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse checkpoint time: %w", err)
	}

	return t, nil
}
