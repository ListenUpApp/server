package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// activityColumns is the ordered list of columns selected in activity queries.
// Must match the scan order in scanActivity.
const activityColumns = `id, user_id, type, created_at,
	user_display_name, user_avatar_color, user_avatar_type, user_avatar_value,
	book_id, book_title, book_author_name, book_cover_path, is_reread,
	duration_ms, milestone_value, milestone_unit, lens_id, lens_name`

// scanActivity scans a sql.Row (or sql.Rows via its Scan method) into a domain.Activity.
func scanActivity(scanner interface{ Scan(dest ...any) error }) (*domain.Activity, error) {
	var a domain.Activity

	var (
		activityType   string
		createdAt      string
		bookID         sql.NullString
		bookTitle      sql.NullString
		bookAuthorName sql.NullString
		bookCoverPath  sql.NullString
		isReread       int
		milestoneUnit  sql.NullString
		lensID         sql.NullString
		lensName       sql.NullString
	)

	err := scanner.Scan(
		&a.ID,
		&a.UserID,
		&activityType,
		&createdAt,
		&a.UserDisplayName,
		&a.UserAvatarColor,
		&a.UserAvatarType,
		&a.UserAvatarValue,
		&bookID,
		&bookTitle,
		&bookAuthorName,
		&bookCoverPath,
		&isReread,
		&a.DurationMs,
		&a.MilestoneValue,
		&milestoneUnit,
		&lensID,
		&lensName,
	)
	if err != nil {
		return nil, err
	}

	// Enum field.
	a.Type = domain.ActivityType(activityType)

	// Parse timestamp.
	a.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}

	// Nullable fields.
	if bookID.Valid {
		a.BookID = bookID.String
	}
	if bookTitle.Valid {
		a.BookTitle = bookTitle.String
	}
	if bookAuthorName.Valid {
		a.BookAuthorName = bookAuthorName.String
	}
	if bookCoverPath.Valid {
		a.BookCoverPath = bookCoverPath.String
	}
	if milestoneUnit.Valid {
		a.MilestoneUnit = milestoneUnit.String
	}
	if lensID.Valid {
		a.ShelfID = lensID.String
	}
	if lensName.Valid {
		a.ShelfName = lensName.String
	}

	// Boolean fields.
	a.IsReread = isReread != 0

	return &a, nil
}

// CreateActivity inserts a new activity into the database.
// Returns store.ErrAlreadyExists if the activity ID already exists.
func (s *Store) CreateActivity(ctx context.Context, activity *domain.Activity) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO activities (
			id, user_id, type, created_at,
			user_display_name, user_avatar_color, user_avatar_type, user_avatar_value,
			book_id, book_title, book_author_name, book_cover_path, is_reread,
			duration_ms, milestone_value, milestone_unit, lens_id, lens_name
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		activity.ID,
		activity.UserID,
		string(activity.Type),
		formatTime(activity.CreatedAt),
		activity.UserDisplayName,
		activity.UserAvatarColor,
		activity.UserAvatarType,
		activity.UserAvatarValue,
		nullString(activity.BookID),
		nullString(activity.BookTitle),
		nullString(activity.BookAuthorName),
		nullString(activity.BookCoverPath),
		boolToInt(activity.IsReread),
		activity.DurationMs,
		activity.MilestoneValue,
		nullString(activity.MilestoneUnit),
		nullString(activity.ShelfID),
		nullString(activity.ShelfName),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return store.ErrAlreadyExists
		}
		return err
	}
	return nil
}

// GetActivity retrieves a single activity by ID.
// Returns store.ErrNotFound if the activity does not exist.
func (s *Store) GetActivity(ctx context.Context, id string) (*domain.Activity, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+activityColumns+` FROM activities WHERE id = ?`, id)

	a, err := scanActivity(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

// GetActivitiesFeed retrieves the global activity feed sorted by created_at descending.
// Use 'before' for cursor-based pagination (pass the CreatedAt of the last item).
// 'beforeID' provides deterministic cursor pagination when multiple activities share a timestamp.
// Returns up to 'limit' activities.
func (s *Store) GetActivitiesFeed(ctx context.Context, limit int, before *time.Time, beforeID string) ([]*domain.Activity, error) {
	var query string
	var args []any

	if before != nil && beforeID != "" {
		query = `SELECT ` + activityColumns + ` FROM activities
			WHERE (created_at < ? OR (created_at = ? AND id < ?))
			ORDER BY created_at DESC, id DESC
			LIMIT ?`
		ts := formatTime(*before)
		args = append(args, ts, ts, beforeID, limit)
	} else if before != nil {
		query = `SELECT ` + activityColumns + ` FROM activities
			WHERE created_at < ?
			ORDER BY created_at DESC
			LIMIT ?`
		args = append(args, formatTime(*before), limit)
	} else {
		query = `SELECT ` + activityColumns + ` FROM activities
			ORDER BY created_at DESC
			LIMIT ?`
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []*domain.Activity
	for rows.Next() {
		a, err := scanActivity(rows)
		if err != nil {
			return nil, err
		}
		activities = append(activities, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return activities, nil
}

// GetUserActivities retrieves activities for a specific user sorted by created_at descending.
func (s *Store) GetUserActivities(ctx context.Context, userID string, limit int) ([]*domain.Activity, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+activityColumns+` FROM activities
		WHERE user_id = ?
		ORDER BY created_at DESC
		LIMIT ?`,
		userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []*domain.Activity
	for rows.Next() {
		a, err := scanActivity(rows)
		if err != nil {
			return nil, err
		}
		activities = append(activities, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return activities, nil
}

// GetBookActivities retrieves activities for a specific book sorted by created_at descending.
func (s *Store) GetBookActivities(ctx context.Context, bookID string, limit int) ([]*domain.Activity, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+activityColumns+` FROM activities
		WHERE book_id = ?
		ORDER BY created_at DESC
		LIMIT ?`,
		bookID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []*domain.Activity
	for rows.Next() {
		a, err := scanActivity(rows)
		if err != nil {
			return nil, err
		}
		activities = append(activities, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return activities, nil
}

// GetUserMilestoneState retrieves the milestone tracking state for a user.
// Returns nil, nil if no state exists (user hasn't had any milestones tracked yet).
func (s *Store) GetUserMilestoneState(ctx context.Context, userID string) (*domain.UserMilestoneState, error) {
	var (
		state     domain.UserMilestoneState
		updatedAt string
	)

	err := s.db.QueryRowContext(ctx,
		`SELECT user_id, last_streak_days, last_listen_hours_total, updated_at
		FROM user_milestone_states WHERE user_id = ?`, userID).Scan(
		&state.UserID,
		&state.LastStreakDays,
		&state.LastListenHoursTotal,
		&updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil // No state yet, not an error
	}
	if err != nil {
		return nil, err
	}

	state.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}

	return &state, nil
}

// UpdateUserMilestoneState updates or creates the milestone tracking state for a user.
func (s *Store) UpdateUserMilestoneState(ctx context.Context, userID string, streakDays, listenHours int) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO user_milestone_states (
			user_id, last_streak_days, last_listen_hours_total, updated_at
		) VALUES (?, ?, ?, ?)`,
		userID,
		streakDays,
		listenHours,
		formatTime(time.Now().UTC()),
	)
	return err
}
