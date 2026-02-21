package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// boolToInt converts a bool to an int for SQLite storage.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// userColumns is the ordered list of columns selected in user queries.
// Must match the scan order in scanUser.
const userColumns = `id, created_at, updated_at, deleted_at, email, email_lower,
	password_hash, is_root, role, status, invited_by, approved_by, approved_at,
	display_name, first_name, last_name, last_login_at,
	can_download, can_share, avatar_type, avatar_color`

// scanUser scans a sql.Row (or sql.Rows via its Scan method) into a domain.User.
func scanUser(scanner interface{ Scan(dest ...any) error }) (*domain.User, error) {
	var u domain.User

	var (
		createdAt   string
		updatedAt   string
		deletedAt   sql.NullString
		emailLower  string
		passwordH   sql.NullString
		isRoot      int
		role        string
		status      string
		invitedBy   sql.NullString
		approvedBy  sql.NullString
		approvedAt  sql.NullString
		lastLoginAt string
		canDownload int
		canShare    int
		avatarType  sql.NullString
		avatarColor sql.NullString
	)

	err := scanner.Scan(
		&u.ID,
		&createdAt,
		&updatedAt,
		&deletedAt,
		&u.Email,
		&emailLower,
		&passwordH,
		&isRoot,
		&role,
		&status,
		&invitedBy,
		&approvedBy,
		&approvedAt,
		&u.DisplayName,
		&u.FirstName,
		&u.LastName,
		&lastLoginAt,
		&canDownload,
		&canShare,
		&avatarType,  // throwaway - not in domain model
		&avatarColor, // throwaway - not in domain model
	)
	if err != nil {
		return nil, err
	}

	// Parse timestamps.
	u.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	u.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	u.DeletedAt, err = parseNullableTime(deletedAt)
	if err != nil {
		return nil, err
	}

	// Parse optional password hash.
	if passwordH.Valid {
		u.PasswordHash = passwordH.String
	}

	// Boolean fields.
	u.IsRoot = isRoot != 0

	// Enum fields.
	u.Role = domain.Role(role)
	u.Status = domain.UserStatus(status)

	// Optional foreign keys.
	if invitedBy.Valid {
		u.InvitedBy = invitedBy.String
	}
	if approvedBy.Valid {
		u.ApprovedBy = approvedBy.String
	}

	// ApprovedAt: time.Time zero value means not approved.
	if approvedAt.Valid && approvedAt.String != "" {
		u.ApprovedAt, err = parseTime(approvedAt.String)
		if err != nil {
			return nil, err
		}
	}

	// Last login.
	u.LastLoginAt, err = parseTime(lastLoginAt)
	if err != nil {
		return nil, err
	}

	// Permissions.
	u.Permissions.CanDownload = canDownload != 0
	u.Permissions.CanShare = canShare != 0

	return &u, nil
}

// CreateUser inserts a new user into the database.
// Returns store.ErrAlreadyExists if the user ID or email already exists.
func (s *Store) CreateUser(ctx context.Context, user *domain.User) error {
	emailLower := strings.ToLower(strings.TrimSpace(user.Email))

	// ApprovedAt: store as NULL if zero value.
	var approvedAtVal sql.NullString
	if !user.ApprovedAt.IsZero() {
		approvedAtVal = sql.NullString{String: formatTime(user.ApprovedAt), Valid: true}
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (
			id, created_at, updated_at, deleted_at, email, email_lower,
			password_hash, is_root, role, status, invited_by, approved_by, approved_at,
			display_name, first_name, last_name, last_login_at,
			can_download, can_share, avatar_type, avatar_color
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		user.ID,
		formatTime(user.CreatedAt),
		formatTime(user.UpdatedAt),
		nullTimeString(user.DeletedAt),
		user.Email,
		emailLower,
		nullString(user.PasswordHash),
		boolToInt(user.IsRoot),
		string(user.Role),
		string(user.Status),
		nullString(user.InvitedBy),
		nullString(user.ApprovedBy),
		approvedAtVal,
		user.DisplayName,
		user.FirstName,
		user.LastName,
		formatTime(user.LastLoginAt),
		boolToInt(user.Permissions.CanDownload),
		boolToInt(user.Permissions.CanShare),
		"", // avatar_type - future use
		"", // avatar_color - future use
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return store.ErrAlreadyExists
		}
		return err
	}
	return nil
}

// GetUser retrieves a user by ID, excluding soft-deleted records.
// Returns store.ErrNotFound if the user does not exist.
func (s *Store) GetUser(ctx context.Context, id string) (*domain.User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = ? AND deleted_at IS NULL`, id)

	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// GetUserByEmail retrieves a user by exact email match, excluding soft-deleted records.
// Returns store.ErrNotFound if the user does not exist.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE email = ? AND deleted_at IS NULL`, email)

	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// GetUserByEmailLower retrieves a user by lowercased email, excluding soft-deleted records.
// Returns store.ErrNotFound if the user does not exist.
func (s *Store) GetUserByEmailLower(ctx context.Context, email string) (*domain.User, error) {
	lower := strings.ToLower(strings.TrimSpace(email))
	row := s.db.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE email_lower = ? AND deleted_at IS NULL`, lower)

	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// ListUsers returns all non-deleted users.
func (s *Store) ListUsers(ctx context.Context) ([]*domain.User, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE deleted_at IS NULL ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

// UpdateUser performs a full row update on an existing user.
// Returns store.ErrNotFound if the user does not exist or is soft-deleted.
func (s *Store) UpdateUser(ctx context.Context, user *domain.User) error {
	emailLower := strings.ToLower(strings.TrimSpace(user.Email))

	var approvedAtVal sql.NullString
	if !user.ApprovedAt.IsZero() {
		approvedAtVal = sql.NullString{String: formatTime(user.ApprovedAt), Valid: true}
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE users SET
			created_at = ?,
			updated_at = ?,
			email = ?,
			email_lower = ?,
			password_hash = ?,
			is_root = ?,
			role = ?,
			status = ?,
			invited_by = ?,
			approved_by = ?,
			approved_at = ?,
			display_name = ?,
			first_name = ?,
			last_name = ?,
			last_login_at = ?,
			can_download = ?,
			can_share = ?
		WHERE id = ? AND deleted_at IS NULL`,
		formatTime(user.CreatedAt),
		formatTime(user.UpdatedAt),
		user.Email,
		emailLower,
		nullString(user.PasswordHash),
		boolToInt(user.IsRoot),
		string(user.Role),
		string(user.Status),
		nullString(user.InvitedBy),
		nullString(user.ApprovedBy),
		approvedAtVal,
		user.DisplayName,
		user.FirstName,
		user.LastName,
		formatTime(user.LastLoginAt),
		boolToInt(user.Permissions.CanDownload),
		boolToInt(user.Permissions.CanShare),
		user.ID,
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

// DeleteUser performs a soft delete by setting deleted_at and updated_at.
// Returns store.ErrNotFound if the user does not exist or is already deleted.
func (s *Store) DeleteUser(ctx context.Context, id string) error {
	now := formatTime(time.Now())

	result, err := s.db.ExecContext(ctx, `
		UPDATE users SET deleted_at = ?, updated_at = ?
		WHERE id = ? AND deleted_at IS NULL`,
		now, now, id)
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
