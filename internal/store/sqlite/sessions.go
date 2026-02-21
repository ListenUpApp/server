package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// sessionColumns is the ordered list of columns selected in session queries.
// Must match the scan order in scanSession.
const sessionColumns = `id, user_id, refresh_token_hash, expires_at, created_at, last_seen_at,
	ip_address, device_type, platform, platform_version,
	client_name, client_version, client_build,
	device_name, device_model, browser_name, browser_version`

// scanSession scans a sql.Row (or sql.Rows via its Scan method) into a domain.Session.
func scanSession(scanner interface{ Scan(dest ...any) error }) (*domain.Session, error) {
	var s domain.Session

	var (
		refreshTokenHash sql.NullString
		expiresAt        string
		createdAt        string
		lastSeenAt       string
		ipAddress        sql.NullString
		deviceType       sql.NullString
		platform         sql.NullString
		platformVersion  sql.NullString
		clientName       sql.NullString
		clientVersion    sql.NullString
		clientBuild      sql.NullString
		deviceName       sql.NullString
		deviceModel      sql.NullString
		browserName      sql.NullString
		browserVersion   sql.NullString
	)

	err := scanner.Scan(
		&s.ID,
		&s.UserID,
		&refreshTokenHash,
		&expiresAt,
		&createdAt,
		&lastSeenAt,
		&ipAddress,
		&deviceType,
		&platform,
		&platformVersion,
		&clientName,
		&clientVersion,
		&clientBuild,
		&deviceName,
		&deviceModel,
		&browserName,
		&browserVersion,
	)
	if err != nil {
		return nil, err
	}

	// Parse timestamps.
	s.ExpiresAt, err = parseTime(expiresAt)
	if err != nil {
		return nil, err
	}
	s.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	s.LastSeenAt, err = parseTime(lastSeenAt)
	if err != nil {
		return nil, err
	}

	// Optional string fields.
	if refreshTokenHash.Valid {
		s.RefreshTokenHash = refreshTokenHash.String
	}
	if ipAddress.Valid {
		s.IPAddress = ipAddress.String
	}
	if deviceType.Valid {
		s.DeviceType = deviceType.String
	}
	if platform.Valid {
		s.Platform = platform.String
	}
	if platformVersion.Valid {
		s.PlatformVersion = platformVersion.String
	}
	if clientName.Valid {
		s.ClientName = clientName.String
	}
	if clientVersion.Valid {
		s.ClientVersion = clientVersion.String
	}
	if clientBuild.Valid {
		s.ClientBuild = clientBuild.String
	}
	if deviceName.Valid {
		s.DeviceName = deviceName.String
	}
	if deviceModel.Valid {
		s.DeviceModel = deviceModel.String
	}
	if browserName.Valid {
		s.BrowserName = browserName.String
	}
	if browserVersion.Valid {
		s.BrowserVersion = browserVersion.String
	}

	return &s, nil
}

// CreateSession inserts a new session into the database.
// Returns store.ErrAlreadyExists if the session ID already exists.
func (s *Store) CreateSession(ctx context.Context, session *domain.Session) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (
			id, user_id, refresh_token_hash, expires_at, created_at, last_seen_at,
			ip_address, device_type, platform, platform_version,
			client_name, client_version, client_build,
			device_name, device_model, browser_name, browser_version
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID,
		session.UserID,
		nullString(session.RefreshTokenHash),
		formatTime(session.ExpiresAt),
		formatTime(session.CreatedAt),
		formatTime(session.LastSeenAt),
		nullString(session.IPAddress),
		nullString(session.DeviceType),
		nullString(session.Platform),
		nullString(session.PlatformVersion),
		nullString(session.ClientName),
		nullString(session.ClientVersion),
		nullString(session.ClientBuild),
		nullString(session.DeviceName),
		nullString(session.DeviceModel),
		nullString(session.BrowserName),
		nullString(session.BrowserVersion),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return store.ErrAlreadyExists
		}
		return err
	}
	return nil
}

// GetSession retrieves a session by ID.
// Returns store.ErrNotFound if the session does not exist.
func (s *Store) GetSession(ctx context.Context, id string) (*domain.Session, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+sessionColumns+` FROM sessions WHERE id = ?`, id)

	sess, err := scanSession(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return sess, nil
}

// ListSessions returns all sessions.
func (s *Store) ListSessions(ctx context.Context) ([]*domain.Session, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+sessionColumns+` FROM sessions ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*domain.Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sessions, nil
}

// UpdateSession performs a full row update on an existing session.
// Returns store.ErrNotFound if the session does not exist.
func (s *Store) UpdateSession(ctx context.Context, session *domain.Session) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE sessions SET
			user_id = ?,
			refresh_token_hash = ?,
			expires_at = ?,
			created_at = ?,
			last_seen_at = ?,
			ip_address = ?,
			device_type = ?,
			platform = ?,
			platform_version = ?,
			client_name = ?,
			client_version = ?,
			client_build = ?,
			device_name = ?,
			device_model = ?,
			browser_name = ?,
			browser_version = ?
		WHERE id = ?`,
		session.UserID,
		nullString(session.RefreshTokenHash),
		formatTime(session.ExpiresAt),
		formatTime(session.CreatedAt),
		formatTime(session.LastSeenAt),
		nullString(session.IPAddress),
		nullString(session.DeviceType),
		nullString(session.Platform),
		nullString(session.PlatformVersion),
		nullString(session.ClientName),
		nullString(session.ClientVersion),
		nullString(session.ClientBuild),
		nullString(session.DeviceName),
		nullString(session.DeviceModel),
		nullString(session.BrowserName),
		nullString(session.BrowserVersion),
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

// DeleteSession performs a hard delete of a session by ID.
// Returns store.ErrNotFound if the session does not exist.
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE id = ?`, id)
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

// GetSessionsByUser returns all sessions for a given user ID.
func (s *Store) GetSessionsByUser(ctx context.Context, userID string) ([]*domain.Session, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+sessionColumns+` FROM sessions WHERE user_id = ? ORDER BY created_at ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*domain.Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sessions, nil
}

// DeleteExpiredSessions deletes all sessions where expires_at is in the past.
// Returns the number of sessions deleted.
func (s *Store) DeleteExpiredSessions(ctx context.Context) (int, error) {
	now := formatTime(time.Now())

	result, err := s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE expires_at < ?`, now)
	if err != nil {
		return 0, err
	}

	n, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}
