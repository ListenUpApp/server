package sqlite

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"fmt"
	"strings"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// --- ABSImport ---

// absImportColumns is the ordered list of columns selected in abs_imports queries.
const absImportColumns = `id, name, backup_path, status, created_at, updated_at, completed_at,
	total_users, total_books, total_sessions, users_mapped, books_mapped, sessions_imported`

// scanABSImport scans a sql.Row (or sql.Rows via its Scan method) into a domain.ABSImport.
func scanABSImport(scanner interface{ Scan(dest ...any) error }) (*domain.ABSImport, error) {
	var imp domain.ABSImport

	var (
		status      string
		createdAt   string
		updatedAt   string
		completedAt sql.NullString
	)

	err := scanner.Scan(
		&imp.ID,
		&imp.Name,
		&imp.BackupPath,
		&status,
		&createdAt,
		&updatedAt,
		&completedAt,
		&imp.TotalUsers,
		&imp.TotalBooks,
		&imp.TotalSessions,
		&imp.UsersMapped,
		&imp.BooksMapped,
		&imp.SessionsImported,
	)
	if err != nil {
		return nil, err
	}

	imp.Status = domain.ABSImportStatus(status)

	imp.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	imp.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	imp.CompletedAt, err = parseNullableTime(completedAt)
	if err != nil {
		return nil, err
	}

	return &imp, nil
}

// CreateABSImport inserts a new ABS import record.
// Returns store.ErrAlreadyExists on duplicate ID.
func (s *Store) CreateABSImport(ctx context.Context, imp *domain.ABSImport) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO abs_imports (
			id, name, backup_path, status, created_at, updated_at, completed_at,
			total_users, total_books, total_sessions, users_mapped, books_mapped, sessions_imported
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		imp.ID,
		imp.Name,
		imp.BackupPath,
		string(imp.Status),
		formatTime(imp.CreatedAt),
		formatTime(imp.UpdatedAt),
		nullTimeString(imp.CompletedAt),
		imp.TotalUsers,
		imp.TotalBooks,
		imp.TotalSessions,
		imp.UsersMapped,
		imp.BooksMapped,
		imp.SessionsImported,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return store.ErrAlreadyExists
		}
		return err
	}
	return nil
}

// GetABSImport retrieves an ABS import by ID.
// Returns store.ErrNotFound if the import does not exist.
func (s *Store) GetABSImport(ctx context.Context, id string) (*domain.ABSImport, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+absImportColumns+` FROM abs_imports WHERE id = ?`, id)

	imp, err := scanABSImport(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return imp, nil
}

