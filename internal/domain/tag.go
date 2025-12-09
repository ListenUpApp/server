package domain

// Tag represents a user-created label for personal organization.
// Tags are flat (no hierarchy) and scoped to a single user.
// Example: "beach read", "book club 2024", "gift for dad"
type Tag struct {
	Syncable
	Name      string `json:"name"`            // Display name
	Slug      string `json:"slug"`            // URL-safe key
	OwnerID   string `json:"owner_id"`        // User who owns this tag
	Color     string `json:"color,omitempty"` // Hex color for UI
	BookCount int    `json:"book_count"`      // Denormalized count
}

// BookTag represents the many-to-many relationship between books and tags.
// Scoped to a user - different users can tag the same book differently.
type BookTag struct {
	BookID    string `json:"book_id"`
	TagID     string `json:"tag_id"`
	UserID    string `json:"user_id"`
	CreatedAt int64  `json:"created_at"` // Unix millis
}
