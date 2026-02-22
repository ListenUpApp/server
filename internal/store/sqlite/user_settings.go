package sqlite

import (
	"context"
	"database/sql"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// settingsColumns is the ordered list of columns selected in user_settings queries.
// Must match the scan order in scanSettings.
const settingsColumns = `user_id, default_playback_speed, default_skip_forward_sec,
	default_skip_backward_sec, default_sleep_timer_min, shake_to_reset_sleep_timer, updated_at`

// scanSettings scans a sql.Row (or sql.Rows via its Scan method) into a domain.UserSettings.
func scanSettings(scanner interface{ Scan(dest ...any) error }) (*domain.UserSettings, error) {
	var us domain.UserSettings

	var (
		sleepTimerMin sql.NullInt64
		shakeToReset  int
		updatedAt     string
	)

	err := scanner.Scan(
		&us.UserID,
		&us.DefaultPlaybackSpeed,
		&us.DefaultSkipForwardSec,
		&us.DefaultSkipBackwardSec,
		&sleepTimerMin,
		&shakeToReset,
		&updatedAt,
	)
	if err != nil {
		return nil, err
	}

	if sleepTimerMin.Valid {
		v := int(sleepTimerMin.Int64)
		us.DefaultSleepTimerMin = &v
	}

	us.ShakeToResetSleepTimer = shakeToReset != 0

	us.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}

	return &us, nil
}

// GetUserSettings retrieves user settings by user ID.
// Returns store.ErrNotFound if no settings exist for the user.
func (s *Store) GetUserSettings(ctx context.Context, userID string) (*domain.UserSettings, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+settingsColumns+` FROM user_settings WHERE user_id = ?`, userID)

	us, err := scanSettings(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return us, nil
}

// UpsertUserSettings creates or replaces user settings.
func (s *Store) UpsertUserSettings(ctx context.Context, settings *domain.UserSettings) error {
	var sleepTimerVal sql.NullInt64
	if settings.DefaultSleepTimerMin != nil {
		sleepTimerVal = sql.NullInt64{Int64: int64(*settings.DefaultSleepTimerMin), Valid: true}
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO user_settings (
			user_id, default_playback_speed, default_skip_forward_sec,
			default_skip_backward_sec, default_sleep_timer_min,
			shake_to_reset_sleep_timer, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		settings.UserID,
		settings.DefaultPlaybackSpeed,
		settings.DefaultSkipForwardSec,
		settings.DefaultSkipBackwardSec,
		sleepTimerVal,
		boolToInt(settings.ShakeToResetSleepTimer),
		formatTime(settings.UpdatedAt),
	)
	return err
}

// DeleteUserSettings deletes user settings by user ID.
// Returns store.ErrNotFound if no settings exist for the user.
func (s *Store) DeleteUserSettings(ctx context.Context, userID string) error {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM user_settings WHERE user_id = ?`, userID)
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

// GetOrCreateUserSettings retrieves user settings, creating defaults if none exist.
// Uses domain.NewUserSettings to generate sensible defaults for new users.
func (s *Store) GetOrCreateUserSettings(ctx context.Context, userID string) (*domain.UserSettings, error) {
	us, err := s.GetUserSettings(ctx, userID)
	if err == nil {
		return us, nil
	}
	if err != store.ErrNotFound {
		return nil, err
	}

	// Create default settings.
	defaults := domain.NewUserSettings(userID)
	if err := s.UpsertUserSettings(ctx, defaults); err != nil {
		return nil, err
	}
	return defaults, nil
}
