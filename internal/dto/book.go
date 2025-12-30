// Package dto provides Data Transfer Objects for API responses and SSE events.
//
// DTOs contain denormalized fields for immediate client rendering while preserving
// normalized IDs for relationships. This ensures self-contained, immediately-renderable
// data across both sync APIs and real-time SSE events.
package dto

import "github.com/listenupapp/listenup-server/internal/domain"

// BookContributor is the client-facing representation of a book-contributor relationship.
// Includes the denormalized contributor name for immediate rendering.
type BookContributor struct {
	ContributorID string   `json:"contributor_id"`
	Name          string   `json:"name"`                  // Denormalized from Contributor entity
	Roles         []string `json:"roles"`                 // String representation of roles
	CreditedAs    string   `json:"credited_as,omitempty"` // Original attribution (e.g., "Richard Bachman" when contributor is Stephen King)
}

// BookSeriesInfo is the client-facing representation of a book-series relationship.
// Includes the denormalized series name and sequence for immediate rendering.
type BookSeriesInfo struct {
	SeriesID string `json:"series_id"`
	Name     string `json:"name"`               // Denormalized from Series entity
	Sequence string `json:"sequence,omitempty"` // Position in this series
}

// BookTag is the client-facing representation of a book-tag relationship.
// Includes denormalized tag info for immediate rendering.
type BookTag struct {
	ID        string `json:"id"`
	Slug      string `json:"slug"`
	BookCount int    `json:"book_count"`
}

// Book is the client-facing representation of a book.
//
// Philosophy: SSE events are UI updates, not database replication.
// Therefore, events must contain everything needed to render immediately:
//   - Denormalized display fields (Author, Narrator, SeriesInfo)
//   - Normalized relationship IDs (Contributors, Series)
//
// This eliminates race conditions and "Unknown Author" flashes while still
// preserving relational integrity for navigation and filtering.
//
// Network cost is negligible (~20 bytes per name, ~10 bytes with gzip).
// Cache refresh is a feature: name changes propagate automatically.
type Book struct {
	*domain.Book // Embeds all database fields

	// Override Contributors with denormalized version
	Contributors []BookContributor `json:"contributors"`

	// Denormalized fields for immediate rendering
	// These are populated by Enricher before sending to clients
	Author     string           `json:"author,omitempty"`      // First contributor with role "author"
	Narrator   string           `json:"narrator,omitempty"`    // First contributor with role "narrator"
	SeriesInfo []BookSeriesInfo `json:"series_info,omitempty"` // Resolved series with names and sequences
	SeriesName string           `json:"series_name,omitempty"` // Primary series name (first in list, for backward compat)
	Genres     []string         `json:"genres,omitempty"`      // Resolved genre names
	Tags       []BookTag        `json:"tags,omitempty"`        // Tags applied to this book
}
