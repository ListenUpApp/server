package sqlite

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// contributorColumns is the ordered list of columns selected in contributor queries.
// Must match the scan order in scanContributor.
const contributorColumns = `id, created_at, updated_at, deleted_at, name, sort_name, biography,
	image_url, image_blur_hash, asin, aliases, website, birth_date, death_date`

// scanContributor scans a sql.Row (or sql.Rows via its Scan method) into a domain.Contributor.
func scanContributor(scanner interface{ Scan(dest ...any) error }) (*domain.Contributor, error) {
	var c domain.Contributor

	var (
		createdAt     string
		updatedAt     string
		deletedAt     sql.NullString
		sortName      sql.NullString
		biography     sql.NullString
		imageURL      sql.NullString
		imageBlurHash sql.NullString
		asin          sql.NullString
		aliasesJSON   string
		website       sql.NullString
		birthDate     sql.NullString
		deathDate     sql.NullString
	)

	err := scanner.Scan(
		&c.ID,
		&createdAt,
		&updatedAt,
		&deletedAt,
		&c.Name,
		&sortName,
		&biography,
		&imageURL,
		&imageBlurHash,
		&asin,
		&aliasesJSON,
		&website,
		&birthDate,
		&deathDate,
	)
	if err != nil {
		return nil, err
	}

	// Parse timestamps.
	c.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	c.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	c.DeletedAt, err = parseNullableTime(deletedAt)
	if err != nil {
		return nil, err
	}

	// Optional string fields.
	if sortName.Valid {
		c.SortName = sortName.String
	}
	if biography.Valid {
		c.Biography = biography.String
	}
	if imageURL.Valid {
		c.ImageURL = imageURL.String
	}
	if imageBlurHash.Valid {
		c.ImageBlurHash = imageBlurHash.String
	}
	if asin.Valid {
		c.ASIN = asin.String
	}
	if website.Valid {
		c.Website = website.String
	}
	if birthDate.Valid {
		c.BirthDate = birthDate.String
	}
	if deathDate.Valid {
		c.DeathDate = deathDate.String
	}

	// Parse aliases JSON array.
	if err := json.Unmarshal([]byte(aliasesJSON), &c.Aliases); err != nil {
		return nil, err
	}

	return &c, nil
}

// CreateContributor inserts a new contributor into the database.
// Returns store.ErrAlreadyExists on duplicate ID.
func (s *Store) CreateContributor(ctx context.Context, c *domain.Contributor) error {
	aliasesJSON, err := json.Marshal(c.Aliases)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO contributors (
			id, created_at, updated_at, deleted_at, name, sort_name, biography,
			image_url, image_blur_hash, asin, aliases, website, birth_date, death_date
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID,
		formatTime(c.CreatedAt),
		formatTime(c.UpdatedAt),
		nullTimeString(c.DeletedAt),
		c.Name,
		nullString(c.SortName),
		nullString(c.Biography),
		nullString(c.ImageURL),
		nullString(c.ImageBlurHash),
		nullString(c.ASIN),
		string(aliasesJSON),
		nullString(c.Website),
		nullString(c.BirthDate),
		nullString(c.DeathDate),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return store.ErrAlreadyExists
		}
		return err
	}
	return nil
}

