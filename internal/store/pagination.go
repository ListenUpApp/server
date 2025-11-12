package store

import (
	"encoding/base64"
	"fmt"
)

// PaginationParams contains pagination request paramters
type PaginationParams struct {
	Limit  int    // The number of items per page (defaults to 100 with a maximum of 1000)
	Cursor string // Opaque cusor for next page (empty for first page)
}

// PaginatedResult contains paginated data and metadata
type PaginatedResult[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"` // Empty if no more pages
	HasMore    bool   `json:"has_more"`
	Total      int    `json:"total,omitempty"` // Optional: total count (expensive to compute)
}

// DefaultPaginationParms returns sensible defaults
func DefaultPaginationParms() PaginationParams {
	return PaginationParams{
		Limit:  100,
		Cursor: "",
	}
}

// Validate checks and corrects pagination paramters
func (p *PaginationParams) Validate() {
	if p.Limit <= 0 {
		p.Limit = 100
	}

	if p.Limit > 1000 {
		p.Limit = 1000
	}
}

// EncodeCursor creates an opaque cusor from a key
// for BadgerDB, we use the last time's key as the cursor
func EncodeCursor(key string) string {
	if key == "" {
		return ""
	}
	return base64.URLEncoding.EncodeToString([]byte(key))
}

// DecodeCusrsor decodes a cursor back to a key
func DecodeCursor(cursor string) (string, error) {
	if cursor == "" {
		return "", nil
	}

	decoded, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return "", fmt.Errorf("invalid cursor: %w", err)
	}

	return string(decoded), nil
}
