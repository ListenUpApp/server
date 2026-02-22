package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
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

// GetOrCreateSeries finds an existing series by name (case-insensitive),
// or creates a new one if not found. Returns the found or newly created series.
func (s *Store) GetOrCreateSeries(ctx context.Context, name string) (*domain.Series, error) {
	// Try to find by case-insensitive name match.
	row := s.db.QueryRowContext(ctx,
		`SELECT `+seriesColumns+` FROM series WHERE LOWER(name) = LOWER(?) AND deleted_at IS NULL`, name)

	series, err := scanSeries(row)
	if err == nil {
		return series, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	// Not found - create new series.
	now := time.Now()
	series = &domain.Series{
		Syncable: domain.Syncable{
			ID:        uuid.New().String(),
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: name,
	}

	if err := s.CreateSeries(ctx, series); err != nil {
		return nil, err
	}
	return series, nil
}

// GetSeriesByIDs retrieves multiple series by their IDs, excluding soft-deleted records.
// Returns only found series (no error for missing IDs).
func (s *Store) GetSeriesByIDs(ctx context.Context, ids []string) ([]*domain.Series, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `SELECT ` + seriesColumns + ` FROM series
		WHERE id IN (` + strings.Join(placeholders, ",") + `) AND deleted_at IS NULL`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query series by IDs: %w", err)
	}
	defer rows.Close()

	var result []*domain.Series
	for rows.Next() {
		sr, err := scanSeries(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, sr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// GetSeriesByASIN retrieves a series by its ASIN, excluding soft-deleted records.
// Returns store.ErrNotFound if no series with that ASIN exists.
func (s *Store) GetSeriesByASIN(ctx context.Context, asin string) (*domain.Series, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+seriesColumns+` FROM series WHERE asin = ? AND deleted_at IS NULL`, asin)

	sr, err := scanSeries(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return sr, nil
}

// GetOrCreateSeriesByName is an alias for GetOrCreateSeries.
func (s *Store) GetOrCreateSeriesByName(ctx context.Context, name string) (*domain.Series, error) {
	return s.GetOrCreateSeries(ctx, name)
}

// DeleteSeries performs a soft delete by setting deleted_at and updated_at.
// Returns store.ErrNotFound if the series does not exist or is already soft-deleted.
func (s *Store) DeleteSeries(ctx context.Context, id string) error {
	now := formatTime(time.Now())

	result, err := s.db.ExecContext(ctx, `
		UPDATE series SET deleted_at = ?, updated_at = ?
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

// ListAllSeries returns all non-deleted series without pagination.
func (s *Store) ListAllSeries(ctx context.Context) ([]*domain.Series, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+seriesColumns+` FROM series WHERE deleted_at IS NULL ORDER BY name COLLATE NOCASE ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*domain.Series
	for rows.Next() {
		sr, err := scanSeries(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, sr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// CountSeries returns the number of non-deleted series.
func (s *Store) CountSeries(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM series WHERE deleted_at IS NULL`).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// CountBooksInSeries returns the number of non-deleted books in a series
// via the book_series join table.
func (s *Store) CountBooksInSeries(ctx context.Context, seriesID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM book_series bs
		JOIN books b ON b.id = bs.book_id
		WHERE bs.series_id = ? AND b.deleted_at IS NULL`,
		seriesID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// CountBooksForMultipleSeries returns a map of series ID to book count
// for the given series IDs. Only counts non-deleted books.
func (s *Store) CountBooksForMultipleSeries(ctx context.Context, seriesIDs []string) (map[string]int, error) {
	if len(seriesIDs) == 0 {
		return make(map[string]int), nil
	}

	placeholders := make([]string, len(seriesIDs))
	args := make([]any, len(seriesIDs))
	for i, id := range seriesIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT bs.series_id, COUNT(*) as cnt
		FROM book_series bs
		JOIN books b ON b.id = bs.book_id
		WHERE bs.series_id IN (`+strings.Join(placeholders, ",")+`) AND b.deleted_at IS NULL
		GROUP BY bs.series_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var id string
		var cnt int
		if err := rows.Scan(&id, &cnt); err != nil {
			return nil, err
		}
		counts[id] = cnt
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return counts, nil
}

// GetBooksBySeries returns all non-deleted books in a series.
// Audio files and chapters are loaded for each book.
func (s *Store) GetBooksBySeries(ctx context.Context, seriesID string) ([]*domain.Book, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+bookColumnsAliased+`
		FROM books b
		JOIN book_series bs ON bs.book_id = b.id
		WHERE bs.series_id = ? AND b.deleted_at IS NULL
		ORDER BY bs.sequence ASC, b.title COLLATE NOCASE ASC`,
		seriesID)
	if err != nil {
		return nil, fmt.Errorf("query books by series: %w", err)
	}
	defer rows.Close()

	var books []*domain.Book
	for rows.Next() {
		b, err := scanBook(rows)
		if err != nil {
			return nil, err
		}
		books = append(books, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, b := range books {
		b.AudioFiles, err = s.loadBookAudioFiles(ctx, s.db, b.ID)
		if err != nil {
			return nil, fmt.Errorf("load audio files for %s: %w", b.ID, err)
		}
		b.Chapters, err = s.loadBookChapters(ctx, s.db, b.ID)
		if err != nil {
			return nil, fmt.Errorf("load chapters for %s: %w", b.ID, err)
		}
	}

	return books, nil
}

// GetBookIDsBySeries returns just the IDs of non-deleted books in a series.
func (s *Store) GetBookIDsBySeries(ctx context.Context, seriesID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT bs.book_id FROM book_series bs
		JOIN books b ON b.id = bs.book_id
		WHERE bs.series_id = ? AND b.deleted_at IS NULL
		ORDER BY bs.sequence ASC`,
		seriesID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

// GetSeriesUpdatedAfter returns all non-deleted series updated after the given timestamp.
func (s *Store) GetSeriesUpdatedAfter(ctx context.Context, timestamp time.Time) ([]*domain.Series, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+seriesColumns+` FROM series
		WHERE updated_at > ? AND deleted_at IS NULL
		ORDER BY updated_at ASC`,
		formatTime(timestamp))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*domain.Series
	for rows.Next() {
		sr, err := scanSeries(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, sr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// GetSeriesDeletedAfter returns the IDs of series soft-deleted after the given timestamp.
func (s *Store) GetSeriesDeletedAfter(ctx context.Context, timestamp time.Time) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM series WHERE deleted_at > ?`,
		formatTime(timestamp))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}
