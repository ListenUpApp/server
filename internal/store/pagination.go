package store

import (
	"encoding/base64"
	"fmt"
	"time"
)

// PaginationParams contains pagination request parameters.
type PaginationParams struct {
	Cursor       string
	Limit        int
	UpdatedAfter time.Time
}

// PaginatedResult contains paginated data and metadata.
type PaginatedResult[T any] struct {
	NextCursor string `json:"next_cursor,omitempty"`
	Items      []T    `json:"items"`
	Total      int    `json:"total,omitempty"`
	HasMore    bool   `json:"has_more"`
}

// DefaultPaginationParms returns sensible defaults.
func DefaultPaginationParms() PaginationParams {
	return PaginationParams{
		Limit:  100,
		Cursor: "",
	}
}

// Validate checks and corrects pagination parameters.
func (p *PaginationParams) Validate() {
	if p.Limit <= 0 {
		p.Limit = 100
	}

	if p.Limit > 1000 {
		p.Limit = 1000
	}
}

// EncodeCursor creates an opaque cusor from a key.
// for BadgerDB, we use the last time's key as the cursor.
func EncodeCursor(key string) string {
	if key == "" {
		return ""
	}
	return base64.URLEncoding.EncodeToString([]byte(key))
}

// DecodeCursor decodes a cursor back to a key.
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