// ListABSImports returns all ABS imports ordered by created_at descending.
func (s *Store) ListABSImports(ctx context.Context) ([]*domain.ABSImport, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+absImportColumns+` FROM abs_imports ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var imports []*domain.ABSImport
	for rows.Next() {
		imp, err := scanABSImport(rows)
		if err != nil {
			return nil, err
		}
		imports = append(imports, imp)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return imports, nil
}

// UpdateABSImport performs a full row update on an existing ABS import.
// Returns store.ErrNotFound if the import does not exist.
func (s *Store) UpdateABSImport(ctx context.Context, imp *domain.ABSImport) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE abs_imports SET
			name = ?,
			backup_path = ?,
			status = ?,
			created_at = ?,
			updated_at = ?,
			completed_at = ?,
			total_users = ?,
			total_books = ?,
			total_sessions = ?,
			users_mapped = ?,
			books_mapped = ?,
			sessions_imported = ?
		WHERE id = ?`,
		imp.Name,
		imp.BackupPath,
		string(imp.Status),
		formatTime(imp.CreatedAt),
		formatTime(imp.UpdatedAt),
		nullTimeString(imp.CompletedAt),
		imp.TotalUsers,
		imp.TotalBooks,
		imp.TotalSessions,
		imp.UsersMapped,
		imp.BooksMapped,
		imp.SessionsImported,
		imp.ID,
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

// DeleteABSImport deletes an ABS import and all related data (cascaded).
// Returns store.ErrNotFound if the import does not exist.
func (s *Store) DeleteABSImport(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM abs_imports WHERE id = ?`, id)
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

// --- ABSImportUser ---

// absImportUserColumns is the ordered list of columns selected in abs_import_users queries.
const absImportUserColumns = `import_id, abs_user_id, abs_username, abs_email,
	listenup_id, lu_email, lu_display_name, mapped_at, session_count, total_listen_ms,
	confidence, match_reason, suggestions`

// scanABSImportUser scans a sql.Row (or sql.Rows via its Scan method) into a domain.ABSImportUser.
func scanABSImportUser(scanner interface{ Scan(dest ...any) error }) (*domain.ABSImportUser, error) {
	var u domain.ABSImportUser

	var (
		listenUpID      sql.NullString
		luEmail         sql.NullString
		luDisplayName   sql.NullString
		mappedAt        sql.NullString
		suggestionsJSON string
	)

	err := scanner.Scan(
		&u.ImportID,
		&u.ABSUserID,
		&u.ABSUsername,
		&u.ABSEmail,
		&listenUpID,
		&luEmail,
		&luDisplayName,
		&mappedAt,
		&u.SessionCount,
		&u.TotalListenMs,
		&u.Confidence,
		&u.MatchReason,
		&suggestionsJSON,
	)
	if err != nil {
		return nil, err
	}

	if listenUpID.Valid {
		u.ListenUpID = &listenUpID.String
	}
	if luEmail.Valid {
		u.ListenUpEmail = &luEmail.String
	}
	if luDisplayName.Valid {
		u.ListenUpDisplayName = &luDisplayName.String
	}

	if mappedAt.Valid {
		t, err := parseTime(mappedAt.String)
		if err != nil {
			return nil, err
		}
		u.MappedAt = &t
	}

	if err := json.Unmarshal([]byte(suggestionsJSON), &u.Suggestions); err != nil {
		return nil, fmt.Errorf("unmarshal suggestions: %w", err)
	}

	return &u, nil
}

// CreateABSImportUser inserts a new ABS import user record.
func (s *Store) CreateABSImportUser(ctx context.Context, user *domain.ABSImportUser) error {
	suggestionsJSON, err := json.Marshal(user.Suggestions)
	if err != nil {
		return fmt.Errorf("marshal suggestions: %w", err)
	}

	var listenUpID sql.NullString
	if user.ListenUpID != nil {
		listenUpID = sql.NullString{String: *user.ListenUpID, Valid: true}
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO abs_import_users (
			import_id, abs_user_id, abs_username, abs_email,
			listenup_id, lu_email, lu_display_name, mapped_at, session_count, total_listen_ms,
			confidence, match_reason, suggestions
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		user.ImportID,
		user.ABSUserID,
		user.ABSUsername,
		user.ABSEmail,
		listenUpID,
		nullableString(user.ListenUpEmail),
		nullableString(user.ListenUpDisplayName),
		nullTimeString(user.MappedAt),
		user.SessionCount,
		user.TotalListenMs,
		user.Confidence,
		user.MatchReason,
		string(suggestionsJSON),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return store.ErrAlreadyExists
		}
		return err
	}
	return nil
}

// GetABSImportUser retrieves an ABS import user by import ID and ABS user ID.
// Returns store.ErrNotFound if the user does not exist.
func (s *Store) GetABSImportUser(ctx context.Context, importID, absUserID string) (*domain.ABSImportUser, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+absImportUserColumns+` FROM abs_import_users
		WHERE import_id = ? AND abs_user_id = ?`,
		importID, absUserID)

	u, err := scanABSImportUser(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// ListABSImportUsers returns ABS import users for a given import, optionally filtered.
func (s *Store) ListABSImportUsers(ctx context.Context, importID string, filter domain.MappingFilter) ([]*domain.ABSImportUser, error) {
	query := `SELECT ` + absImportUserColumns + ` FROM abs_import_users WHERE import_id = ?`
	args := []any{importID}

	switch filter {
	case domain.MappingFilterMapped:
		query += ` AND listenup_id IS NOT NULL`
	case domain.MappingFilterUnmapped:
		query += ` AND listenup_id IS NULL`
	}

	query += ` ORDER BY abs_username ASC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*domain.ABSImportUser
	for rows.Next() {
		u, err := scanABSImportUser(rows)
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

// UpdateABSImportUserMapping updates the ListenUp user mapping for an ABS import user.
func (s *Store) UpdateABSImportUserMapping(ctx context.Context, importID, absUserID string, listenUpID, luEmail, luDisplayName *string) error {
	var luID sql.NullString
	var mappedAt sql.NullString
	if listenUpID != nil && *listenUpID != "" {
		luID = sql.NullString{String: *listenUpID, Valid: true}
		mappedAt = sql.NullString{String: formatTime(time.Now().UTC()), Valid: true}
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE abs_import_users SET listenup_id = ?, lu_email = ?, lu_display_name = ?, mapped_at = ?
		WHERE import_id = ? AND abs_user_id = ?`,
		luID, nullableString(luEmail), nullableString(luDisplayName), mappedAt, importID, absUserID)
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

// --- ABSImportBook ---

// absImportBookColumns is the ordered list of columns selected in abs_import_books queries.
const absImportBookColumns = `import_id, abs_media_id, abs_title, abs_author, abs_duration_ms,
	abs_asin, abs_isbn, listenup_id, lu_title, lu_author, mapped_at, session_count,
	confidence, match_reason, suggestions`

// scanABSImportBook scans a sql.Row (or sql.Rows via its Scan method) into a domain.ABSImportBook.
func scanABSImportBook(scanner interface{ Scan(dest ...any) error }) (*domain.ABSImportBook, error) {
	var b domain.ABSImportBook

	var (
		listenUpID      sql.NullString
		luTitle         sql.NullString
		luAuthor        sql.NullString
		mappedAt        sql.NullString
		suggestionsJSON string
	)

	err := scanner.Scan(
		&b.ImportID,
		&b.ABSMediaID,
		&b.ABSTitle,
		&b.ABSAuthor,
		&b.ABSDurationMs,
		&b.ABSASIN,
		&b.ABSISBN,
		&listenUpID,
		&luTitle,
		&luAuthor,
		&mappedAt,
		&b.SessionCount,
		&b.Confidence,
		&b.MatchReason,
		&suggestionsJSON,
	)
	if err != nil {
		return nil, err
	}

	if listenUpID.Valid {
		b.ListenUpID = &listenUpID.String
	}
	if luTitle.Valid {
		b.ListenUpTitle = &luTitle.String
	}
	if luAuthor.Valid {
		b.ListenUpAuthor = &luAuthor.String
	}

	if mappedAt.Valid {
		t, err := parseTime(mappedAt.String)
		if err != nil {
			return nil, err
		}
		b.MappedAt = &t
	}

	if err := json.Unmarshal([]byte(suggestionsJSON), &b.Suggestions); err != nil {
		return nil, fmt.Errorf("unmarshal suggestions: %w", err)
	}

	return &b, nil
}

// CreateABSImportBook inserts a new ABS import book record.
func (s *Store) CreateABSImportBook(ctx context.Context, book *domain.ABSImportBook) error {
	suggestionsJSON, err := json.Marshal(book.Suggestions)
	if err != nil {
		return fmt.Errorf("marshal suggestions: %w", err)
	}

	var listenUpID sql.NullString
	if book.ListenUpID != nil {
		listenUpID = sql.NullString{String: *book.ListenUpID, Valid: true}
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO abs_import_books (
			import_id, abs_media_id, abs_title, abs_author, abs_duration_ms,
			abs_asin, abs_isbn, listenup_id, lu_title, lu_author, mapped_at, session_count,
			confidence, match_reason, suggestions
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		book.ImportID,
		book.ABSMediaID,
		book.ABSTitle,
		book.ABSAuthor,
		book.ABSDurationMs,
		book.ABSASIN,
		book.ABSISBN,
		listenUpID,
		nullableString(book.ListenUpTitle),
		nullableString(book.ListenUpAuthor),
		nullTimeString(book.MappedAt),
		book.SessionCount,
		book.Confidence,
		book.MatchReason,
		string(suggestionsJSON),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return store.ErrAlreadyExists
		}
		return err
	}
	return nil
}

// GetABSImportBook retrieves an ABS import book by import ID and ABS media ID.
// Returns store.ErrNotFound if the book does not exist.
func (s *Store) GetABSImportBook(ctx context.Context, importID, absMediaID string) (*domain.ABSImportBook, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+absImportBookColumns+` FROM abs_import_books
		WHERE import_id = ? AND abs_media_id = ?`,
		importID, absMediaID)

	b, err := scanABSImportBook(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return b, nil
}

// ListABSImportBooks returns ABS import books for a given import, optionally filtered.
func (s *Store) ListABSImportBooks(ctx context.Context, importID string, filter domain.MappingFilter) ([]*domain.ABSImportBook, error) {
	query := `SELECT ` + absImportBookColumns + ` FROM abs_import_books WHERE import_id = ?`
	args := []any{importID}

	switch filter {
	case domain.MappingFilterMapped:
		query += ` AND listenup_id IS NOT NULL`
	case domain.MappingFilterUnmapped:
		query += ` AND listenup_id IS NULL`
	}

	query += ` ORDER BY abs_title ASC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []*domain.ABSImportBook
	for rows.Next() {
		b, err := scanABSImportBook(rows)
		if err != nil {
			return nil, err
		}
		books = append(books, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return books, nil
}

// UpdateABSImportBookMapping updates the ListenUp book mapping for an ABS import book.
func (s *Store) UpdateABSImportBookMapping(ctx context.Context, importID, absMediaID string, listenUpID, luTitle, luAuthor *string) error {
	var luID sql.NullString
	var mappedAt sql.NullString
	if listenUpID != nil && *listenUpID != "" {
		luID = sql.NullString{String: *listenUpID, Valid: true}
		mappedAt = sql.NullString{String: formatTime(time.Now().UTC()), Valid: true}
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE abs_import_books SET listenup_id = ?, lu_title = ?, lu_author = ?, mapped_at = ?
		WHERE import_id = ? AND abs_media_id = ?`,
		luID, nullableString(luTitle), nullableString(luAuthor), mappedAt, importID, absMediaID)
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

// --- ABSImportSession ---

// absImportSessionColumns is the ordered list of columns selected in abs_import_sessions queries.
const absImportSessionColumns = `import_id, abs_session_id, abs_user_id, abs_media_id,
	start_time, duration, start_position, end_position,
	status, imported_at, skip_reason`

// scanABSImportSession scans a sql.Row (or sql.Rows via its Scan method) into a domain.ABSImportSession.
func scanABSImportSession(scanner interface{ Scan(dest ...any) error }) (*domain.ABSImportSession, error) {
	var sess domain.ABSImportSession

	var (
		startTime  string
		status     string
		importedAt sql.NullString
		skipReason sql.NullString
	)

	err := scanner.Scan(
		&sess.ImportID,
		&sess.ABSSessionID,
		&sess.ABSUserID,
		&sess.ABSMediaID,
		&startTime,
		&sess.Duration,
		&sess.StartPosition,
		&sess.EndPosition,
		&status,
		&importedAt,
		&skipReason,
	)
	if err != nil {
		return nil, err
	}

	sess.StartTime, err = parseTime(startTime)
	if err != nil {
		return nil, err
	}

	sess.Status = domain.SessionImportStatus(status)

	sess.ImportedAt, err = parseNullableTime(importedAt)
	if err != nil {
		return nil, err
	}

	if skipReason.Valid {
		sess.SkipReason = &skipReason.String
	}

	return &sess, nil
}

// CreateABSImportSession inserts a new ABS import session record.
func (s *Store) CreateABSImportSession(ctx context.Context, session *domain.ABSImportSession) error {
	var skipReason sql.NullString
	if session.SkipReason != nil {
		skipReason = sql.NullString{String: *session.SkipReason, Valid: true}
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO abs_import_sessions (
			import_id, abs_session_id, abs_user_id, abs_media_id,
			start_time, duration, start_position, end_position,
			status, imported_at, skip_reason
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ImportID,
		session.ABSSessionID,
		session.ABSUserID,
		session.ABSMediaID,
		formatTime(session.StartTime),
		session.Duration,
		session.StartPosition,
		session.EndPosition,
		string(session.Status),
		nullTimeString(session.ImportedAt),
		skipReason,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return store.ErrAlreadyExists
		}
		return err
	}
	return nil
}

// GetABSImportSession retrieves an ABS import session by import ID and session ID.
// Returns store.ErrNotFound if the session does not exist.
func (s *Store) GetABSImportSession(ctx context.Context, importID, sessionID string) (*domain.ABSImportSession, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+absImportSessionColumns+` FROM abs_import_sessions
		WHERE import_id = ? AND abs_session_id = ?`,
		importID, sessionID)

	sess, err := scanABSImportSession(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return sess, nil
}

// ListABSImportSessions returns ABS import sessions for a given import, optionally filtered by status.
func (s *Store) ListABSImportSessions(ctx context.Context, importID string, filter domain.SessionStatusFilter) ([]*domain.ABSImportSession, error) {
	query := `SELECT ` + absImportSessionColumns + ` FROM abs_import_sessions WHERE import_id = ?`
	args := []any{importID}

	switch filter {
	case domain.SessionFilterPending:
		query += ` AND (status = 'pending_user' OR status = 'pending_book')`
	case domain.SessionFilterReady:
		query += ` AND status = 'ready'`
	case domain.SessionFilterImported:
		query += ` AND status = 'imported'`
	case domain.SessionFilterSkipped:
		query += ` AND status = 'skipped'`
	}

	query += ` ORDER BY start_time ASC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*domain.ABSImportSession
	for rows.Next() {
		sess, err := scanABSImportSession(rows)
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

// UpdateABSImportSessionStatus updates the status of an ABS import session.
// If the status is "imported", sets imported_at to now.
func (s *Store) UpdateABSImportSessionStatus(ctx context.Context, importID, sessionID string, status domain.SessionImportStatus) error {
	var importedAt sql.NullString
	if status == domain.SessionStatusImported {
		importedAt = sql.NullString{String: formatTime(time.Now().UTC()), Valid: true}
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE abs_import_sessions SET status = ?, imported_at = COALESCE(?, imported_at)
		WHERE import_id = ? AND abs_session_id = ?`,
		string(status), importedAt, importID, sessionID)
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

// SkipABSImportSession marks an ABS import session as skipped with a reason.
func (s *Store) SkipABSImportSession(ctx context.Context, importID, sessionID, reason string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE abs_import_sessions SET status = 'skipped', skip_reason = ?
		WHERE import_id = ? AND abs_session_id = ?`,
		reason, importID, sessionID)
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

// RecalculateSessionStatuses recalculates the import status for all sessions
// in a given import based on current user and book mappings.
// Sessions become "ready" when both their user and book are mapped.
// Sessions remain "pending_user" or "pending_book" when one or both are unmapped.
// Already imported or skipped sessions are not affected.
func (s *Store) RecalculateSessionStatuses(ctx context.Context, importID string) error {
	// Update sessions where user is not mapped to pending_user.
	_, err := s.db.ExecContext(ctx, `
		UPDATE abs_import_sessions SET status = 'pending_user'
		WHERE import_id = ?
			AND status NOT IN ('imported', 'skipped')
			AND abs_user_id NOT IN (
				SELECT abs_user_id FROM abs_import_users
				WHERE import_id = ? AND listenup_id IS NOT NULL
			)`,
		importID, importID)
	if err != nil {
		return fmt.Errorf("update pending_user sessions: %w", err)
	}

	// Update sessions where user is mapped but book is not to pending_book.
	_, err = s.db.ExecContext(ctx, `
		UPDATE abs_import_sessions SET status = 'pending_book'
		WHERE import_id = ?
			AND status NOT IN ('imported', 'skipped')
			AND abs_user_id IN (
				SELECT abs_user_id FROM abs_import_users
				WHERE import_id = ? AND listenup_id IS NOT NULL
			)
			AND abs_media_id NOT IN (
				SELECT abs_media_id FROM abs_import_books
				WHERE import_id = ? AND listenup_id IS NOT NULL
			)`,
		importID, importID, importID)
	if err != nil {
		return fmt.Errorf("update pending_book sessions: %w", err)
	}

	// Update sessions where both user and book are mapped to ready.
	_, err = s.db.ExecContext(ctx, `
		UPDATE abs_import_sessions SET status = 'ready'
		WHERE import_id = ?
			AND status NOT IN ('imported', 'skipped')
			AND abs_user_id IN (
				SELECT abs_user_id FROM abs_import_users
				WHERE import_id = ? AND listenup_id IS NOT NULL
			)
			AND abs_media_id IN (
				SELECT abs_media_id FROM abs_import_books
				WHERE import_id = ? AND listenup_id IS NOT NULL
			)`,
		importID, importID, importID)
	if err != nil {
		return fmt.Errorf("update ready sessions: %w", err)
	}

	return nil
}

// GetABSImportStats returns aggregated statistics for an ABS import.
// Returns (mapped, unmapped, ready, imported, err) counts.
func (s *Store) GetABSImportStats(ctx context.Context, importID string) (mapped, unmapped, ready, imported int, err error) {
	// Count mapped entities (users + books that have a ListenUp mapping).
	err = s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM abs_import_users WHERE import_id = ? AND listenup_id IS NOT NULL) +
			(SELECT COUNT(*) FROM abs_import_books WHERE import_id = ? AND listenup_id IS NOT NULL)`,
		importID, importID).Scan(&mapped)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("count mapped: %w", err)
	}

	// Count unmapped entities (users + books without mapping).
	err = s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM abs_import_users WHERE import_id = ? AND listenup_id IS NULL) +
			(SELECT COUNT(*) FROM abs_import_books WHERE import_id = ? AND listenup_id IS NULL)`,
		importID, importID).Scan(&unmapped)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("count unmapped: %w", err)
	}

	// Count ready sessions.
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM abs_import_sessions WHERE import_id = ? AND status = 'ready'`,
		importID).Scan(&ready)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("count ready: %w", err)
	}

	// Count imported sessions.
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM abs_import_sessions WHERE import_id = ? AND status = 'imported'`,
		importID).Scan(&imported)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("count imported: %w", err)
	}

	return mapped, unmapped, ready, imported, nil
}

// --- ABSImportProgress ---

// absImportProgressColumns is the ordered list of columns selected in abs_import_progress queries.
const absImportProgressColumns = `import_id, abs_user_id, abs_media_id,
	current_time, duration, progress, is_finished, finished_at,
	last_update, status, imported_at`

// scanABSImportProgress scans a sql.Row (or sql.Rows via its Scan method) into a domain.ABSImportProgress.
func scanABSImportProgress(scanner interface{ Scan(dest ...any) error }) (*domain.ABSImportProgress, error) {
	var p domain.ABSImportProgress

	var (
		isFinished int
		finishedAt sql.NullString
		lastUpdate string
		status     string
		importedAt sql.NullString
	)

	err := scanner.Scan(
		&p.ImportID,
		&p.ABSUserID,
		&p.ABSMediaID,
		&p.CurrentTime,
		&p.Duration,
		&p.Progress,
		&isFinished,
		&finishedAt,
		&lastUpdate,
		&status,
		&importedAt,
	)
	if err != nil {
		return nil, err
	}

	p.IsFinished = isFinished != 0
	p.Status = domain.SessionImportStatus(status)

	p.FinishedAt, err = parseNullableTime(finishedAt)
	if err != nil {
		return nil, err
	}

	p.LastUpdate, err = parseTime(lastUpdate)
	if err != nil {
		return nil, err
	}

	p.ImportedAt, err = parseNullableTime(importedAt)
	if err != nil {
		return nil, err
	}

	return &p, nil
}

// CreateABSImportProgress inserts a new ABS import progress record.
func (s *Store) CreateABSImportProgress(ctx context.Context, progress *domain.ABSImportProgress) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO abs_import_progress (
			import_id, abs_user_id, abs_media_id,
			current_time, duration, progress, is_finished, finished_at,
			last_update, status, imported_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		progress.ImportID,
		progress.ABSUserID,
		progress.ABSMediaID,
		progress.CurrentTime,
		progress.Duration,
		progress.Progress,
		boolToInt(progress.IsFinished),
		nullTimeString(progress.FinishedAt),
		formatTime(progress.LastUpdate),
		string(progress.Status),
		nullTimeString(progress.ImportedAt),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return store.ErrAlreadyExists
		}
		return err
	}
	return nil
}

// GetABSImportProgress retrieves an ABS import progress entry.
// Returns store.ErrNotFound if the progress entry does not exist.
func (s *Store) GetABSImportProgress(ctx context.Context, importID, absUserID, absMediaID string) (*domain.ABSImportProgress, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+absImportProgressColumns+` FROM abs_import_progress
		WHERE import_id = ? AND abs_user_id = ? AND abs_media_id = ?`,
		importID, absUserID, absMediaID)

	p, err := scanABSImportProgress(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

// ListABSImportProgressForUser returns all progress entries for a user within an import.
func (s *Store) ListABSImportProgressForUser(ctx context.Context, importID, absUserID string) ([]*domain.ABSImportProgress, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+absImportProgressColumns+` FROM abs_import_progress
		WHERE import_id = ? AND abs_user_id = ?
		ORDER BY last_update DESC`,
		importID, absUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var progress []*domain.ABSImportProgress
	for rows.Next() {
		p, err := scanABSImportProgress(rows)
		if err != nil {
			return nil, err
		}
		progress = append(progress, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return progress, nil
}

// FindABSImportProgressByListenUpBook finds an ABS import progress entry
// by matching through the book mapping table.
// Returns store.ErrNotFound if no matching progress entry exists.
func (s *Store) FindABSImportProgressByListenUpBook(ctx context.Context, importID, absUserID, listenUpBookID string) (*domain.ABSImportProgress, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+absImportProgressColumns+` FROM abs_import_progress p
		JOIN abs_import_books b ON b.import_id = p.import_id AND b.abs_media_id = p.abs_media_id
		WHERE p.import_id = ? AND p.abs_user_id = ? AND b.listenup_id = ?`,
		importID, absUserID, listenUpBookID)

	p, err := scanABSImportProgress(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}
