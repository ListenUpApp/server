package domain

import "time"

// Lens represents a user-curated list of books for personal organization and social discovery.
// Unlike Collections (admin-managed access boundaries), Lenses are personal - each belongs
// to one user. Users can create Lenses to organize their books by theme, reading progress,
// or any other personal categorization.
type Lens struct {
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ID          string    `json:"id"`
	OwnerID     string    `json:"owner_id"`    // User who owns this lens
	Name        string    `json:"name"`        // Display name of the lens
	Description string    `json:"description"` // Optional description
	Color       string    `json:"color"`       // Reserved for future UI customization
	Icon        string    `json:"icon"`        // Reserved for future UI customization
	BookIDs     []string  `json:"book_ids"`    // Ordered list of book IDs (newest first)
}

// AddBook adds a book ID to the lens, prepending it to maintain newest-first ordering.
// If the book is already present, this is a no-op. Updates UpdatedAt on success.
func (l *Lens) AddBook(bookID string) bool {
	for _, id := range l.BookIDs {
		if id == bookID {
			return false // Already present
		}
	}
	// Prepend to maintain newest-first ordering
	l.BookIDs = append([]string{bookID}, l.BookIDs...)
	l.UpdatedAt = time.Now()
	return true
}

// RemoveBook removes a book ID from the lens.
// Updates UpdatedAt on success. Returns false if the book was not present.
func (l *Lens) RemoveBook(bookID string) bool {
	for i, id := range l.BookIDs {
		if id == bookID {
			l.BookIDs = append(l.BookIDs[:i], l.BookIDs[i+1:]...)
			l.UpdatedAt = time.Now()
			return true
		}
	}
	return false
}

// ContainsBook checks if a book ID is in this lens.
func (l *Lens) ContainsBook(bookID string) bool {
	for _, id := range l.BookIDs {
		if id == bookID {
			return true
		}
	}
	return false
}
