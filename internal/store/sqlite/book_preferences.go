package sqlite

import (
	"context"
	"database/sql"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// scanBookPreferences scans a sql.Row (or sql.Rows via its Scan method) into a domain.BookPreferences.
func scanBookPreferences(scanner interface{ Scan(dest ...any) error }) (*domain.BookPreferences, error) {
	var bp domain.BookPreferences

	var (
		playbackSpeed             sql.NullFloat64
		skipForwardSec            sql.NullInt64
		hideFromContinueListening int
		updatedAt                 string
	)

	err := scanner.Scan(
		&bp.UserID,
		&bp.BookID,
		&playbackSpeed,
		&skipForwardSec,
		&hideFromContinueListening,
		&updatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Nullable fields.
	if playbackSpeed.Valid {
		v := float32(playbackSpeed.Float64)
		bp.PlaybackSpeed = &v
	}
	if skipForwardSec.Valid {
		v := int(skipForwardSec.Int64)
		bp.SkipForwardSec = &v
	}

	// Boolean fields.
	bp.HideFromContinueListening = hideFromContinueListening != 0

	// Parse timestamp.
	bp.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}

	return &bp, nil
}

// GetBookPreferences retrieves book preferences for a user+book.
// Returns store.ErrBookPreferencesNotFound if the preferences do not exist.
func (s *Store) GetBookPreferences(ctx context.Context, userID, bookID string) (*domain.BookPreferences, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT user_id, book_id, playback_speed, skip_forward_sec,
		       hide_from_continue_listening, updated_at
		FROM book_preferences
		WHERE user_id = ? AND book_id = ?`,
		userID, bookID)

	prefs, err := scanBookPreferences(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrBookPreferencesNotFound
	}
	if err != nil {
		return nil, err
	}
	return prefs, nil
}

// UpsertBookPreferences creates or replaces book preferences for a user+book.
func (s *Store) UpsertBookPreferences(ctx context.Context, prefs *domain.BookPreferences) error {
	// Convert nullable Go types to SQL-compatible values.
	var playbackSpeed interface{}
	if prefs.PlaybackSpeed != nil {
		playbackSpeed = *prefs.PlaybackSpeed
	}

	var skipForwardSec interface{}
	if prefs.SkipForwardSec != nil {
		skipForwardSec = *prefs.SkipForwardSec
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO book_preferences (
			user_id, book_id, playback_speed, skip_forward_sec,
			hide_from_continue_listening, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)`,
		prefs.UserID,
		prefs.BookID,
		playbackSpeed,
		skipForwardSec,
		boolToInt(prefs.HideFromContinueListening),
		formatTime(prefs.UpdatedAt),
	)
	return err
}

// DeleteBookPreferences removes book preferences for a user+book.
// This operation is idempotent.
func (s *Store) DeleteBookPreferences(ctx context.Context, userID, bookID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM book_preferences WHERE user_id = ? AND book_id = ?`,
		userID, bookID)
	return err
}

// GetAllBookPreferences retrieves all book preferences for a user.
func (s *Store) GetAllBookPreferences(ctx context.Context, userID string) ([]*domain.BookPreferences, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, book_id, playback_speed, skip_forward_sec,
		       hide_from_continue_listening, updated_at
		FROM book_preferences
		WHERE user_id = ?
		ORDER BY updated_at DESC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prefs []*domain.BookPreferences
	for rows.Next() {
		bp, err := scanBookPreferences(rows)
		if err != nil {
			return nil, err
		}
		prefs = append(prefs, bp)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return prefs, nil
}
