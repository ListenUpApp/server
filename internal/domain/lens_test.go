package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLens_AddBook_PrependsNewestFirst(t *testing.T) {
	lens := &Lens{
		ID:      "lens-1",
		OwnerID: "user-1",
		Name:    "My Reading List",
		BookIDs: []string{"book-1", "book-2"},
	}

	added := lens.AddBook("book-3")

	assert.True(t, added)
	assert.Equal(t, []string{"book-3", "book-1", "book-2"}, lens.BookIDs)
}

func TestLens_AddBook_UpdatesTimestamp(t *testing.T) {
	now := time.Now()
	lens := &Lens{
		ID:        "lens-1",
		OwnerID:   "user-1",
		Name:      "My Reading List",
		UpdatedAt: now.Add(-time.Hour), // Set to an hour ago
	}

	lens.AddBook("book-1")

	assert.True(t, lens.UpdatedAt.After(now.Add(-time.Hour)))
}

func TestLens_AddBook_IgnoresDuplicates(t *testing.T) {
	lens := &Lens{
		ID:      "lens-1",
		OwnerID: "user-1",
		Name:    "My Reading List",
		BookIDs: []string{"book-1", "book-2"},
	}
	originalUpdatedAt := lens.UpdatedAt

	added := lens.AddBook("book-1")

	assert.False(t, added)
	assert.Equal(t, []string{"book-1", "book-2"}, lens.BookIDs)
	assert.Equal(t, originalUpdatedAt, lens.UpdatedAt) // Should not update timestamp
}

func TestLens_AddBook_ToEmptyList(t *testing.T) {
	lens := &Lens{
		ID:      "lens-1",
		OwnerID: "user-1",
		Name:    "Empty Lens",
		BookIDs: []string{},
	}

	added := lens.AddBook("book-1")

	assert.True(t, added)
	assert.Equal(t, []string{"book-1"}, lens.BookIDs)
}

func TestLens_AddBook_ToNilList(t *testing.T) {
	lens := &Lens{
		ID:      "lens-1",
		OwnerID: "user-1",
		Name:    "Nil Lens",
		BookIDs: nil,
	}

	added := lens.AddBook("book-1")

	assert.True(t, added)
	assert.Equal(t, []string{"book-1"}, lens.BookIDs)
}

func TestLens_RemoveBook_Works(t *testing.T) {
	lens := &Lens{
		ID:      "lens-1",
		OwnerID: "user-1",
		Name:    "My Reading List",
		BookIDs: []string{"book-1", "book-2", "book-3"},
	}

	removed := lens.RemoveBook("book-2")

	assert.True(t, removed)
	assert.Equal(t, []string{"book-1", "book-3"}, lens.BookIDs)
}

func TestLens_RemoveBook_UpdatesTimestamp(t *testing.T) {
	now := time.Now()
	lens := &Lens{
		ID:        "lens-1",
		OwnerID:   "user-1",
		Name:      "My Reading List",
		BookIDs:   []string{"book-1"},
		UpdatedAt: now.Add(-time.Hour), // Set to an hour ago
	}

	lens.RemoveBook("book-1")

	assert.True(t, lens.UpdatedAt.After(now.Add(-time.Hour)))
}

func TestLens_RemoveBook_HandlesNonExistentGracefully(t *testing.T) {
	lens := &Lens{
		ID:      "lens-1",
		OwnerID: "user-1",
		Name:    "My Reading List",
		BookIDs: []string{"book-1", "book-2"},
	}
	originalUpdatedAt := lens.UpdatedAt

	removed := lens.RemoveBook("book-nonexistent")

	assert.False(t, removed)
	assert.Equal(t, []string{"book-1", "book-2"}, lens.BookIDs)
	assert.Equal(t, originalUpdatedAt, lens.UpdatedAt) // Should not update timestamp
}

func TestLens_RemoveBook_FromEmptyList(t *testing.T) {
	lens := &Lens{
		ID:      "lens-1",
		OwnerID: "user-1",
		Name:    "Empty Lens",
		BookIDs: []string{},
	}

	removed := lens.RemoveBook("book-1")

	assert.False(t, removed)
	assert.Empty(t, lens.BookIDs)
}

func TestLens_RemoveBook_FirstElement(t *testing.T) {
	lens := &Lens{
		ID:      "lens-1",
		OwnerID: "user-1",
		Name:    "My Reading List",
		BookIDs: []string{"book-1", "book-2", "book-3"},
	}

	removed := lens.RemoveBook("book-1")

	assert.True(t, removed)
	assert.Equal(t, []string{"book-2", "book-3"}, lens.BookIDs)
}

func TestLens_RemoveBook_LastElement(t *testing.T) {
	lens := &Lens{
		ID:      "lens-1",
		OwnerID: "user-1",
		Name:    "My Reading List",
		BookIDs: []string{"book-1", "book-2", "book-3"},
	}

	removed := lens.RemoveBook("book-3")

	assert.True(t, removed)
	assert.Equal(t, []string{"book-1", "book-2"}, lens.BookIDs)
}

func TestLens_ContainsBook_ReturnsTrue(t *testing.T) {
	lens := &Lens{
		ID:      "lens-1",
		OwnerID: "user-1",
		Name:    "My Reading List",
		BookIDs: []string{"book-1", "book-2", "book-3"},
	}

	assert.True(t, lens.ContainsBook("book-1"))
	assert.True(t, lens.ContainsBook("book-2"))
	assert.True(t, lens.ContainsBook("book-3"))
}

func TestLens_ContainsBook_ReturnsFalse(t *testing.T) {
	lens := &Lens{
		ID:      "lens-1",
		OwnerID: "user-1",
		Name:    "My Reading List",
		BookIDs: []string{"book-1", "book-2"},
	}

	assert.False(t, lens.ContainsBook("book-nonexistent"))
}

func TestLens_ContainsBook_EmptyList(t *testing.T) {
	lens := &Lens{
		ID:      "lens-1",
		OwnerID: "user-1",
		Name:    "Empty Lens",
		BookIDs: []string{},
	}

	assert.False(t, lens.ContainsBook("book-1"))
}

func TestLens_ContainsBook_NilList(t *testing.T) {
	lens := &Lens{
		ID:      "lens-1",
		OwnerID: "user-1",
		Name:    "Nil Lens",
		BookIDs: nil,
	}

	assert.False(t, lens.ContainsBook("book-1"))
}
