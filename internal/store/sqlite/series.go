package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// seriesColumns is the ordered list of columns selected in series queries.
// Must match the scan order in scanSeries.
const seriesColumns = `id, created_at, updated_at, deleted_at, name, description, asin,
	cover_path, cover_filename, cover_format, cover_size,
	cover_inode, cover_mod_time, cover_blur_hash`

// scanSeries scans a sql.Row (or sql.Rows via its Scan method) into a domain.Series.
func scanSeries(scanner interface{ Scan(dest ...any) error }) (*domain.Series, error) {
	var s domain.Series

	var (
		createdAt     string
		updatedAt     string
		deletedAt     sql.NullString
		description   sql.NullString
		asin          sql.NullString
		coverPath     sql.NullString
		coverFilename sql.NullString
		coverFormat   sql.NullString
		coverSize     sql.NullInt64
		coverInode    sql.NullInt64
		coverModTime  sql.NullInt64
		coverBlurHash sql.NullString
	)

	err := scanner.Scan(
		&s.ID,
		&createdAt,
		&updatedAt,
		&deletedAt,
		&s.Name,
		&description,
		&asin,
		&coverPath,
		&coverFilename,
		&coverFormat,
		&coverSize,
		&coverInode,
		&coverModTime,
		&coverBlurHash,
	)
	if err != nil {
		return nil, err
	}

	// Parse timestamps.
	s.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	s.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	s.DeletedAt, err = parseNullableTime(deletedAt)
	if err != nil {
		return nil, err
	}

	// Optional strings.
	if description.Valid {
		s.Description = description.String
	}
	if asin.Valid {
		s.ASIN = asin.String
	}

	// Cover image: only populate if cover_path is present.
	if coverPath.Valid {
		s.CoverImage = &domain.ImageFileInfo{
			Path:     coverPath.String,
			Filename: coverFilename.String,
			Format:   coverFormat.String,
			BlurHash: coverBlurHash.String,
		}
		if coverSize.Valid {
			s.CoverImage.Size = coverSize.Int64
		}
		if coverInode.Valid {
			s.CoverImage.Inode = uint64(coverInode.Int64)
		}
		if coverModTime.Valid {
			s.CoverImage.ModTime = coverModTime.Int64
		}
	}

	return &s, nil
}

// CreateSeries inserts a new series into the database.
// Returns store.ErrAlreadyExists on duplicate ID.
func (s *Store) CreateSeries(ctx context.Context, series *domain.Series) error {
	coverPath, coverFilename, coverFormat, coverSize, coverInode, coverModTime, coverBlurHash := coverArgs(series.CoverImage)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO series (
			id, created_at, updated_at, deleted_at, name, description, asin,
			cover_path, cover_filename, cover_format, cover_size,
			cover_inode, cover_mod_time, cover_blur_hash
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		series.ID,
		formatTime(series.CreatedAt),
		formatTime(series.UpdatedAt),
		nullTimeString(series.DeletedAt),
		series.Name,
		nullString(series.Description),
		nullString(series.ASIN),
		coverPath,
		coverFilename,
		coverFormat,
		coverSize,
		coverInode,
		coverModTime,
		coverBlurHash,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return store.ErrAlreadyExists
		}
		return err
	}
	return nil
}

// GetSeries retrieves a series by ID, excluding soft-deleted records.
// Returns store.ErrNotFound if the series does not exist.
func (s *Store) GetSeries(ctx context.Context, id string) (*domain.Series, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+seriesColumns+` FROM series WHERE id = ? AND deleted_at IS NULL`, id)

	series, err := scanSeries(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return series, nil
}

// ListSeries returns paginated series ordered by name (case-insensitive) then id.
// Soft-deleted series are excluded.
func (s *Store) ListSeries(ctx context.Context, params store.PaginationParams) (*store.PaginatedResult[*domain.Series], error) {
	params.Validate()

	// Decode cursor into (name, id) pair.
	var cursorName, cursorID string
	if params.Cursor != "" {
		decoded, err := store.DecodeCursor(params.Cursor)
		if err != nil {
			return nil, fmt.Errorf("decode cursor: %w", err)
		}
		parts := strings.SplitN(decoded, "|", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid cursor format")
		}
		cursorName = parts[0]
		cursorID = parts[1]
	}

	// Count total non-deleted series.
	var total int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM series WHERE deleted_at IS NULL`).Scan(&total)
	if err != nil {
		return nil, err
	}

	// Fetch one extra to determine HasMore.
	fetchLimit := params.Limit + 1

	var rows *sql.Rows
	if cursorName != "" || cursorID != "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+seriesColumns+` FROM series
			WHERE deleted_at IS NULL
				AND (name COLLATE NOCASE > ? OR (name COLLATE NOCASE = ? AND id > ?))
			ORDER BY name COLLATE NOCASE ASC, id ASC
			LIMIT ?`,
			cursorName, cursorName, cursorID, fetchLimit)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+seriesColumns+` FROM series
			WHERE deleted_at IS NULL
			ORDER BY name COLLATE NOCASE ASC, id ASC
			LIMIT ?`,
			fetchLimit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*domain.Series
	for rows.Next() {
		series, err := scanSeries(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, series)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := &store.PaginatedResult[*domain.Series]{
		Items: items,
		Total: total,
	}

	// If we got more than limit, there are more pages.
	if len(items) > params.Limit {
		result.HasMore = true
		result.Items = items[:params.Limit]
		last := result.Items[params.Limit-1]
		result.NextCursor = store.EncodeCursor(last.Name + "|" + last.ID)
	}

	// Ensure Items is never nil.
	if result.Items == nil {
		result.Items = []*domain.Series{}
	}

	return result, nil
}

// UpdateSeries performs a full row update on an existing series.
// Returns store.ErrNotFound if the series does not exist or is soft-deleted.
func (s *Store) UpdateSeries(ctx context.Context, series *domain.Series) error {
	coverPath, coverFilename, coverFormat, coverSize, coverInode, coverModTime, coverBlurHash := coverArgs(series.CoverImage)

	result, err := s.db.ExecContext(ctx, `
		UPDATE series SET
			created_at = ?,
			updated_at = ?,
			name = ?,
			description = ?,
			asin = ?,
			cover_path = ?,
			cover_filename = ?,
			cover_format = ?,
			cover_size = ?,
			cover_inode = ?,
			cover_mod_time = ?,
			cover_blur_hash = ?
		WHERE id = ? AND deleted_at IS NULL`,
		formatTime(series.CreatedAt),
		formatTime(series.UpdatedAt),
		series.Name,
		nullString(series.Description),
		nullString(series.ASIN),
		coverPath,
		coverFilename,
		coverFormat,
		coverSize,
		coverInode,
		coverModTime,
		coverBlurHash,
		series.ID,
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
