package sqlite

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/dto"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
)

// bookColumns is the ordered list of columns selected in book queries.
// Must match the scan order in scanBook.
const bookColumns = `id, created_at, updated_at, deleted_at, scanned_at,
	isbn, title, subtitle, path, description, publisher, publish_year,
	language, asin, audible_region,
	total_duration, total_size, abridged,
	cover_path, cover_filename, cover_format, cover_size,
	cover_inode, cover_mod_time, cover_blur_hash,
	staged_collection_ids`

// scanBook scans a sql.Row (or sql.Rows via its Scan method) into a domain.Book.
func scanBook(scanner interface{ Scan(dest ...any) error }) (*domain.Book, error) {
	var b domain.Book

	var (
		createdAt  string
		updatedAt  string
		deletedAt  sql.NullString
		scannedAt  string
		isbn       sql.NullString
		subtitle   sql.NullString
		desc       sql.NullString
		publisher  sql.NullString
		publishYr  sql.NullString
		language   sql.NullString
		asin       sql.NullString
		audibleReg sql.NullString
		abridged   int

		coverPath     sql.NullString
		coverFilename sql.NullString
		coverFormat   sql.NullString
		coverSize     sql.NullInt64
		coverInode    sql.NullInt64
		coverModTime  sql.NullInt64
		coverBlurHash sql.NullString

		stagedCollIDs string
	)

	err := scanner.Scan(
		&b.ID,
		&createdAt,
		&updatedAt,
		&deletedAt,
		&scannedAt,
		&isbn,
		&b.Title,
		&subtitle,
		&b.Path,
		&desc,
		&publisher,
		&publishYr,
		&language,
		&asin,
		&audibleReg,
		&b.TotalDuration,
		&b.TotalSize,
		&abridged,
		&coverPath,
		&coverFilename,
		&coverFormat,
		&coverSize,
		&coverInode,
		&coverModTime,
		&coverBlurHash,
		&stagedCollIDs,
	)
	if err != nil {
		return nil, err
	}

	// Parse timestamps.
	b.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	b.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	b.DeletedAt, err = parseNullableTime(deletedAt)
	if err != nil {
		return nil, err
	}
	b.ScannedAt, err = parseTime(scannedAt)
	if err != nil {
		return nil, err
	}

	// Optional string fields.
	if isbn.Valid {
		b.ISBN = isbn.String
	}
	if subtitle.Valid {
		b.Subtitle = subtitle.String
	}
	if desc.Valid {
		b.Description = desc.String
	}
	if publisher.Valid {
		b.Publisher = publisher.String
	}
	if publishYr.Valid {
		b.PublishYear = publishYr.String
	}
	if language.Valid {
		b.Language = language.String
	}
	if asin.Valid {
		b.ASIN = asin.String
	}
	if audibleReg.Valid {
		b.AudibleRegion = audibleReg.String
	}

	// Boolean fields.
	b.Abridged = abridged != 0

	// Cover image - only set if cover_path is present.
	if coverPath.Valid {
		b.CoverImage = &domain.ImageFileInfo{
			Path:     coverPath.String,
			Filename: coverFilename.String,
			Format:   coverFormat.String,
			Size:     coverSize.Int64,
			Inode:    uint64(coverInode.Int64),
			ModTime:  coverModTime.Int64,
			BlurHash: coverBlurHash.String,
		}
	}

	// Parse staged_collection_ids JSON array.
	if err := json.Unmarshal([]byte(stagedCollIDs), &b.StagedCollectionIDs); err != nil {
		return nil, fmt.Errorf("unmarshal staged_collection_ids: %w", err)
	}

	return &b, nil
}