// GetContributor retrieves a contributor by ID, excluding soft-deleted records.
// Returns store.ErrNotFound if the contributor does not exist.
func (s *Store) GetContributor(ctx context.Context, id string) (*domain.Contributor, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+contributorColumns+` FROM contributors WHERE id = ? AND deleted_at IS NULL`, id)

	c, err := scanContributor(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

// ListContributors returns a paginated list of non-deleted contributors,
// ordered by sort_name (case-insensitive) then id.
func (s *Store) ListContributors(ctx context.Context, params store.PaginationParams) (*store.PaginatedResult[*domain.Contributor], error) {
	params.Validate()

	// Count total non-deleted contributors.
	var total int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM contributors WHERE deleted_at IS NULL`).Scan(&total)
	if err != nil {
		return nil, err
	}

	// Build query with optional cursor.
	query := `SELECT ` + contributorColumns + ` FROM contributors WHERE deleted_at IS NULL`
	var args []any

	if params.Cursor != "" {
		cursorKey, err := store.DecodeCursor(params.Cursor)
		if err != nil {
			return nil, err
		}
		// Cursor format: "sort_name|id"
		parts := strings.SplitN(cursorKey, "|", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid cursor format")
		}
		cursorSortName := parts[0]
		cursorID := parts[1]

		query += ` AND (sort_name COLLATE NOCASE > ? OR (sort_name COLLATE NOCASE = ? AND id > ?))`
		args = append(args, cursorSortName, cursorSortName, cursorID)
	}

	query += ` ORDER BY sort_name COLLATE NOCASE ASC, id ASC LIMIT ?`
	args = append(args, params.Limit+1)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*domain.Contributor
	for rows.Next() {
		c, err := scanContributor(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	hasMore := len(items) > params.Limit
	if hasMore {
		items = items[:params.Limit]
	}

	var nextCursor string
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		nextCursor = store.EncodeCursor(last.SortName + "|" + last.ID)
	}

	return &store.PaginatedResult[*domain.Contributor]{
		Items:      items,
		Total:      total,
		HasMore:    hasMore,
		NextCursor: nextCursor,
	}, nil
}

// UpdateContributor performs a full row update on an existing contributor.
// Returns store.ErrNotFound if the contributor does not exist or is soft-deleted.
func (s *Store) UpdateContributor(ctx context.Context, c *domain.Contributor) error {
	aliasesJSON, err := json.Marshal(c.Aliases)
	if err != nil {
		return err
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE contributors SET
			created_at = ?,
			updated_at = ?,
			name = ?,
			sort_name = ?,
			biography = ?,
			image_url = ?,
			image_blur_hash = ?,
			asin = ?,
			aliases = ?,
			website = ?,
			birth_date = ?,
			death_date = ?
		WHERE id = ? AND deleted_at IS NULL`,
		formatTime(c.CreatedAt),
		formatTime(c.UpdatedAt),
		c.Name,
		nullString(c.SortName),
		nullString(c.Biography),
		nullString(c.ImageURL),
		nullString(c.ImageBlurHash),
		nullString(c.ASIN),
		string(aliasesJSON),
		nullString(c.Website),
		nullString(c.BirthDate),
		nullString(c.DeathDate),
		c.ID,
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

// GetOrCreateContributor finds an existing contributor by name (case-insensitive),
// or creates a new one if not found. Returns the found or newly created contributor.
func (s *Store) GetOrCreateContributor(ctx context.Context, name string) (*domain.Contributor, error) {
	// Try to find by case-insensitive name match.
	row := s.db.QueryRowContext(ctx,
		`SELECT `+contributorColumns+` FROM contributors WHERE LOWER(name) = LOWER(?) AND deleted_at IS NULL`, name)

	c, err := scanContributor(row)
	if err == nil {
		return c, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	// Not found - create new contributor.
	now := time.Now()
	c = &domain.Contributor{
		Syncable: domain.Syncable{
			ID:        uuid.New().String(),
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:     name,
		SortName: name,
		Aliases:  []string{},
	}

	if err := s.CreateContributor(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// GetContributorByASIN retrieves a contributor by their ASIN, excluding soft-deleted records.
// Returns store.ErrNotFound if no contributor with that ASIN exists.
func (s *Store) GetContributorByASIN(ctx context.Context, asin string) (*domain.Contributor, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+contributorColumns+` FROM contributors WHERE asin = ? AND deleted_at IS NULL`, asin)

	c, err := scanContributor(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

// GetContributorsByIDs retrieves multiple contributors by their IDs, excluding soft-deleted records.
// Returns only found contributors (no error for missing IDs).
func (s *Store) GetContributorsByIDs(ctx context.Context, ids []string) ([]*domain.Contributor, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `SELECT ` + contributorColumns + ` FROM contributors
		WHERE id IN (` + strings.Join(placeholders, ",") + `) AND deleted_at IS NULL`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query contributors by IDs: %w", err)
	}
	defer rows.Close()

	var contributors []*domain.Contributor
	for rows.Next() {
		c, err := scanContributor(rows)
		if err != nil {
			return nil, err
		}
		contributors = append(contributors, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return contributors, nil
}

// GetOrCreateContributorByName is an alias for GetOrCreateContributor.
func (s *Store) GetOrCreateContributorByName(ctx context.Context, name string) (*domain.Contributor, error) {
	return s.GetOrCreateContributor(ctx, name)
}

// GetOrCreateContributorByNameWithAlias finds a contributor by name (case-insensitive)
// or by checking aliases. If found by alias, the second return value is true.
// If not found at all, a new contributor is created.
func (s *Store) GetOrCreateContributorByNameWithAlias(ctx context.Context, name string) (*domain.Contributor, bool, error) {
	// First try exact name match (case-insensitive).
	row := s.db.QueryRowContext(ctx,
		`SELECT `+contributorColumns+` FROM contributors WHERE LOWER(name) = LOWER(?) AND deleted_at IS NULL`, name)

	c, err := scanContributor(row)
	if err == nil {
		return c, false, nil
	}
	if err != sql.ErrNoRows {
		return nil, false, err
	}

	// Try to find by alias. The aliases column is a JSON array, so we search with LIKE
	// for the name within the JSON. We use a case-insensitive comparison.
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+contributorColumns+` FROM contributors
		WHERE deleted_at IS NULL AND LOWER(aliases) LIKE '%' || LOWER(?) || '%'`, name)
	if err != nil {
		return nil, false, fmt.Errorf("query contributors by alias: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		candidate, err := scanContributor(rows)
		if err != nil {
			return nil, false, err
		}
		// Verify the name actually appears in the aliases list (not a substring match).
		for _, alias := range candidate.Aliases {
			if strings.EqualFold(alias, name) {
				return candidate, true, nil
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}

	// Not found by name or alias - create new.
	now := time.Now()
	c = &domain.Contributor{
		Syncable: domain.Syncable{
			ID:        uuid.New().String(),
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:     name,
		SortName: name,
		Aliases:  []string{},
	}

	if err := s.CreateContributor(ctx, c); err != nil {
		return nil, false, err
	}
	return c, false, nil
}

// DeleteContributor performs a soft delete by setting deleted_at and updated_at.
// Returns store.ErrNotFound if the contributor does not exist or is already soft-deleted.
func (s *Store) DeleteContributor(ctx context.Context, id string) error {
	now := formatTime(time.Now())

	result, err := s.db.ExecContext(ctx, `
		UPDATE contributors SET deleted_at = ?, updated_at = ?
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

// ListAllContributors returns all non-deleted contributors without pagination.
func (s *Store) ListAllContributors(ctx context.Context) ([]*domain.Contributor, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+contributorColumns+` FROM contributors WHERE deleted_at IS NULL ORDER BY sort_name COLLATE NOCASE ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contributors []*domain.Contributor
	for rows.Next() {
		c, err := scanContributor(rows)
		if err != nil {
			return nil, err
		}
		contributors = append(contributors, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return contributors, nil
}

// CountContributors returns the number of non-deleted contributors.
func (s *Store) CountContributors(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM contributors WHERE deleted_at IS NULL`).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// CountBooksForContributor returns the number of books associated with a contributor
// via the book_contributors join table. Only counts non-deleted books.
func (s *Store) CountBooksForContributor(ctx context.Context, contributorID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM book_contributors bc
		JOIN books b ON b.id = bc.book_id
		WHERE bc.contributor_id = ? AND b.deleted_at IS NULL`,
		contributorID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// CountBooksForAllContributors returns a map of contributor ID to book count
// for all non-deleted contributors. Only counts non-deleted books.
func (s *Store) CountBooksForAllContributors(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT bc.contributor_id, COUNT(*) as cnt
		FROM book_contributors bc
		JOIN books b ON b.id = bc.book_id
		JOIN contributors c ON c.id = bc.contributor_id
		WHERE b.deleted_at IS NULL AND c.deleted_at IS NULL
		GROUP BY bc.contributor_id`)
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

// GetBooksByContributor returns all non-deleted books associated with a contributor.
// Audio files and chapters are loaded for each book.
func (s *Store) GetBooksByContributor(ctx context.Context, contributorID string) ([]*domain.Book, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+bookColumnsAliased+`
		FROM books b
		JOIN book_contributors bc ON bc.book_id = b.id
		WHERE bc.contributor_id = ? AND b.deleted_at IS NULL
		ORDER BY b.title COLLATE NOCASE ASC`,
		contributorID)
	if err != nil {
		return nil, fmt.Errorf("query books by contributor: %w", err)
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

// GetBooksByContributorRole returns all non-deleted books where the contributor has a specific role.
// The roles column is a JSON array, so we search with LIKE for the role string.
// Audio files and chapters are loaded for each book.
func (s *Store) GetBooksByContributorRole(ctx context.Context, contributorID string, role domain.ContributorRole) ([]*domain.Book, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+bookColumnsAliased+`
		FROM books b
		JOIN book_contributors bc ON bc.book_id = b.id
		WHERE bc.contributor_id = ? AND b.deleted_at IS NULL
			AND bc.roles LIKE '%' || ? || '%'
		ORDER BY b.title COLLATE NOCASE ASC`,
		contributorID, string(role))
	if err != nil {
		return nil, fmt.Errorf("query books by contributor role: %w", err)
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

// SearchContributorsByName searches for non-deleted contributors whose name matches
// the query using a case-insensitive LIKE search. Results are limited by the limit parameter.
func (s *Store) SearchContributorsByName(ctx context.Context, query string, limit int) ([]*domain.Contributor, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+contributorColumns+` FROM contributors
		WHERE deleted_at IS NULL AND name LIKE '%' || ? || '%'
		ORDER BY sort_name COLLATE NOCASE ASC
		LIMIT ?`,
		query, limit)
	if err != nil {
		return nil, fmt.Errorf("search contributors: %w", err)
	}
	defer rows.Close()

	var contributors []*domain.Contributor
	for rows.Next() {
		c, err := scanContributor(rows)
		if err != nil {
			return nil, err
		}
		contributors = append(contributors, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return contributors, nil
}

// GetContributorsUpdatedAfter returns all non-deleted contributors updated after the given timestamp.
func (s *Store) GetContributorsUpdatedAfter(ctx context.Context, timestamp time.Time) ([]*domain.Contributor, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+contributorColumns+` FROM contributors
		WHERE updated_at > ? AND deleted_at IS NULL
		ORDER BY updated_at ASC`,
		formatTime(timestamp))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contributors []*domain.Contributor
	for rows.Next() {
		c, err := scanContributor(rows)
		if err != nil {
			return nil, err
		}
		contributors = append(contributors, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return contributors, nil
}

// GetContributorsDeletedAfter returns the IDs of contributors soft-deleted after the given timestamp.
func (s *Store) GetContributorsDeletedAfter(ctx context.Context, timestamp time.Time) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM contributors WHERE deleted_at > ?`,
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

// MergeContributors merges the source contributor into the target contributor.
// All book_contributors referencing the source are updated to reference the target.
// The source's name is added to the target's aliases. The source is then soft-deleted.
// Returns the updated target contributor.
func (s *Store) MergeContributors(ctx context.Context, sourceID, targetID string) (*domain.Contributor, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Load the source contributor.
	sourceRow := tx.QueryRowContext(ctx,
		`SELECT `+contributorColumns+` FROM contributors WHERE id = ? AND deleted_at IS NULL`, sourceID)
	source, err := scanContributor(sourceRow)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get source contributor: %w", err)
	}

	// Load the target contributor.
	targetRow := tx.QueryRowContext(ctx,
		`SELECT `+contributorColumns+` FROM contributors WHERE id = ? AND deleted_at IS NULL`, targetID)
	target, err := scanContributor(targetRow)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get target contributor: %w", err)
	}

	// Update book_contributors: move source references to target.
	// For rows where the target already has an entry for the same book, we delete the source entry.
	_, err = tx.ExecContext(ctx, `
		DELETE FROM book_contributors
		WHERE contributor_id = ? AND book_id IN (
			SELECT book_id FROM book_contributors WHERE contributor_id = ?
		)`, sourceID, targetID)
	if err != nil {
		return nil, fmt.Errorf("delete duplicate book_contributors: %w", err)
	}

	// Move remaining source entries to target.
	_, err = tx.ExecContext(ctx, `
		UPDATE book_contributors SET contributor_id = ?
		WHERE contributor_id = ?`, targetID, sourceID)
	if err != nil {
		return nil, fmt.Errorf("update book_contributors: %w", err)
	}

	// Add source name to target aliases if not already present.
	now := time.Now()
	aliasExists := false
	for _, a := range target.Aliases {
		if strings.EqualFold(a, source.Name) {
			aliasExists = true
			break
		}
	}
	if !aliasExists {
		target.Aliases = append(target.Aliases, source.Name)
	}

	// Also merge source aliases into target.
	for _, srcAlias := range source.Aliases {
		found := false
		for _, tgtAlias := range target.Aliases {
			if strings.EqualFold(tgtAlias, srcAlias) {
				found = true
				break
			}
		}
		if !found {
			target.Aliases = append(target.Aliases, srcAlias)
		}
	}

	target.UpdatedAt = now

	aliasesJSON, err := json.Marshal(target.Aliases)
	if err != nil {
		return nil, fmt.Errorf("marshal aliases: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE contributors SET updated_at = ?, aliases = ?
		WHERE id = ?`,
		formatTime(target.UpdatedAt),
		string(aliasesJSON),
		target.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("update target contributor: %w", err)
	}

	// Soft-delete the source contributor.
	_, err = tx.ExecContext(ctx, `
		UPDATE contributors SET deleted_at = ?, updated_at = ?
		WHERE id = ?`,
		formatTime(now), formatTime(now), sourceID)
	if err != nil {
		return nil, fmt.Errorf("soft-delete source contributor: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit merge: %w", err)
	}

	return target, nil
}

// UnmergeContributor reverses a merge by creating a new contributor from an alias name
// on the source contributor. The alias is removed from the source's aliases list.
// Book contributors that were credited under the alias name are moved to the new contributor.
// Returns the newly created contributor.
func (s *Store) UnmergeContributor(ctx context.Context, sourceID, aliasName string) (*domain.Contributor, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Load the source contributor.
	sourceRow := tx.QueryRowContext(ctx,
		`SELECT `+contributorColumns+` FROM contributors WHERE id = ? AND deleted_at IS NULL`, sourceID)
	source, err := scanContributor(sourceRow)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get source contributor: %w", err)
	}

	// Verify the alias exists on this contributor.
	aliasFound := false
	var newAliases []string
	for _, a := range source.Aliases {
		if strings.EqualFold(a, aliasName) {
			aliasFound = true
		} else {
			newAliases = append(newAliases, a)
		}
	}
	if !aliasFound {
		return nil, store.ErrNotFound
	}

	// Create new contributor from the alias.
	now := time.Now()
	newContrib := &domain.Contributor{
		Syncable: domain.Syncable{
			ID:        uuid.New().String(),
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:     aliasName,
		SortName: aliasName,
		Aliases:  []string{},
	}

	aliasesJSON, err := json.Marshal(newContrib.Aliases)
	if err != nil {
		return nil, fmt.Errorf("marshal new aliases: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO contributors (
			id, created_at, updated_at, deleted_at, name, sort_name, biography,
			image_url, image_blur_hash, asin, aliases, website, birth_date, death_date
		) VALUES (?, ?, ?, NULL, ?, ?, NULL, NULL, NULL, NULL, ?, NULL, NULL, NULL)`,
		newContrib.ID,
		formatTime(newContrib.CreatedAt),
		formatTime(newContrib.UpdatedAt),
		newContrib.Name,
		newContrib.SortName,
		string(aliasesJSON),
	)
	if err != nil {
		return nil, fmt.Errorf("insert new contributor: %w", err)
	}

	// Move book_contributors that were credited as this alias to the new contributor.
	_, err = tx.ExecContext(ctx, `
		UPDATE book_contributors SET contributor_id = ?
		WHERE contributor_id = ? AND credited_as = ?`,
		newContrib.ID, sourceID, aliasName)
	if err != nil {
		return nil, fmt.Errorf("move book_contributors: %w", err)
	}

	// Update source: remove the alias.
	source.Aliases = newAliases
	if source.Aliases == nil {
		source.Aliases = []string{}
	}
	source.UpdatedAt = now

	sourceAliasesJSON, err := json.Marshal(source.Aliases)
	if err != nil {
		return nil, fmt.Errorf("marshal source aliases: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE contributors SET updated_at = ?, aliases = ?
		WHERE id = ?`,
		formatTime(source.UpdatedAt),
		string(sourceAliasesJSON),
		source.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("update source contributor: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit unmerge: %w", err)
	}

	return newContrib, nil
}
