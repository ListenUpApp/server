package domain

import "time"

// GenreAlias maps raw metadata strings to normalized genres.
// Example: "Sci-Fi/Fantasy" -> ["science-fiction", "fantasy"]
// This grows over time as users map unmapped genres.
type GenreAlias struct {
	ID        string    `json:"id"`
	RawValue  string    `json:"raw_value"`  // Original metadata string
	GenreIDs  []string  `json:"genre_ids"`  // Maps to these genre IDs
	CreatedBy string    `json:"created_by"` // User who created mapping
	CreatedAt time.Time `json:"created_at"`
}

// UnmappedGenre tracks raw genre strings that couldn't be mapped.
// Displayed in admin UI for manual resolution.
type UnmappedGenre struct {
	RawValue  string    `json:"raw_value"`
	BookCount int       `json:"book_count"` // How many books have this
	FirstSeen time.Time `json:"first_seen"`
	BookIDs   []string  `json:"book_ids"` // Sample of affected books
}
