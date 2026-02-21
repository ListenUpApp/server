package sqlite

import (
	"context"
	"database/sql"
	"strings"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// listeningEventColumns is the ordered list of columns selected in listening event queries.
// Must match the scan order in scanListeningEvent.
const listeningEventColumns = `id, user_id, book_id,
	start_position_ms, end_position_ms, started_at, ended_at,
	playback_speed, device_id, device_name, source, duration_ms, created_at`

// scanListeningEvent scans a sql.Row (or sql.Rows via its Scan method) into a domain.ListeningEvent.
func scanListeningEvent(scanner interface{ Scan(dest ...any) error }) (*domain.ListeningEvent, error) {
	var e domain.ListeningEvent

	var (
		startedAt  string
		endedAt    string
		createdAt  string
		deviceName sql.NullString
	)

	err := scanner.Scan(
		&e.ID,
		&e.UserID,
		&e.BookID,
		&e.StartPositionMs,
		&e.EndPositionMs,
		&startedAt,
		&endedAt,
		&e.PlaybackSpeed,
		&e.DeviceID,
		&deviceName,
		&e.Source,
		&e.DurationMs,
		&createdAt,
	)
	if err != nil {
		return nil, err
	}

	// Parse timestamps.
	e.StartedAt, err = parseTime(startedAt)
	if err != nil {
		return nil, err
	}
	e.EndedAt, err = parseTime(endedAt)
	if err != nil {
		return nil, err
	}
	e.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}

	// Optional string fields.
	if deviceName.Valid {
		e.DeviceName = deviceName.String
	}

	return &e, nil
}

// playbackStateColumns is the ordered list of columns selected in playback state queries.
// Must match the scan order in scanPlaybackState.
const playbackStateColumns = `user_id, book_id, current_position_ms, is_finished, finished_at,
	started_at, last_played_at, total_listen_time_ms, updated_at`

// scanPlaybackState scans a sql.Row (or sql.Rows via its Scan method) into a domain.PlaybackState.
func scanPlaybackState(scanner interface{ Scan(dest ...any) error }) (*domain.PlaybackState, error) {
	var ps domain.PlaybackState

	var (
		isFinished int
		finishedAt sql.NullString
		startedAt  string
		lastPlayed string
		updatedAt  string
	)

	err := scanner.Scan(
		&ps.UserID,
		&ps.BookID,
		&ps.CurrentPositionMs,
		&isFinished,
		&finishedAt,
		&startedAt,
		&lastPlayed,
		&ps.TotalListenTimeMs,
		&updatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Boolean fields.
	ps.IsFinished = isFinished != 0

	// Parse nullable timestamp.
	ps.FinishedAt, err = parseNullableTime(finishedAt)
	if err != nil {
		return nil, err
	}

	// Parse timestamps.
	ps.StartedAt, err = parseTime(startedAt)
	if err != nil {
		return nil, err
	}
	ps.LastPlayedAt, err = parseTime(lastPlayed)
	if err != nil {
		return nil, err
	}
	ps.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}

	return &ps, nil
}

// CreateListeningEvent inserts a new listening event into the database.
// Returns store.ErrAlreadyExists if the event ID already exists.
func (s *Store) CreateListeningEvent(ctx context.Context, event *domain.ListeningEvent) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO listening_events (
			id, user_id, book_id,
			start_position_ms, end_position_ms, started_at, ended_at,
			playback_speed, device_id, device_name, source, duration_ms, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.ID,
		event.UserID,
		event.BookID,
		event.StartPositionMs,
		event.EndPositionMs,
		formatTime(event.StartedAt),
		formatTime(event.EndedAt),
		event.PlaybackSpeed,
		event.DeviceID,
		nullString(event.DeviceName),
		event.Source,
		event.DurationMs,
		formatTime(event.CreatedAt),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return store.ErrAlreadyExists
		}
		return err
	}
	return nil
}

// GetListeningEvent retrieves a listening event by ID.
// Returns store.ErrNotFound if the event does not exist.
func (s *Store) GetListeningEvent(ctx context.Context, id string) (*domain.ListeningEvent, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+listeningEventColumns+` FROM listening_events WHERE id = ?`, id)

	event, err := scanListeningEvent(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return event, nil
}

// GetListeningEventsForBook retrieves all listening events for a book, ordered by ended_at descending.
func (s *Store) GetListeningEventsForBook(ctx context.Context, bookID string) ([]*domain.ListeningEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+listeningEventColumns+` FROM listening_events WHERE book_id = ? ORDER BY ended_at DESC`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*domain.ListeningEvent
	for rows.Next() {
		event, err := scanListeningEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

// GetListeningEvents retrieves all listening events for a user, ordered by ended_at descending.
func (s *Store) GetListeningEvents(ctx context.Context, userID string) ([]*domain.ListeningEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+listeningEventColumns+` FROM listening_events WHERE user_id = ? ORDER BY ended_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*domain.ListeningEvent
	for rows.Next() {
		event, err := scanListeningEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

// UpsertPlaybackState creates or replaces playback state for a user+book.
func (s *Store) UpsertPlaybackState(ctx context.Context, state *domain.PlaybackState) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO playback_state (
			user_id, book_id, current_position_ms, is_finished, finished_at,
			started_at, last_played_at, total_listen_time_ms, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		state.UserID,
		state.BookID,
		state.CurrentPositionMs,
		boolToInt(state.IsFinished),
		nullTimeString(state.FinishedAt),
		formatTime(state.StartedAt),
		formatTime(state.LastPlayedAt),
		state.TotalListenTimeMs,
		formatTime(state.UpdatedAt),
	)
	return err
}

// GetPlaybackState retrieves playback state for a user+book.
// Returns store.ErrNotFound if the state does not exist.
func (s *Store) GetPlaybackState(ctx context.Context, userID, bookID string) (*domain.PlaybackState, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+playbackStateColumns+` FROM playback_state WHERE user_id = ? AND book_id = ?`,
		userID, bookID)

	state, err := scanPlaybackState(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return state, nil
}

// GetContinueListening returns in-progress, non-hidden books for a user,
// ordered by last_played_at descending with a limit.
func (s *Store) GetContinueListening(ctx context.Context, userID string, limit int) ([]*domain.PlaybackState, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ps.user_id, ps.book_id, ps.current_position_ms, ps.is_finished, ps.finished_at,
		       ps.started_at, ps.last_played_at, ps.total_listen_time_ms, ps.updated_at
		FROM playback_state ps
		LEFT JOIN book_preferences bp ON bp.user_id = ps.user_id AND bp.book_id = ps.book_id
		WHERE ps.user_id = ?
		  AND ps.is_finished = 0
		  AND ps.current_position_ms > 0
		  AND (bp.hide_from_continue_listening IS NULL OR bp.hide_from_continue_listening = 0)
		ORDER BY ps.last_played_at DESC
		LIMIT ?`,
		userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var states []*domain.PlaybackState
	for rows.Next() {
		state, err := scanPlaybackState(rows)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return states, nil
}

// GetTotalListenTime returns the total listening time in milliseconds for a user
// by summing the duration_ms of all their listening events.
// Returns 0 if the user has no listening events.
func (s *Store) GetTotalListenTime(ctx context.Context, userID string) (int64, error) {
	var total sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT SUM(duration_ms) FROM listening_events WHERE user_id = ?`, userID).Scan(&total)
	if err != nil {
		return 0, err
	}
	if !total.Valid {
		return 0, nil
	}
	return total.Int64, nil
}