// loadBookAudioFiles loads audio files for a book from the book_audio_files table.
func (s *Store) loadBookAudioFiles(ctx context.Context, querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}, bookID string) ([]domain.AudioFileInfo, error) {
	rows, err := querier.QueryContext(ctx, `
		SELECT id, path, filename, format, codec, size, duration, bitrate, inode, mod_time
		FROM book_audio_files
		WHERE book_id = ?
		ORDER BY sort_order ASC`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []domain.AudioFileInfo
	for rows.Next() {
		var af domain.AudioFileInfo
		var codec sql.NullString
		var bitrate sql.NullInt64
		err := rows.Scan(
			&af.ID, &af.Path, &af.Filename, &af.Format, &codec,
			&af.Size, &af.Duration, &bitrate, &af.Inode, &af.ModTime,
		)
		if err != nil {
			return nil, err
		}
		if codec.Valid {
			af.Codec = codec.String
		}
		if bitrate.Valid {
			af.Bitrate = int(bitrate.Int64)
		}
		files = append(files, af)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return files, nil
}

// loadBookChapters loads chapters for a book from the book_chapters table.
func (s *Store) loadBookChapters(ctx context.Context, querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}, bookID string) ([]domain.Chapter, error) {
	rows, err := querier.QueryContext(ctx, `
		SELECT idx, title, audio_file_id, start_time, end_time
		FROM book_chapters
		WHERE book_id = ?
		ORDER BY idx ASC`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chapters []domain.Chapter
	for rows.Next() {
		var ch domain.Chapter
		var audioFileID sql.NullString
		err := rows.Scan(&ch.Index, &ch.Title, &audioFileID, &ch.StartTime, &ch.EndTime)
		if err != nil {
			return nil, err
		}
		if audioFileID.Valid {
			ch.AudioFileID = audioFileID.String
		}
		chapters = append(chapters, ch)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return chapters, nil
}

// insertBookAudioFiles inserts audio files for a book within a transaction.
func insertBookAudioFiles(ctx context.Context, tx *sql.Tx, bookID string, files []domain.AudioFileInfo) error {
	for i, af := range files {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO book_audio_files (
				id, book_id, path, filename, format, codec,
				size, duration, bitrate, inode, mod_time, sort_order
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			af.ID, bookID, af.Path, af.Filename, af.Format, nullString(af.Codec),
			af.Size, af.Duration, nullInt64(int64(af.Bitrate)),
			int64(af.Inode), af.ModTime, i,
		)
		if err != nil {
			return fmt.Errorf("insert audio file %s: %w", af.ID, err)
		}
	}
	return nil
}

// insertBookChapters inserts chapters for a book within a transaction.
func insertBookChapters(ctx context.Context, tx *sql.Tx, bookID string, chapters []domain.Chapter) error {
	for _, ch := range chapters {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO book_chapters (
				book_id, idx, title, audio_file_id, start_time, end_time
			) VALUES (?, ?, ?, ?, ?, ?)`,
			bookID, ch.Index, ch.Title, nullString(ch.AudioFileID),
			ch.StartTime, ch.EndTime,
		)
		if err != nil {
			return fmt.Errorf("insert chapter %d: %w", ch.Index, err)
		}
	}
	return nil
}

// coverArgs returns the SQL arguments for cover image columns.
func coverArgs(img *domain.ImageFileInfo) (coverPath, coverFilename, coverFormat sql.NullString, coverSize, coverInode, coverModTime sql.NullInt64, coverBlurHash sql.NullString) {
	if img == nil {
		return
	}
	coverPath = sql.NullString{String: img.Path, Valid: true}
	coverFilename = sql.NullString{String: img.Filename, Valid: true}
	coverFormat = sql.NullString{String: img.Format, Valid: true}
	coverSize = sql.NullInt64{Int64: img.Size, Valid: true}
	coverInode = sql.NullInt64{Int64: int64(img.Inode), Valid: true}
	coverModTime = sql.NullInt64{Int64: img.ModTime, Valid: true}
	coverBlurHash = sql.NullString{String: img.BlurHash, Valid: true}
	return
}

// CreateBook inserts a book row along with its audio files and chapters in a transaction.
// Returns store.ErrAlreadyExists on duplicate ID or path.
func (s *Store) CreateBook(ctx context.Context, book *domain.Book) error {
	stagedJSON, err := json.Marshal(book.StagedCollectionIDs)
	if err != nil {
		return fmt.Errorf("marshal staged_collection_ids: %w", err)
	}

	coverPath, coverFilename, coverFormat, coverSize, coverInode, coverModTime, coverBlurHash := coverArgs(book.CoverImage)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO books (
			id, created_at, updated_at, deleted_at, scanned_at,
			isbn, title, subtitle, path, description, publisher, publish_year,
			language, asin, audible_region,
			total_duration, total_size, abridged,
			cover_path, cover_filename, cover_format, cover_size,
			cover_inode, cover_mod_time, cover_blur_hash,
			staged_collection_ids
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		book.ID,
		formatTime(book.CreatedAt),
		formatTime(book.UpdatedAt),
		nullTimeString(book.DeletedAt),
		formatTime(book.ScannedAt),
		nullString(book.ISBN),
		book.Title,
		nullString(book.Subtitle),
		book.Path,
		nullString(book.Description),
		nullString(book.Publisher),
		nullString(book.PublishYear),
		nullString(book.Language),
		nullString(book.ASIN),
		nullString(book.AudibleRegion),
		book.TotalDuration,
		book.TotalSize,
		boolToInt(book.Abridged),
		coverPath, coverFilename, coverFormat, coverSize,
		coverInode, coverModTime, coverBlurHash,
		string(stagedJSON),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return store.ErrAlreadyExists
		}
		return err
	}

	if err := insertBookAudioFiles(ctx, tx, book.ID, book.AudioFiles); err != nil {
		return err
	}

	if err := insertBookChapters(ctx, tx, book.ID, book.Chapters); err != nil {
		return err
	}

	return tx.Commit()
}

// GetBook retrieves a book by ID, excluding soft-deleted records.
// Audio files and chapters are loaded. Contributors and Series are NOT loaded.
// The userID parameter is accepted for interface compatibility but not used for access checks.
// Returns store.ErrNotFound if the book does not exist.
func (s *Store) GetBook(ctx context.Context, id string, _ string) (*domain.Book, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+bookColumns+` FROM books WHERE id = ? AND deleted_at IS NULL`, id)

	b, err := scanBook(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	b.AudioFiles, err = s.loadBookAudioFiles(ctx, s.db, id)
	if err != nil {
		return nil, fmt.Errorf("load audio files: %w", err)
	}

	b.Chapters, err = s.loadBookChapters(ctx, s.db, id)
	if err != nil {
		return nil, fmt.Errorf("load chapters: %w", err)
	}

	b.GenreIDs, err = s.GetGenreIDsForBook(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load genre IDs: %w", err)
	}

	return b, nil
}

// GetBookByPath retrieves a book by its file path, excluding soft-deleted records.
// Returns store.ErrNotFound if the book does not exist.
func (s *Store) GetBookByPath(ctx context.Context, path string) (*domain.Book, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+bookColumns+` FROM books WHERE path = ? AND deleted_at IS NULL`, path)

	b, err := scanBook(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	b.AudioFiles, err = s.loadBookAudioFiles(ctx, s.db, b.ID)
	if err != nil {
		return nil, fmt.Errorf("load audio files: %w", err)
	}

	b.Chapters, err = s.loadBookChapters(ctx, s.db, b.ID)
	if err != nil {
		return nil, fmt.Errorf("load chapters: %w", err)
	}

	return b, nil
}

// ListBooks returns a paginated list of non-deleted books ordered by updated_at, id.
// Audio files and chapters are loaded for each book.
func (s *Store) ListBooks(ctx context.Context, params store.PaginationParams) (*store.PaginatedResult[*domain.Book], error) {
	params.Validate()

	// Decode cursor: format is "updated_at|id".
	var cursorTime, cursorID string
	if params.Cursor != "" {
		decoded, err := store.DecodeCursor(params.Cursor)
		if err != nil {
			return nil, fmt.Errorf("decode cursor: %w", err)
		}
		parts := strings.SplitN(decoded, "|", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid cursor format")
		}
		cursorTime = parts[0]
		cursorID = parts[1]
	}

	// Count total non-deleted books.
	var total int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM books WHERE deleted_at IS NULL`).Scan(&total)
	if err != nil {
		return nil, err
	}

	// Fetch limit+1 rows to determine hasMore.
	var rows *sql.Rows
	if cursorTime == "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+bookColumns+` FROM books
			WHERE deleted_at IS NULL
			ORDER BY updated_at ASC, id ASC
			LIMIT ?`, params.Limit+1)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+bookColumns+` FROM books
			WHERE deleted_at IS NULL
			AND (updated_at > ? OR (updated_at = ? AND id > ?))
			ORDER BY updated_at ASC, id ASC
			LIMIT ?`, cursorTime, cursorTime, cursorID, params.Limit+1)
	}
	if err != nil {
		return nil, err
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

	// Determine pagination.
	hasMore := len(books) > params.Limit
	if hasMore {
		books = books[:params.Limit]
	}

	// Load audio files and chapters for each book.
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

	// Build next cursor.
	var nextCursor string
	if hasMore && len(books) > 0 {
		last := books[len(books)-1]
		nextCursor = store.EncodeCursor(formatTime(last.UpdatedAt) + "|" + last.ID)
	}

	return &store.PaginatedResult[*domain.Book]{
		Items:      books,
		Total:      total,
		HasMore:    hasMore,
		NextCursor: nextCursor,
	}, nil
}

// UpdateBook updates a book row and replaces its audio files and chapters in a transaction.
// Returns store.ErrNotFound if the book does not exist or is soft-deleted.
func (s *Store) UpdateBook(ctx context.Context, book *domain.Book) error {
	stagedJSON, err := json.Marshal(book.StagedCollectionIDs)
	if err != nil {
		return fmt.Errorf("marshal staged_collection_ids: %w", err)
	}

	coverPath, coverFilename, coverFormat, coverSize, coverInode, coverModTime, coverBlurHash := coverArgs(book.CoverImage)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		UPDATE books SET
			created_at = ?, updated_at = ?, scanned_at = ?,
			isbn = ?, title = ?, subtitle = ?, path = ?,
			description = ?, publisher = ?, publish_year = ?,
			language = ?, asin = ?, audible_region = ?,
			total_duration = ?, total_size = ?, abridged = ?,
			cover_path = ?, cover_filename = ?, cover_format = ?, cover_size = ?,
			cover_inode = ?, cover_mod_time = ?, cover_blur_hash = ?,
			staged_collection_ids = ?
		WHERE id = ? AND deleted_at IS NULL`,
		formatTime(book.CreatedAt),
		formatTime(book.UpdatedAt),
		formatTime(book.ScannedAt),
		nullString(book.ISBN),
		book.Title,
		nullString(book.Subtitle),
		book.Path,
		nullString(book.Description),
		nullString(book.Publisher),
		nullString(book.PublishYear),
		nullString(book.Language),
		nullString(book.ASIN),
		nullString(book.AudibleRegion),
		book.TotalDuration,
		book.TotalSize,
		boolToInt(book.Abridged),
		coverPath, coverFilename, coverFormat, coverSize,
		coverInode, coverModTime, coverBlurHash,
		string(stagedJSON),
		book.ID,
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

	// Replace audio files: delete existing, then re-insert.
	if _, err := tx.ExecContext(ctx, `DELETE FROM book_audio_files WHERE book_id = ?`, book.ID); err != nil {
		return err
	}
	if err := insertBookAudioFiles(ctx, tx, book.ID, book.AudioFiles); err != nil {
		return err
	}

	// Replace chapters: delete existing, then re-insert.
	if _, err := tx.ExecContext(ctx, `DELETE FROM book_chapters WHERE book_id = ?`, book.ID); err != nil {
		return err
	}
	if err := insertBookChapters(ctx, tx, book.ID, book.Chapters); err != nil {
		return err
	}

	return tx.Commit()
}

// DeleteBook performs a soft delete by setting deleted_at and updated_at.
// Returns store.ErrNotFound if the book does not exist or is already deleted.
func (s *Store) DeleteBook(ctx context.Context, id string) error {
	now := formatTime(time.Now())

	result, err := s.db.ExecContext(ctx, `
		UPDATE books SET deleted_at = ?, updated_at = ?
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

// GetBooksForSync returns all non-deleted books with updated_at after the given timestamp.
// Audio files and chapters are loaded for each book.
func (s *Store) GetBooksForSync(ctx context.Context, updatedAfter time.Time) ([]*domain.Book, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+bookColumns+` FROM books
		WHERE updated_at > ? AND deleted_at IS NULL
		ORDER BY updated_at ASC`,
		formatTime(updatedAfter))
	if err != nil {
		return nil, err
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

	// Load audio files and chapters for each book.
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

// GetBooksDeletedAfter returns the IDs of books where deleted_at is after the given timestamp.
func (s *Store) GetBooksDeletedAfter(ctx context.Context, timestamp time.Time) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM books WHERE deleted_at > ?`,
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

// GetBookNoAccessCheck retrieves a book by ID without access control.
// Identical to GetBook since the SQLite implementation does not perform access checks.
func (s *Store) GetBookNoAccessCheck(ctx context.Context, id string) (*domain.Book, error) {
	return s.GetBook(ctx, id, "")
}

// GetBookByInode retrieves a book by an audio file inode.
// This looks up the inode in book_audio_files to find the book_id, then loads the full book.
// Returns store.ErrNotFound if no audio file with this inode exists.
func (s *Store) GetBookByInode(ctx context.Context, inode int64) (*domain.Book, error) {
	var bookID string
	err := s.db.QueryRowContext(ctx,
		`SELECT book_id FROM book_audio_files WHERE inode = ? LIMIT 1`, inode).Scan(&bookID)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get book by inode: %w", err)
	}
	return s.GetBookNoAccessCheck(ctx, bookID)
}

// GetBookByASIN retrieves a book by its Amazon ASIN.
// Returns store.ErrNotFound if no non-deleted book with this ASIN exists.
func (s *Store) GetBookByASIN(ctx context.Context, asin string) (*domain.Book, error) {
	if asin == "" {
		return nil, store.ErrNotFound
	}

	row := s.db.QueryRowContext(ctx,
		`SELECT `+bookColumns+` FROM books WHERE asin = ? AND deleted_at IS NULL`, asin)

	b, err := scanBook(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	b.AudioFiles, err = s.loadBookAudioFiles(ctx, s.db, b.ID)
	if err != nil {
		return nil, fmt.Errorf("load audio files: %w", err)
	}

	b.Chapters, err = s.loadBookChapters(ctx, s.db, b.ID)
	if err != nil {
		return nil, fmt.Errorf("load chapters: %w", err)
	}

	return b, nil
}

// GetBookByISBN retrieves a book by its ISBN.
// Returns store.ErrNotFound if no non-deleted book with this ISBN exists.
func (s *Store) GetBookByISBN(ctx context.Context, isbn string) (*domain.Book, error) {
	if isbn == "" {
		return nil, store.ErrNotFound
	}

	row := s.db.QueryRowContext(ctx,
		`SELECT `+bookColumns+` FROM books WHERE isbn = ? AND deleted_at IS NULL`, isbn)

	b, err := scanBook(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	b.AudioFiles, err = s.loadBookAudioFiles(ctx, s.db, b.ID)
	if err != nil {
		return nil, fmt.Errorf("load audio files: %w", err)
	}

	b.Chapters, err = s.loadBookChapters(ctx, s.db, b.ID)
	if err != nil {
		return nil, fmt.Errorf("load chapters: %w", err)
	}

	return b, nil
}

// BookExists checks if a non-deleted book exists by ID.
func (s *Store) BookExists(ctx context.Context, id string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM books WHERE id = ? AND deleted_at IS NULL`, id).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ListAllBooks returns all non-deleted books without pagination.
// Audio files and chapters are loaded for each book.
func (s *Store) ListAllBooks(ctx context.Context) ([]*domain.Book, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+bookColumns+` FROM books WHERE deleted_at IS NULL ORDER BY updated_at ASC`)
	if err != nil {
		return nil, err
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

	// Load audio files and chapters for each book.
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

// CountBooks returns the total number of non-deleted books.
func (s *Store) CountBooks(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM books WHERE deleted_at IS NULL`).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// GetAllBookIDs returns all non-deleted book IDs.
func (s *Store) GetAllBookIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM books WHERE deleted_at IS NULL`)
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

// GetBooksByCollectionPaginated returns a paginated list of books in a collection.
// Books are fetched via the collection_books join table.
func (s *Store) GetBooksByCollectionPaginated(ctx context.Context, _, collectionID string, params store.PaginationParams) (*store.PaginatedResult[*domain.Book], error) {
	params.Validate()

	// Count total books in collection.
	var total int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM collection_books cb
		JOIN books b ON b.id = cb.book_id
		WHERE cb.collection_id = ? AND b.deleted_at IS NULL`,
		collectionID).Scan(&total)
	if err != nil {
		return nil, err
	}

	// Decode cursor: index-based offset.
	startIdx := 0
	if params.Cursor != "" {
		decoded, err := store.DecodeCursor(params.Cursor)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor: %w", err)
		}
		idx, err := strconv.Atoi(decoded)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor format: %w", err)
		}
		startIdx = idx
	}

	// Fetch books via join with offset.
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+bookColumnsAliased+`
		FROM collection_books cb
		JOIN books b ON b.id = cb.book_id
		WHERE cb.collection_id = ? AND b.deleted_at IS NULL
		ORDER BY b.title COLLATE NOCASE ASC
		LIMIT ? OFFSET ?`,
		collectionID, params.Limit+1, startIdx)
	if err != nil {
		return nil, err
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

	hasMore := len(books) > params.Limit
	if hasMore {
		books = books[:params.Limit]
	}

	// Load audio files and chapters for each book.
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

	result := &store.PaginatedResult[*domain.Book]{
		Items:   books,
		Total:   total,
		HasMore: hasMore,
	}

	if hasMore {
		result.NextCursor = store.EncodeCursor(strconv.Itoa(startIdx + params.Limit))
	}

	return result, nil
}

// SearchBooksByTitle finds non-deleted books where the title contains the query (case-insensitive).
// Returns up to 100 candidates. Audio files and chapters are loaded.
func (s *Store) SearchBooksByTitle(ctx context.Context, title string) ([]*domain.Book, error) {
	if title == "" {
		return nil, nil
	}

	query := "%" + strings.ToLower(strings.TrimSpace(title)) + "%"

	rows, err := s.db.QueryContext(ctx,
		`SELECT `+bookColumns+` FROM books
		WHERE deleted_at IS NULL AND LOWER(title) LIKE ?
		LIMIT 100`, query)
	if err != nil {
		return nil, err
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

	// Load audio files and chapters for each book.
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

// TouchEntity updates the updated_at timestamp for an entity (book, contributor, or series).
func (s *Store) TouchEntity(ctx context.Context, entityType, id string) error {
	now := formatTime(time.Now())

	var query string
	switch entityType {
	case "book":
		query = `UPDATE books SET updated_at = ? WHERE id = ? AND deleted_at IS NULL`
	case "contributor":
		query = `UPDATE contributors SET updated_at = ? WHERE id = ? AND deleted_at IS NULL`
	case "series":
		query = `UPDATE series SET updated_at = ? WHERE id = ? AND deleted_at IS NULL`
	default:
		return fmt.Errorf("unknown entity type: %s", entityType)
	}

	result, err := s.db.ExecContext(ctx, query, now, id)
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

// EnrichBook denormalizes a book with contributor names, series name, and genre names.
// This is a convenience wrapper around the enricher for API handlers.
func (s *Store) EnrichBook(ctx context.Context, book *domain.Book) (*dto.Book, error) {
	return s.enricher.EnrichBook(ctx, book)
}

// SetBookContributors replaces all contributors for a book using store.ContributorInput.
// For each contributor:
//   - If name matches existing (case-insensitive) -> link to that contributor
//   - Else -> create new contributor and link
//
// Returns the updated book.
func (s *Store) SetBookContributors(ctx context.Context, bookID string, contributors []store.ContributorInput) (*domain.Book, error) {
	// Build domain.BookContributor list by resolving names to IDs.
	bookContributors := make([]domain.BookContributor, 0, len(contributors))

	for _, input := range contributors {
		// Get or create the contributor by name.
		contributor, err := s.GetOrCreateContributor(ctx, input.Name)
		if err != nil {
			return nil, fmt.Errorf("get or create contributor %q: %w", input.Name, err)
		}

		bookContributors = append(bookContributors, domain.BookContributor{
			ContributorID: contributor.ID,
			Roles:         input.Roles,
		})
	}

	// Replace all book_contributors rows.
	if err := s.setBookContributorsInternal(ctx, bookID, bookContributors); err != nil {
		return nil, fmt.Errorf("set book contributors: %w", err)
	}

	// Touch book updated_at.
	if err := s.TouchEntity(ctx, "book", bookID); err != nil {
		return nil, fmt.Errorf("touch book: %w", err)
	}

	return s.GetBookNoAccessCheck(ctx, bookID)
}

// SetBookSeries replaces all series for a book using store.SeriesInput.
// For each series:
//   - If name matches existing (case-insensitive) -> link to that series
//   - Else -> create new series and link
//
// Returns the updated book.
func (s *Store) SetBookSeries(ctx context.Context, bookID string, seriesInputs []store.SeriesInput) (*domain.Book, error) {
	// Build domain.BookSeries list by resolving names to IDs.
	bookSeries := make([]domain.BookSeries, 0, len(seriesInputs))

	for _, input := range seriesInputs {
		// Get or create the series by name.
		series, err := s.GetOrCreateSeries(ctx, input.Name)
		if err != nil {
			return nil, fmt.Errorf("get or create series %q: %w", input.Name, err)
		}

		bookSeries = append(bookSeries, domain.BookSeries{
			SeriesID: series.ID,
			Sequence: input.Sequence,
		})
	}

	// Replace all book_series rows.
	if err := s.setBookSeriesInternal(ctx, bookID, bookSeries); err != nil {
		return nil, fmt.Errorf("set book series: %w", err)
	}

	// Touch book updated_at.
	if err := s.TouchEntity(ctx, "book", bookID); err != nil {
		return nil, fmt.Errorf("touch book: %w", err)
	}

	return s.GetBookNoAccessCheck(ctx, bookID)
}

// BroadcastBookCreated enriches and broadcasts a book.created SSE event.
// Should be called AFTER cover extraction to ensure cover is available when clients receive event.
func (s *Store) BroadcastBookCreated(ctx context.Context, book *domain.Book) error {
	enrichedBook, err := s.enricher.EnrichBook(ctx, book)
	if err != nil {
		// Don't fail broadcasting if enrichment fails.
		if s.logger != nil {
			s.logger.Warn("failed to enrich book for SSE event",
				"book_id", book.ID,
				"error", err,
			)
		}
		// Fallback: wrap domain.Book in dto.Book without enrichment
		enrichedBook = &dto.Book{Book: book}
	}

	s.emitter.Emit(sse.NewBookCreatedEvent(enrichedBook))
	return nil
}
