package domain

import "time"

// Collection represents a logical grouping of books within a library.
// In our access model, collections are privacy boundaries - not organizational tools.
// Books in no collection are visible to all users. Books in a collection are visible
// only to users who own or have been granted access to that collection.
// If a user wants to organize books (but not restrict access) they would
// use a lens (once we get around to writing that logic).
type Collection struct {
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ID        string    `json:"id"`
	LibraryID string    `json:"library_id"`
	OwnerID   string    `json:"owner_id"` // User who owns this collection
	Name      string    `json:"name"`
	BookIDs   []string  `json:"book_ids"`
	IsInbox   bool      `json:"is_inbox"` // System inbox for staging new books
}

// IsSystemCollection returns true if this is the Inbox collection.
// System collections cannot be deleted by users.
func (c *Collection) IsSystemCollection() bool {
	return c.IsInbox
}

// AddBook adds a book ID to the collection if not already present.
func (c *Collection) AddBook(bookID string) bool {
	for _, id := range c.BookIDs {
		if id == bookID {
			return false // Already present
		}
	}
	c.BookIDs = append(c.BookIDs, bookID)
	return true
}

// RemoveBook removes a book ID from the collection.
func (c *Collection) RemoveBook(bookID string) bool {
	for i, id := range c.BookIDs {
		if id == bookID {
			c.BookIDs = append(c.BookIDs[:i], c.BookIDs[i+1:]...)
			return true
		}
	}
	return false
}

// ContainsBook checks if a book ID is in this collection.
func (c *Collection) ContainsBook(bookID string) bool {
	for _, id := range c.BookIDs {
		if id == bookID {
			return true
		}
	}
	return false
}
