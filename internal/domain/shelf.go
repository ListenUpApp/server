package domain

import (
	"slices"
	"time"
)

// Shelf represents a user-curated list of books for personal organization and social discovery.
// Unlike Collections (admin-managed access boundaries), Shelves are personal - each belongs
// to one user. Users can create Shelves to organize their books by theme, reading progress,
// or any other personal categorization.
type Shelf struct {
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ID          string    `json:"id"`
	OwnerID     string    `json:"owner_id"`    // User who owns this shelf
	Name        string    `json:"name"`        // Display name of the shelf
	Description string    `json:"description"` // Optional description
	Color       string    `json:"color"`       // Reserved for future UI customization
	Icon        string    `json:"icon"`        // Reserved for future UI customization
	BookIDs     []string  `json:"book_ids"`    // Ordered list of book IDs (newest first)
}

// AddBook adds a book ID to the shelf, prepending it to maintain newest-first ordering.
// If the book is already present, this is a no-op. Updates UpdatedAt on success.
func (s *Shelf) AddBook(bookID string) bool {
	if slices.Contains(s.BookIDs, bookID) {
		return false // Already present
	}
	// Prepend to maintain newest-first ordering
	s.BookIDs = append([]string{bookID}, s.BookIDs...)
	s.UpdatedAt = time.Now()
	return true
}

// RemoveBook removes a book ID from the shelf.
// Updates UpdatedAt on success. Returns false if the book was not present.
func (s *Shelf) RemoveBook(bookID string) bool {
	for i, id := range s.BookIDs {
		if id == bookID {
			s.BookIDs = append(s.BookIDs[:i], s.BookIDs[i+1:]...)
			s.UpdatedAt = time.Now()
			return true
		}
	}
	return false
}

// ContainsBook checks if a book ID is in this shelf.
func (s *Shelf) ContainsBook(bookID string) bool {
	return slices.Contains(s.BookIDs, bookID)
}
