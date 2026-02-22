package sqlite

import (
	"context"
	"database/sql"
	"iter"
	"strings"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// readingSessionColumns is the ordered list of columns selected in reading session queries.
// Must match the scan order in scanReadingSession.
const readingSessionColumns = `id, user_id, book_id, started_at, finished_at,
	is_completed, final_progress, listen_time_ms, created_at, updated_at`

// scanReadingSession scans a sql.Row (or sql.Rows via its Scan method) into a domain.BookReadingSession.
func scanReadingSession(scanner interface{ Scan(dest ...any) error }) (*domain.BookReadingSession, error) {
	var rs domain.BookReadingSession

	var (
		startedAt   string
		finishedAt  sql.NullString
		isCompleted int
		createdAt   string
		updatedAt   string
	)

	err := scanner.Scan(
		&rs.ID,
		&rs.UserID,
		&rs.BookID,
		&startedAt,
		&finishedAt,
		&isCompleted,
		&rs.FinalProgress,
		&rs.ListenTimeMs,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Parse timestamps.
	rs.StartedAt, err = parseTime(startedAt)
	if err != nil {
		return nil, err
	}
	rs.FinishedAt, err = parseNullableTime(finishedAt)
	if err != nil {
		return nil, err
	}
	rs.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	rs.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}

	// Boolean fields.
	rs.IsCompleted = isCompleted != 0

	return &rs, nil
}

// CreateReadingSession inserts a new reading session into the database.
// Returns store.ErrAlreadyExists if the session ID already exists.
func (s *Store) CreateReadingSession(ctx context.Context, session *domain.BookReadingSession) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO book_reading_sessions (
			id, user_id, book_id, started_at, finished_at,
			is_completed, final_progress, listen_time_ms, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID,
		session.UserID,
		session.BookID,
		formatTime(session.StartedAt),
		nullTimeString(session.FinishedAt),
		boolToInt(session.IsCompleted),
		session.FinalProgress,
		session.ListenTimeMs,
		formatTime(session.CreatedAt),
		formatTime(session.UpdatedAt),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return store.ErrAlreadyExists
		}
		return err
	}
	return nil
}

// UpdateReadingSession performs a full row update on an existing reading session.
// Returns store.ErrNotFound if the session does not exist.
func (s *Store) UpdateReadingSession(ctx context.Context, session *domain.BookReadingSession) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE book_reading_sessions SET
			user_id = ?,
			book_id = ?,
			started_at = ?,
			finished_at = ?,
			is_completed = ?,
			final_progress = ?,
			listen_time_ms = ?,
			created_at = ?,
			updated_at = ?
		WHERE id = ?`,
		session.UserID,
		session.BookID,
		formatTime(session.StartedAt),
		nullTimeString(session.FinishedAt),
		boolToInt(session.IsCompleted),
		session.FinalProgress,
		session.ListenTimeMs,
		formatTime(session.CreatedAt),
		formatTime(session.UpdatedAt),
		session.ID,
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

// GetReadingSessions returns all reading sessions for a user and book,
// ordered by started_at descending (most recent first).
func (s *Store) GetReadingSessions(ctx context.Context, userID, bookID string) ([]*domain.BookReadingSession, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+readingSessionColumns+` FROM book_reading_sessions
		WHERE user_id = ? AND book_id = ?
		ORDER BY started_at DESC`,
		userID, bookID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*domain.BookReadingSession
	for rows.Next() {
		rs, err := scanReadingSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, rs)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sessions, nil
}

// GetActiveReadingSession returns the most recent active (unfinished) reading session
// for a user and book. Returns nil, nil if no active session exists.
func (s *Store) GetActiveReadingSession(ctx context.Context, userID, bookID string) (*domain.BookReadingSession, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+readingSessionColumns+` FROM book_reading_sessions
		WHERE user_id = ? AND book_id = ? AND finished_at IS NULL
		ORDER BY started_at DESC
		LIMIT 1`,
		userID, bookID,
	)

	rs, err := scanReadingSession(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return rs, nil
}

// GetReadingSession retrieves a single reading session by ID.
// Returns store.ErrNotFound if the session does not exist.
func (s *Store) GetReadingSession(ctx context.Context, id string) (*domain.BookReadingSession, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+readingSessionColumns+` FROM book_reading_sessions WHERE id = ?`, id)

	rs, err := scanReadingSession(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return rs, nil
}

// DeleteReadingSession deletes a reading session by ID.
// This operation is idempotent.
func (s *Store) DeleteReadingSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM book_reading_sessions WHERE id = ?`, id)
	return err
}

