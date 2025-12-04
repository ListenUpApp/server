// Package search provides full-text search functionality using Bleve.
// It enables federated search across books, contributors, and series with
// faceted filtering, fuzzy matching, and hierarchical genre traversal.
package search

import (
	"strconv"

	"github.com/listenupapp/listenup-server/internal/domain"
)

// DocType represents the type of document in the unified index.
type DocType string

const (
	DocTypeBook        DocType = "book"
	DocTypeContributor DocType = "contributor"
	DocTypeSeries      DocType = "series"
)

// SearchDocument is the unified document structure for the Bleve index.
// All searchable entities are indexed as SearchDocuments with type discrimination.
//
// Design note: We denormalize author/narrator/series names into book documents
// to enable fast single-query search across all related content. The trade-off
// is storage space for query performance - a worthwhile exchange for audiobook
// libraries where users expect instant results.
type SearchDocument struct {
	// Identity
	ID   string  `json:"id"`   // Original entity ID (book_xxx, contributor_xxx, etc.)
	Type DocType `json:"type"` // Discriminator for result grouping

	// Primary searchable text (different meaning per type)
	// Book: title, Contributor: name, Series: name
	Name string `json:"name"`

	// Book-specific fields (empty for other types)
	Subtitle    string `json:"subtitle,omitempty"`
	Description string `json:"description,omitempty"`
	Author      string `json:"author,omitempty"`      // Denormalized for search
	Narrator    string `json:"narrator,omitempty"`    // Denormalized for search
	Publisher   string `json:"publisher,omitempty"`
	SeriesName  string `json:"series_name,omitempty"` // Denormalized for search

	// Genre paths for hierarchical filtering
	// e.g., ["/fiction/fantasy/epic-fantasy", "/fiction/fantasy", "/fiction"]
	GenrePaths []string `json:"genre_paths,omitempty"`

	// Genre slugs for exact matching
	GenreSlugs []string `json:"genre_slugs,omitempty"`

	// Contributor-specific fields
	Biography string `json:"biography,omitempty"`

	// Numeric fields for range queries and sorting
	Duration    int64 `json:"duration,omitempty"`     // Milliseconds (books only)
	PublishYear int   `json:"publish_year,omitempty"` // (books only)
	BookCount   int   `json:"book_count,omitempty"`   // (contributors/series only)

	// Timestamps for sorting
	CreatedAt int64 `json:"created_at"` // Unix millis
	UpdatedAt int64 `json:"updated_at"` // Unix millis
}

// BookToSearchDocument converts a domain Book to a SearchDocument.
// Requires denormalized fields (author, narrator, series name, genre paths)
// to be provided by the caller, as the search package shouldn't depend on store.
func BookToSearchDocument(
	book *domain.Book,
	author, narrator, seriesName string,
	genrePaths, genreSlugs []string,
) *SearchDocument {
	doc := &SearchDocument{
		ID:          book.ID,
		Type:        DocTypeBook,
		Name:        book.Title,
		Subtitle:    book.Subtitle,
		Description: book.Description,
		Author:      author,
		Narrator:    narrator,
		Publisher:   book.Publisher,
		SeriesName:  seriesName,
		GenrePaths:  genrePaths,
		GenreSlugs:  genreSlugs,
		Duration:    book.TotalDuration,
		CreatedAt:   book.CreatedAt.UnixMilli(),
		UpdatedAt:   book.UpdatedAt.UnixMilli(),
	}

	// Parse publish year
	if book.PublishYear != "" {
		if year, err := strconv.Atoi(book.PublishYear); err == nil {
			doc.PublishYear = year
		}
	}

	return doc
}

// ContributorToSearchDocument converts a domain Contributor to a SearchDocument.
func ContributorToSearchDocument(c *domain.Contributor, bookCount int) *SearchDocument {
	return &SearchDocument{
		ID:        c.ID,
		Type:      DocTypeContributor,
		Name:      c.Name,
		Biography: c.Biography,
		BookCount: bookCount,
		CreatedAt: c.CreatedAt.UnixMilli(),
		UpdatedAt: c.UpdatedAt.UnixMilli(),
	}
}

// SeriesToSearchDocument converts a domain Series to a SearchDocument.
func SeriesToSearchDocument(s *domain.Series) *SearchDocument {
	return &SearchDocument{
		ID:          s.ID,
		Type:        DocTypeSeries,
		Name:        s.Name,
		Description: s.Description,
		BookCount:   s.TotalBooks,
		CreatedAt:   s.CreatedAt.UnixMilli(),
		UpdatedAt:   s.UpdatedAt.UnixMilli(),
	}
}
