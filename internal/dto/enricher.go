package dto

import (
	"context"
	"fmt"

	"github.com/listenupapp/listenup-server/internal/domain"
)

// Store defines the interface for fetching related entities during enrichment.
// This allows Enricher to remain testable and independent of concrete store implementation.
type Store interface {
	GetContributorsByIDs(ctx context.Context, ids []string) ([]*domain.Contributor, error)
	GetGenresByIDs(ctx context.Context, ids []string) ([]*domain.Genre, error)
	GetSeries(ctx context.Context, id string) (*domain.Series, error)
	GetSeriesByIDs(ctx context.Context, ids []string) ([]*domain.Series, error)
	GetTagsForBook(ctx context.Context, bookID string) ([]*domain.Tag, error)
	GetTagsForBookIDs(ctx context.Context, bookIDs []string) (map[string][]*domain.Tag, error)
}

// Enricher denormalizes domain models for client consumption.
//
// Design philosophy:
//   - Batch fetching: One query per entity type, not per book
//   - Graceful degradation: Missing data returns empty strings, not errors
//   - Idempotent: Safe to enrich the same book multiple times
type Enricher struct {
	store Store
}

// NewEnricher creates a new enricher.
func NewEnricher(store Store) *Enricher {
	return &Enricher{store: store}
}

// EnrichBook denormalizes a single book for client consumption.
// Returns a new dto.Book with display fields populated from related entities.
//
// Gracefully handles missing data:
//   - No contributors → Author/Narrator remain empty
//   - No series → SeriesName remains empty
//   - Database errors → Returns error (caller can fallback to un-enriched)
func (e *Enricher) EnrichBook(ctx context.Context, book *domain.Book) (*Book, error) {
	dto := &Book{Book: book}

	// Extract all contributor IDs
	contributorIDs := make([]string, 0, len(book.Contributors))
	for _, bc := range book.Contributors {
		contributorIDs = append(contributorIDs, bc.ContributorID)
	}

	// Batch fetch contributors
	var contributorMap map[string]*domain.Contributor
	if len(contributorIDs) > 0 {
		contributors, err := e.store.GetContributorsByIDs(ctx, contributorIDs)
		if err != nil {
			return nil, fmt.Errorf("fetch contributors: %w", err)
		}

		// Build map for O(1) lookups
		contributorMap = make(map[string]*domain.Contributor, len(contributors))
		for _, contributor := range contributors {
			contributorMap[contributor.ID] = contributor
		}
	}

	// Build denormalized contributors list with names
	dto.Contributors = make([]BookContributor, 0, len(book.Contributors))
	for _, bc := range book.Contributors {
		name := ""
		if contributor, ok := contributorMap[bc.ContributorID]; ok {
			name = contributor.Name
		}

		// Convert roles to strings
		roles := make([]string, len(bc.Roles))
		for i, role := range bc.Roles {
			roles[i] = role.String()
		}

		dto.Contributors = append(dto.Contributors, BookContributor{
			ContributorID: bc.ContributorID,
			Name:          name,
			Roles:         roles,
			CreditedAs:    bc.CreditedAs,
		})
	}

	// Enrich author (first contributor with "author" role)
	for _, bc := range book.Contributors {
		if hasRole(bc, domain.RoleAuthor) {
			if contributor, ok := contributorMap[bc.ContributorID]; ok {
				dto.Author = contributor.Name
				break
			}
		}
	}

	// Enrich narrator (first contributor with "narrator" role)
	for _, bc := range book.Contributors {
		if hasRole(bc, domain.RoleNarrator) {
			if contributor, ok := contributorMap[bc.ContributorID]; ok {
				dto.Narrator = contributor.Name
				break
			}
		}
	}

	// Enrich series info (for all series the book belongs to)
	if len(book.Series) > 0 {
		dto.SeriesInfo = make([]BookSeriesInfo, 0, len(book.Series))
		for _, bs := range book.Series {
			series, err := e.store.GetSeries(ctx, bs.SeriesID)
			if err != nil {
				// Don't fail enrichment if series lookup fails
				// Skip this series entry
				continue
			}
			dto.SeriesInfo = append(dto.SeriesInfo, BookSeriesInfo{
				SeriesID: bs.SeriesID,
				Name:     series.Name,
				Sequence: bs.Sequence,
			})
		}
		// Set primary series name for backward compatibility
		if len(dto.SeriesInfo) > 0 {
			dto.SeriesName = dto.SeriesInfo[0].Name
		}
	}

	// Enrich genres (convert IDs to names).
	// Genre lookup failures are non-fatal - just skip genres.
	if len(book.GenreIDs) > 0 {
		if genres, err := e.store.GetGenresByIDs(ctx, book.GenreIDs); err == nil {
			dto.Genres = make([]string, 0, len(genres))
			for _, g := range genres {
				dto.Genres = append(dto.Genres, g.Name)
			}
		}
	}

	// Enrich tags.
	// Tag lookup failures are non-fatal - just skip tags.
	if tags, err := e.store.GetTagsForBook(ctx, book.ID); err == nil && len(tags) > 0 {
		dto.Tags = make([]BookTag, len(tags))
		for i, tag := range tags {
			dto.Tags[i] = BookTag{
				ID:        tag.ID,
				Slug:      tag.Slug,
				BookCount: tag.BookCount,
			}
		}
	}

	return dto, nil
}

