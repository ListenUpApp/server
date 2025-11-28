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
	Name          string   `json:"name"`  // Denormalized from Contributor entity
	Roles         []string `json:"roles"` // String representation of roles
}

// Book is the client-facing representation of a book.
//
// Philosophy: SSE events are UI updates, not database replication.
// Therefore, events must contain everything needed to render immediately:
//   - Denormalized display fields (Author, Narrator, SeriesName)
//   - Normalized relationship IDs (Contributors, SeriesID)
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
	Author     string `json:"author,omitempty"`      // First contributor with role "author"
	Narrator   string `json:"narrator,omitempty"`    // First contributor with role "narrator"
	SeriesName string `json:"series_name,omitempty"` // Resolved series name
}
