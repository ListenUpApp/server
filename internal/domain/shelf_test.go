package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestShelf_AddBook_PrependsNewestFirst(t *testing.T) {
	shelf := &Shelf{
		ID:      "shelf-1",
		OwnerID: "user-1",
		Name:    "My Reading List",
		BookIDs: []string{"book-1", "book-2"},
	}

	added := shelf.AddBook("book-3")

	assert.True(t, added)
	assert.Equal(t, []string{"book-3", "book-1", "book-2"}, shelf.BookIDs)
}

func TestShelf_AddBook_UpdatesTimestamp(t *testing.T) {
	now := time.Now()
	shelf := &Shelf{
		ID:        "shelf-1",
		OwnerID:   "user-1",
		Name:      "My Reading List",
		UpdatedAt: now.Add(-time.Hour), // Set to an hour ago
	}

	shelf.AddBook("book-1")

	assert.True(t, shelf.UpdatedAt.After(now.Add(-time.Hour)))
}

func TestShelf_AddBook_IgnoresDuplicates(t *testing.T) {
	shelf := &Shelf{
		ID:      "shelf-1",
		OwnerID: "user-1",
		Name:    "My Reading List",
		BookIDs: []string{"book-1", "book-2"},
	}
	originalUpdatedAt := shelf.UpdatedAt

	added := shelf.AddBook("book-1")

	assert.False(t, added)
	assert.Equal(t, []string{"book-1", "book-2"}, shelf.BookIDs)
	assert.Equal(t, originalUpdatedAt, shelf.UpdatedAt) // Should not update timestamp
}

func TestShelf_AddBook_ToEmptyList(t *testing.T) {
	shelf := &Shelf{
		ID:      "shelf-1",
		OwnerID: "user-1",
		Name:    "Empty Shelf",
		BookIDs: []string{},
	}

	added := shelf.AddBook("book-1")

	assert.True(t, added)
	assert.Equal(t, []string{"book-1"}, shelf.BookIDs)
}

func TestShelf_AddBook_ToNilList(t *testing.T) {
	shelf := &Shelf{
		ID:      "shelf-1",
		OwnerID: "user-1",
		Name:    "Nil Shelf",
		BookIDs: nil,
	}

	added := shelf.AddBook("book-1")

	assert.True(t, added)
	assert.Equal(t, []string{"book-1"}, shelf.BookIDs)
}

func TestShelf_RemoveBook_Works(t *testing.T) {
	shelf := &Shelf{
		ID:      "shelf-1",
		OwnerID: "user-1",
		Name:    "My Reading List",
		BookIDs: []string{"book-1", "book-2", "book-3"},
	}

	removed := shelf.RemoveBook("book-2")

	assert.True(t, removed)
	assert.Equal(t, []string{"book-1", "book-3"}, shelf.BookIDs)
}

func TestShelf_RemoveBook_UpdatesTimestamp(t *testing.T) {
	now := time.Now()
	shelf := &Shelf{
		ID:        "shelf-1",
		OwnerID:   "user-1",
		Name:      "My Reading List",
		BookIDs:   []string{"book-1"},
		UpdatedAt: now.Add(-time.Hour), // Set to an hour ago
	}

	shelf.RemoveBook("book-1")

	assert.True(t, shelf.UpdatedAt.After(now.Add(-time.Hour)))
}

func TestShelf_RemoveBook_HandlesNonExistentGracefully(t *testing.T) {
	shelf := &Shelf{
		ID:      "shelf-1",
		OwnerID: "user-1",
		Name:    "My Reading List",
		BookIDs: []string{"book-1", "book-2"},
	}
	originalUpdatedAt := shelf.UpdatedAt

	removed := shelf.RemoveBook("book-nonexistent")

	assert.False(t, removed)
	assert.Equal(t, []string{"book-1", "book-2"}, shelf.BookIDs)
	assert.Equal(t, originalUpdatedAt, shelf.UpdatedAt) // Should not update timestamp
}

func TestShelf_RemoveBook_FromEmptyList(t *testing.T) {
	shelf := &Shelf{
		ID:      "shelf-1",
		OwnerID: "user-1",
		Name:    "Empty Shelf",
		BookIDs: []string{},
	}

	removed := shelf.RemoveBook("book-1")

	assert.False(t, removed)
	assert.Empty(t, shelf.BookIDs)
}

func TestShelf_RemoveBook_FirstElement(t *testing.T) {
	shelf := &Shelf{
		ID:      "shelf-1",
		OwnerID: "user-1",
		Name:    "My Reading List",
		BookIDs: []string{"book-1", "book-2", "book-3"},
	}

	removed := shelf.RemoveBook("book-1")

	assert.True(t, removed)
	assert.Equal(t, []string{"book-2", "book-3"}, shelf.BookIDs)
}

func TestShelf_RemoveBook_LastElement(t *testing.T) {
	shelf := &Shelf{
		ID:      "shelf-1",
		OwnerID: "user-1",
		Name:    "My Reading List",
		BookIDs: []string{"book-1", "book-2", "book-3"},
	}

	removed := shelf.RemoveBook("book-3")

	assert.True(t, removed)
	assert.Equal(t, []string{"book-1", "book-2"}, shelf.BookIDs)
}

func TestShelf_ContainsBook_ReturnsTrue(t *testing.T) {
	shelf := &Shelf{
		ID:      "shelf-1",
		OwnerID: "user-1",
		Name:    "My Reading List",
		BookIDs: []string{"book-1", "book-2", "book-3"},
	}

	assert.True(t, shelf.ContainsBook("book-1"))
	assert.True(t, shelf.ContainsBook("book-2"))
	assert.True(t, shelf.ContainsBook("book-3"))
}

func TestShelf_ContainsBook_ReturnsFalse(t *testing.T) {
	shelf := &Shelf{
		ID:      "shelf-1",
		OwnerID: "user-1",
		Name:    "My Reading List",
		BookIDs: []string{"book-1", "book-2"},
	}

	assert.False(t, shelf.ContainsBook("book-nonexistent"))
}

func TestShelf_ContainsBook_EmptyList(t *testing.T) {
	shelf := &Shelf{
		ID:      "shelf-1",
		OwnerID: "user-1",
		Name:    "Empty Shelf",
		BookIDs: []string{},
	}

	assert.False(t, shelf.ContainsBook("book-1"))
}

func TestShelf_ContainsBook_NilList(t *testing.T) {
	shelf := &Shelf{
		ID:      "shelf-1",
		OwnerID: "user-1",
		Name:    "Nil Shelf",
		BookIDs: nil,
	}

	assert.False(t, shelf.ContainsBook("book-1"))
}