// EnrichBooks denormalizes multiple books efficiently using batch fetching.
//
// This is more efficient than calling EnrichBook in a loop because it:
//   - Collects all contributor IDs across all books
//   - Fetches all contributors in a single query
//   - Reuses the contributor map for all books
//
// Use this for paginated API responses. Use EnrichBook for single-book operations (SSE events).
func (e *Enricher) EnrichBooks(ctx context.Context, books []*domain.Book) ([]*Book, error) {
	if len(books) == 0 {
		return []*Book{}, nil
	}

	// Collect all unique contributor IDs across all books
	contributorIDsMap := make(map[string]bool)
	for _, book := range books {
		for _, bc := range book.Contributors {
			contributorIDsMap[bc.ContributorID] = true
		}
	}

	// Convert map to slice
	contributorIDs := make([]string, 0, len(contributorIDsMap))
	for id := range contributorIDsMap {
		contributorIDs = append(contributorIDs, id)
	}

	// Batch fetch all contributors
	var contributorMap map[string]*domain.Contributor
	if len(contributorIDs) > 0 {
		contributors, err := e.store.GetContributorsByIDs(ctx, contributorIDs)
		if err != nil {
			return nil, fmt.Errorf("fetch contributors: %w", err)
		}

		// Build map for O(1) lookups
		contributorMap = make(map[string]*domain.Contributor, len(contributors))
		for _, contributor := range contributors {
			contributorMap[contributor.ID] = contributor
		}
	}

	// Collect all unique series IDs across all books
	seriesIDsMap := make(map[string]bool)
	for _, book := range books {
		for _, bs := range book.Series {
			seriesIDsMap[bs.SeriesID] = true
		}
	}

	// Batch fetch all series
	var seriesMap map[string]*domain.Series
	if len(seriesIDsMap) > 0 {
		seriesIDs := make([]string, 0, len(seriesIDsMap))
		for id := range seriesIDsMap {
			seriesIDs = append(seriesIDs, id)
		}

		seriesList, err := e.store.GetSeriesByIDs(ctx, seriesIDs)
		if err == nil {
			seriesMap = make(map[string]*domain.Series, len(seriesList))
			for _, series := range seriesList {
				seriesMap[series.ID] = series
			}
		}
	}

	// Collect all unique genre IDs across all books
	genreIDsMap := make(map[string]bool)
	for _, book := range books {
		for _, genreID := range book.GenreIDs {
			genreIDsMap[genreID] = true
		}
	}

	// Batch fetch all genres
	var genreMap map[string]*domain.Genre
	if len(genreIDsMap) > 0 {
		genreIDs := make([]string, 0, len(genreIDsMap))
		for id := range genreIDsMap {
			genreIDs = append(genreIDs, id)
		}

		genres, err := e.store.GetGenresByIDs(ctx, genreIDs)
		if err != nil {
			// Don't fail enrichment if genre lookup fails
		} else {
			genreMap = make(map[string]*domain.Genre, len(genres))
			for _, genre := range genres {
				genreMap[genre.ID] = genre
			}
		}
	}

	// Batch fetch all tags for all books
	bookIDs := make([]string, len(books))
	for i, book := range books {
		bookIDs[i] = book.ID
	}
	tagsMap, _ := e.store.GetTagsForBookIDs(ctx, bookIDs)

	// Enrich each book
	enrichedBooks := make([]*Book, len(books))
	for i, book := range books {
		dto := &Book{Book: book}

		// Build denormalized contributors list with names
		dto.Contributors = make([]BookContributor, 0, len(book.Contributors))
		for _, bc := range book.Contributors {
			name := ""
			if contributor, ok := contributorMap[bc.ContributorID]; ok {
				name = contributor.Name
			}

			// Convert roles to strings
			roles := make([]string, len(bc.Roles))
			for j, role := range bc.Roles {
				roles[j] = role.String()
			}

			dto.Contributors = append(dto.Contributors, BookContributor{
				ContributorID: bc.ContributorID,
				Name:          name,
				Roles:         roles,
				CreditedAs:    bc.CreditedAs,
			})
		}

		// Enrich author
		for _, bc := range book.Contributors {
			if hasRole(bc, domain.RoleAuthor) {
				if contributor, ok := contributorMap[bc.ContributorID]; ok {
					dto.Author = contributor.Name
					break
				}
			}
		}

		// Enrich narrator
		for _, bc := range book.Contributors {
			if hasRole(bc, domain.RoleNarrator) {
				if contributor, ok := contributorMap[bc.ContributorID]; ok {
					dto.Narrator = contributor.Name
					break
				}
			}
		}

		// Enrich series info (use pre-fetched series map)
		if len(book.Series) > 0 && seriesMap != nil {
			dto.SeriesInfo = make([]BookSeriesInfo, 0, len(book.Series))
			for _, bs := range book.Series {
				series, ok := seriesMap[bs.SeriesID]
				if !ok {
					continue
				}
				dto.SeriesInfo = append(dto.SeriesInfo, BookSeriesInfo{
					SeriesID: bs.SeriesID,
					Name:     series.Name,
					Sequence: bs.Sequence,
				})
			}
			// Set primary series name for backward compatibility
			if len(dto.SeriesInfo) > 0 {
				dto.SeriesName = dto.SeriesInfo[0].Name
			}
		}

		// Enrich genres (use pre-fetched genre map)
		if len(book.GenreIDs) > 0 && genreMap != nil {
			dto.Genres = make([]string, 0, len(book.GenreIDs))
			for _, genreID := range book.GenreIDs {
				if genre, ok := genreMap[genreID]; ok {
					dto.Genres = append(dto.Genres, genre.Name)
				}
			}
		}

		// Enrich tags (use pre-fetched tags map)
		if tagsMap != nil {
			if tags, ok := tagsMap[book.ID]; ok && len(tags) > 0 {
				dto.Tags = make([]BookTag, len(tags))
				for j, tag := range tags {
					dto.Tags[j] = BookTag{
						ID:        tag.ID,
						Slug:      tag.Slug,
						BookCount: tag.BookCount,
					}
				}
			}
		}

		enrichedBooks[i] = dto
	}

	return enrichedBooks, nil
}

// hasRole checks if a BookContributor has the specified role.
func hasRole(bc domain.BookContributor, role domain.ContributorRole) bool {
	for _, r := range bc.Roles {
		if r == role {
			return true
		}
	}
	return false
}
