package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// profileColumns is the ordered list of columns selected in profile queries.
// Must match the scan order in scanProfile.
const profileColumns = `user_id, avatar_type, avatar_value, tagline, created_at, updated_at`

// scanProfile scans a sql.Row (or sql.Rows via its Scan method) into a domain.UserProfile.
func scanProfile(scanner interface{ Scan(dest ...any) error }) (*domain.UserProfile, error) {
	var p domain.UserProfile

	var (
		avatarType string
		createdAt  string
		updatedAt  string
	)

	err := scanner.Scan(
		&p.UserID,
		&avatarType,
		&p.AvatarValue,
		&p.Tagline,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, err
	}

	p.AvatarType = domain.AvatarType(avatarType)

	p.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	p.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}

	return &p, nil
}

// GetUserProfile retrieves a user profile by user ID.
// Returns store.ErrNotFound if the profile does not exist.
func (s *Store) GetUserProfile(ctx context.Context, userID string) (*domain.UserProfile, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+profileColumns+` FROM user_profiles WHERE user_id = ?`, userID)

	p, err := scanProfile(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

// GetUserProfilesByIDs retrieves profiles for multiple user IDs.
// Returns a map from user ID to profile. Missing profiles are omitted from the map.
func (s *Store) GetUserProfilesByIDs(ctx context.Context, userIDs []string) (map[string]*domain.UserProfile, error) {
	if len(userIDs) == 0 {
		return make(map[string]*domain.UserProfile), nil
	}

	placeholders := make([]string, len(userIDs))
	args := make([]any, len(userIDs))
	for i, id := range userIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(
		`SELECT %s FROM user_profiles WHERE user_id IN (%s)`,
		profileColumns,
		strings.Join(placeholders, ","),
	)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	profiles := make(map[string]*domain.UserProfile)
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		profiles[p.UserID] = p
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return profiles, nil
}

// SaveUserProfile creates or replaces a user profile.
// Uses INSERT OR REPLACE to handle both creation and update.
func (s *Store) SaveUserProfile(ctx context.Context, profile *domain.UserProfile) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO user_profiles (
			user_id, avatar_type, avatar_value, tagline, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)`,
		profile.UserID,
		string(profile.AvatarType),
		profile.AvatarValue,
		profile.Tagline,
		formatTime(profile.CreatedAt),
		formatTime(profile.UpdatedAt),
	)
	return err
}

// DeleteUserProfile deletes a user profile by user ID.
// Returns store.ErrNotFound if the profile does not exist.
func (s *Store) DeleteUserProfile(ctx context.Context, userID string) error {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM user_profiles WHERE user_id = ?`, userID)
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