// GetActiveSession is an alias for GetActiveReadingSession.
// Returns the most recent active (unfinished) reading session for a user and book.
func (s *Store) GetActiveSession(ctx context.Context, userID, bookID string) (*domain.BookReadingSession, error) {
	return s.GetActiveReadingSession(ctx, userID, bookID)
}

// GetUserReadingSessions returns all reading sessions for a user,
// ordered by started_at descending (most recent first).
// If limit > 0, at most limit sessions are returned.
func (s *Store) GetUserReadingSessions(ctx context.Context, userID string, limit int) ([]*domain.BookReadingSession, error) {
	query := `SELECT ` + readingSessionColumns + ` FROM book_reading_sessions
		WHERE user_id = ?
		ORDER BY started_at DESC`

	var args []any
	args = append(args, userID)

	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*domain.BookReadingSession
	for rows.Next() {
		rs, err := scanReadingSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, rs)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sessions, nil
}

// GetUserBookSessions is an alias for GetReadingSessions.
// Returns all reading sessions for a user and book.
func (s *Store) GetUserBookSessions(ctx context.Context, userID, bookID string) ([]*domain.BookReadingSession, error) {
	return s.GetReadingSessions(ctx, userID, bookID)
}

// GetBookSessions returns all reading sessions for a book (across all users),
// ordered by started_at descending.
func (s *Store) GetBookSessions(ctx context.Context, bookID string) ([]*domain.BookReadingSession, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+readingSessionColumns+` FROM book_reading_sessions
		WHERE book_id = ?
		ORDER BY started_at DESC`,
		bookID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*domain.BookReadingSession
	for rows.Next() {
		rs, err := scanReadingSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, rs)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sessions, nil
}

// GetAllActiveSessions returns all active (unfinished) reading sessions across all users.
func (s *Store) GetAllActiveSessions(ctx context.Context) ([]*domain.BookReadingSession, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+readingSessionColumns+` FROM book_reading_sessions
		WHERE finished_at IS NULL
		ORDER BY started_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*domain.BookReadingSession
	for rows.Next() {
		rs, err := scanReadingSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, rs)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sessions, nil
}

// CleanupStaleSessions marks reading sessions as finished if they've been active
// longer than maxAge. Returns the number of sessions cleaned up.
func (s *Store) CleanupStaleSessions(ctx context.Context, maxAge time.Duration) (int, error) {
	cutoff := time.Now().Add(-maxAge)
	result, err := s.db.ExecContext(ctx, `
		UPDATE book_reading_sessions
		SET finished_at = updated_at, updated_at = ?
		WHERE finished_at IS NULL AND updated_at < ?`,
		formatTime(time.Now()), formatTime(cutoff))
	if err != nil {
		return 0, err
	}
	n, err := result.RowsAffected()
	return int(n), err
}

// ListAllSessions returns an iterator over all reading sessions.
// This is useful for export/backup operations.
func (s *Store) ListAllSessions(ctx context.Context) iter.Seq2[*domain.BookReadingSession, error] {
	return func(yield func(*domain.BookReadingSession, error) bool) {
		rows, err := s.db.QueryContext(ctx,
			`SELECT `+readingSessionColumns+` FROM book_reading_sessions ORDER BY started_at DESC`)
		if err != nil {
			yield(nil, err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			if ctx.Err() != nil {
				yield(nil, ctx.Err())
				return
			}

			rs, err := scanReadingSession(rows)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			if !yield(rs, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(nil, err)
		}
	}
}
