package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
)

// GetUserStats retrieves pre-aggregated stats for a user.
// Returns nil, nil if no stats exist yet.
func (s *Store) GetUserStats(ctx context.Context, userID string) (*domain.UserStats, error) {
	var stats domain.UserStats
	var updatedAt string
	var lastListenedDate sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT user_id, total_listen_ms, books_finished, current_streak,
			longest_streak, last_listened_date, updated_at
		FROM user_stats WHERE user_id = ?`, userID).Scan(
		&stats.UserID,
		&stats.TotalListenTimeMs,
		&stats.TotalBooksFinished,
		&stats.CurrentStreakDays,
		&stats.LongestStreakDays,
		&lastListenedDate,
		&updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	stats.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	if lastListenedDate.Valid {
		stats.LastListenedDate = lastListenedDate.String
	}

	return &stats, nil
}

// GetAllUserStats retrieves stats for all users.
func (s *Store) GetAllUserStats(ctx context.Context) ([]*domain.UserStats, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, total_listen_ms, books_finished, current_streak,
			longest_streak, last_listened_date, updated_at
		FROM user_stats`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*domain.UserStats
	for rows.Next() {
		var stats domain.UserStats
		var updatedAt string
		var lastListenedDate sql.NullString

		if err := rows.Scan(
			&stats.UserID,
			&stats.TotalListenTimeMs,
			&stats.TotalBooksFinished,
			&stats.CurrentStreakDays,
			&stats.LongestStreakDays,
			&lastListenedDate,
			&updatedAt,
		); err != nil {
			return nil, err
		}

		stats.UpdatedAt, err = parseTime(updatedAt)
		if err != nil {
			return nil, err
		}
		if lastListenedDate.Valid {
			stats.LastListenedDate = lastListenedDate.String
		}

		results = append(results, &stats)
	}
	return results, rows.Err()
}

// EnsureUserStats creates a user_stats row if it doesn't exist.
func (s *Store) EnsureUserStats(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO user_stats (user_id, updated_at)
		VALUES (?, ?)`,
		userID, formatTime(time.Now().UTC()),
	)
	return err
}

// IncrementListenTime atomically increments the total listen time for a user.
func (s *Store) IncrementListenTime(ctx context.Context, userID string, deltaMs int64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_stats (user_id, total_listen_ms, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			total_listen_ms = total_listen_ms + excluded.total_listen_ms,
			updated_at = excluded.updated_at`,
		userID, deltaMs, formatTime(time.Now().UTC()),
	)
	return err
}

// IncrementBooksFinishedAtomic atomically increments the books finished count.
func (s *Store) IncrementBooksFinishedAtomic(ctx context.Context, userID string, delta int) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_stats (user_id, books_finished, updated_at)
		VALUES (?, MAX(0, ?), ?)
		ON CONFLICT(user_id) DO UPDATE SET
			books_finished = MAX(0, books_finished + ?),
			updated_at = ?`,
		userID, delta, formatTime(time.Now().UTC()),
		delta, formatTime(time.Now().UTC()),
	)
	return err
}

// UpdateUserStreak updates the streak fields and last listened date for a user.
func (s *Store) UpdateUserStreak(ctx context.Context, userID string, currentStreak, longestStreak int, lastListenedDate string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_stats (user_id, current_streak, longest_streak, last_listened_date, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			current_streak = excluded.current_streak,
			longest_streak = excluded.longest_streak,
			last_listened_date = excluded.last_listened_date,
			updated_at = excluded.updated_at`,
		userID, currentStreak, longestStreak, nullString(lastListenedDate), formatTime(time.Now().UTC()),
	)
	return err
}

// UpdateUserStatsLastListened updates the last listened date for a user.
func (s *Store) UpdateUserStatsLastListened(ctx context.Context, userID string, date string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_stats (user_id, last_listened_date, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			last_listened_date = excluded.last_listened_date,
			updated_at = excluded.updated_at`,
		userID, nullString(date), formatTime(time.Now().UTC()),
	)
	return err
}

// UpdateUserStatsFromEvent atomically ensures, increments listen time, and updates last listened date.
func (s *Store) UpdateUserStatsFromEvent(ctx context.Context, userID string, deltaMs int64, lastListenedDate string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_stats (user_id, total_listen_ms, last_listened_date, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			total_listen_ms = total_listen_ms + ?,
			last_listened_date = ?,
			updated_at = ?`,
		userID, deltaMs, nullString(lastListenedDate), formatTime(time.Now().UTC()),
		deltaMs, nullString(lastListenedDate), formatTime(time.Now().UTC()),
	)
	return err
}

// SetUserStats saves a complete UserStats (used for backfill/restore).
func (s *Store) SetUserStats(ctx context.Context, stats *domain.UserStats) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO user_stats (
			user_id, total_listen_ms, books_finished, current_streak,
			longest_streak, last_listened_date, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		stats.UserID,
		stats.TotalListenTimeMs,
		stats.TotalBooksFinished,
		stats.CurrentStreakDays,
		stats.LongestStreakDays,
		nullString(stats.LastListenedDate),
		formatTime(stats.UpdatedAt),
	)
	return err
}

// ClearAllUserStats deletes all user_stats rows.
func (s *Store) ClearAllUserStats(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM user_stats`)
	return err
}
