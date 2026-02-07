package domain

import "time"

// Tag represents a global community tag for categorizing books.
// Tags are shared across all users — no ownership model.
// Slug is the source of truth; clients transform for display: "slow-burn" → "Slow Burn".
type Tag struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`       // Canonical form: lowercase, hyphenated
	BookCount int       `json:"book_count"` // Denormalized count of books with this tag
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Touch updates the UpdatedAt timestamp.
func (t *Tag) Touch() {
	t.UpdatedAt = time.Now()
}

// BookTag represents the many-to-many relationship between books and tags.
// Unlike shelves, this is book-level (not user-scoped) — all users see the same tags on a book.
type BookTag struct {
	BookID    string    `json:"book_id"`
	TagID     string    `json:"tag_id"`
	CreatedAt time.Time `json:"created_at"`
}
