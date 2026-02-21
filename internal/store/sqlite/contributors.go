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
