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
	GetSeries(ctx context.Context, id string) (*domain.Series, error)
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

	// Enrich series name
	if book.SeriesID != "" {
		series, err := e.store.GetSeries(ctx, book.SeriesID)
		if err != nil {
			// Don't fail enrichment if series lookup fails
			// Series name remains empty, which is acceptable
		} else {
			dto.SeriesName = series.Name
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

	// Note: We could batch-fetch series here too, but series lookups are rare
	// compared to contributors (most books don't have series).
	// Optimize if profiling shows series lookups are a bottleneck.

	// Enrich each book
	enrichedBooks := make([]*Book, len(books))
	for i, book := range books {
		dto := &Book{Book: book}

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

		// Enrich series (individual lookup per book with series)
		if book.SeriesID != "" {
			series, err := e.store.GetSeries(ctx, book.SeriesID)
			if err != nil {
				// Don't fail entire batch if one series lookup fails
			} else {
				dto.SeriesName = series.Name
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
