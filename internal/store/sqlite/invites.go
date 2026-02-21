package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// inviteColumns is the ordered list of columns selected in invite queries.
// Must match the scan order in scanInvite.
const inviteColumns = `id, created_at, updated_at, deleted_at,
	code, name, email, role, created_by, expires_at, claimed_at, claimed_by`

// scanInvite scans a sql.Row (or sql.Rows via its Scan method) into a domain.Invite.
func scanInvite(scanner interface{ Scan(dest ...any) error }) (*domain.Invite, error) {
	var inv domain.Invite

	var (
		createdAt string
		updatedAt string
		deletedAt sql.NullString
		role      string
		expiresAt string
		claimedAt sql.NullString
		claimedBy sql.NullString
	)

	err := scanner.Scan(
		&inv.ID,
		&createdAt,
		&updatedAt,
		&deletedAt,
		&inv.Code,
		&inv.Name,
		&inv.Email,
		&role,
		&inv.CreatedBy,
		&expiresAt,
		&claimedAt,
		&claimedBy,
	)
	if err != nil {
		return nil, err
	}

	// Parse timestamps.
	inv.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	inv.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	inv.DeletedAt, err = parseNullableTime(deletedAt)
	if err != nil {
		return nil, err
	}
	inv.ExpiresAt, err = parseTime(expiresAt)
	if err != nil {
		return nil, err
	}
	inv.ClaimedAt, err = parseNullableTime(claimedAt)
	if err != nil {
		return nil, err
	}

	// Enum fields.
	inv.Role = domain.Role(role)

	// Optional foreign key.
	if claimedBy.Valid {
		inv.ClaimedBy = claimedBy.String
	}

	return &inv, nil
}

// CreateInvite inserts a new invite into the database.
// Returns store.ErrAlreadyExists if the invite code already exists.
func (s *Store) CreateInvite(ctx context.Context, invite *domain.Invite) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO invites (
			id, created_at, updated_at, deleted_at,
			code, name, email, role, created_by, expires_at, claimed_at, claimed_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		invite.ID,
		formatTime(invite.CreatedAt),
		formatTime(invite.UpdatedAt),
		nullTimeString(invite.DeletedAt),
		invite.Code,
		invite.Name,
		invite.Email,
		string(invite.Role),
		invite.CreatedBy,
		formatTime(invite.ExpiresAt),
		nullTimeString(invite.ClaimedAt),
		nullString(invite.ClaimedBy),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return store.ErrAlreadyExists
		}
		return err
	}
	return nil
}

// GetInvite retrieves an invite by ID, excluding soft-deleted records.
// Returns store.ErrNotFound if the invite does not exist.
func (s *Store) GetInvite(ctx context.Context, id string) (*domain.Invite, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+inviteColumns+` FROM invites WHERE id = ? AND deleted_at IS NULL`, id)

	inv, err := scanInvite(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return inv, nil
}

// GetInviteByToken retrieves an invite by its unique code, excluding soft-deleted records.
// Returns store.ErrNotFound if the invite does not exist.
func (s *Store) GetInviteByToken(ctx context.Context, code string) (*domain.Invite, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+inviteColumns+` FROM invites WHERE code = ? AND deleted_at IS NULL`, code)

	inv, err := scanInvite(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return inv, nil
}

// UseInvite updates an invite to mark it as claimed.
// Sets claimed_at, claimed_by, and updated_at on the invite record.
func (s *Store) UseInvite(ctx context.Context, invite *domain.Invite) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE invites SET
			claimed_at = ?,
			claimed_by = ?,
			updated_at = ?
		WHERE id = ? AND deleted_at IS NULL`,
		nullTimeString(invite.ClaimedAt),
		nullString(invite.ClaimedBy),
		formatTime(invite.UpdatedAt),
		invite.ID,
	)
	return err
}

// ListInvites returns all non-deleted invites ordered by created_at descending.
func (s *Store) ListInvites(ctx context.Context) ([]*domain.Invite, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+inviteColumns+` FROM invites WHERE deleted_at IS NULL ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invites []*domain.Invite
	for rows.Next() {
		inv, err := scanInvite(rows)
		if err != nil {
			return nil, err
		}
		invites = append(invites, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return invites, nil
}

// GetInviteByCode is an alias for GetInviteByToken.
// Retrieves an invite by its unique code, excluding soft-deleted records.
func (s *Store) GetInviteByCode(ctx context.Context, code string) (*domain.Invite, error) {
	return s.GetInviteByToken(ctx, code)
}

// UpdateInvite performs a full update on an existing invite.
// Returns store.ErrNotFound if the invite does not exist.
func (s *Store) UpdateInvite(ctx context.Context, invite *domain.Invite) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE invites SET
			created_at = ?,
			updated_at = ?,
			deleted_at = ?,
			code = ?,
			name = ?,
			email = ?,
			role = ?,
			created_by = ?,
			expires_at = ?,
			claimed_at = ?,
			claimed_by = ?
		WHERE id = ? AND deleted_at IS NULL`,
		formatTime(invite.CreatedAt),
		formatTime(invite.UpdatedAt),
		nullTimeString(invite.DeletedAt),
		invite.Code,
		invite.Name,
		invite.Email,
		string(invite.Role),
		invite.CreatedBy,
		formatTime(invite.ExpiresAt),
		nullTimeString(invite.ClaimedAt),
		nullString(invite.ClaimedBy),
		invite.ID,
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

// DeleteInvite soft-deletes an invite by setting its deleted_at timestamp.
// This operation is idempotent.
func (s *Store) DeleteInvite(ctx context.Context, inviteID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE invites SET
			deleted_at = ?,
			updated_at = ?
		WHERE id = ? AND deleted_at IS NULL`,
		formatTime(time.Now().UTC()),
		formatTime(time.Now().UTC()),
		inviteID,
	)
	return err
}

// ListInvitesByCreator returns all non-deleted invites created by a specific user,
// ordered by created_at descending.
func (s *Store) ListInvitesByCreator(ctx context.Context, creatorID string) ([]*domain.Invite, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+inviteColumns+` FROM invites
		WHERE created_by = ? AND deleted_at IS NULL
		ORDER BY created_at DESC`,
		creatorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invites []*domain.Invite
	for rows.Next() {
		inv, err := scanInvite(rows)
		if err != nil {
			return nil, err
		}
		invites = append(invites, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return invites, nil
}
