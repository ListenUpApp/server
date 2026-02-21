package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

func TestCreateAndGetLens(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "user-1")
	insertTestBook(t, s, "book-1", "Book One", "/books/one")
	insertTestBook(t, s, "book-2", "Book Two", "/books/two")

	now := time.Now()
	lens := &domain.Lens{
		CreatedAt:   now,
		UpdatedAt:   now,
		ID:          "lens-1",
		OwnerID:     "user-1",
		Name:        "Sci-Fi Favorites",
		Description: "My favorite sci-fi audiobooks",
		Color:       "#FF5733",
		Icon:        "rocket",
		BookIDs:     []string{"book-1", "book-2"},
	}

	if err := s.CreateLens(ctx, lens); err != nil {
		t.Fatalf("CreateLens: %v", err)
	}

	got, err := s.GetLens(ctx, "lens-1")
	if err != nil {
		t.Fatalf("GetLens: %v", err)
	}

	if got.ID != lens.ID {
		t.Errorf("ID: got %q, want %q", got.ID, lens.ID)
	}
	if got.OwnerID != lens.OwnerID {
		t.Errorf("OwnerID: got %q, want %q", got.OwnerID, lens.OwnerID)
	}
	if got.Name != lens.Name {
		t.Errorf("Name: got %q, want %q", got.Name, lens.Name)
	}
	if got.Description != lens.Description {
		t.Errorf("Description: got %q, want %q", got.Description, lens.Description)
	}
	if got.Color != lens.Color {
		t.Errorf("Color: got %q, want %q", got.Color, lens.Color)
	}
	if got.Icon != lens.Icon {
		t.Errorf("Icon: got %q, want %q", got.Icon, lens.Icon)
	}

	// Timestamps should round-trip.
	if got.CreatedAt.Unix() != lens.CreatedAt.Unix() {
		t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, lens.CreatedAt)
	}
	if got.UpdatedAt.Unix() != lens.UpdatedAt.Unix() {
		t.Errorf("UpdatedAt: got %v, want %v", got.UpdatedAt, lens.UpdatedAt)
	}

	// Verify BookIDs are returned in sort_order (insertion order).
	if len(got.BookIDs) != 2 {
		t.Fatalf("BookIDs: got %d, want 2", len(got.BookIDs))
	}
	if got.BookIDs[0] != "book-1" {
		t.Errorf("BookIDs[0]: got %q, want %q", got.BookIDs[0], "book-1")
	}
	if got.BookIDs[1] != "book-2" {
		t.Errorf("BookIDs[1]: got %q, want %q", got.BookIDs[1], "book-2")
	}
}

func TestGetLens_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetLens(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var storeErr *store.Error
	if !errors.As(err, &storeErr) {
		t.Fatalf("expected *store.Error, got %T: %v", err, err)
	}
	if storeErr.Code != store.ErrNotFound.Code {
		t.Errorf("expected status %d, got %d", store.ErrNotFound.Code, storeErr.Code)
	}
}

func TestUpdateLens(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "user-1")
	insertTestBook(t, s, "book-1", "Book One", "/books/one")
	insertTestBook(t, s, "book-2", "Book Two", "/books/two")
	insertTestBook(t, s, "book-3", "Book Three", "/books/three")

	now := time.Now()
	lens := &domain.Lens{
		CreatedAt:   now,
		UpdatedAt:   now,
		ID:          "lens-upd",
		OwnerID:     "user-1",
		Name:        "Original",
		Description: "Original description",
		BookIDs:     []string{"book-1"},
	}

	if err := s.CreateLens(ctx, lens); err != nil {
		t.Fatalf("CreateLens: %v", err)
	}

	// Modify name, description, and replace BookIDs.
	lens.Name = "Updated Lens"
	lens.Description = "Updated description"
	lens.BookIDs = []string{"book-2", "book-3"}
	lens.UpdatedAt = time.Now()

	if err := s.UpdateLens(ctx, lens); err != nil {
		t.Fatalf("UpdateLens: %v", err)
	}

	got, err := s.GetLens(ctx, "lens-upd")
	if err != nil {
		t.Fatalf("GetLens after update: %v", err)
	}

	if got.Name != "Updated Lens" {
		t.Errorf("Name: got %q, want %q", got.Name, "Updated Lens")
	}
	if got.Description != "Updated description" {
		t.Errorf("Description: got %q, want %q", got.Description, "Updated description")
	}
	if len(got.BookIDs) != 2 {
		t.Fatalf("BookIDs: got %d, want 2", len(got.BookIDs))
	}
	if got.BookIDs[0] != "book-2" {
		t.Errorf("BookIDs[0]: got %q, want %q", got.BookIDs[0], "book-2")
	}
	if got.BookIDs[1] != "book-3" {
		t.Errorf("BookIDs[1]: got %q, want %q", got.BookIDs[1], "book-3")
	}
}

func TestDeleteLens(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "user-1")

	now := time.Now()
	lens := &domain.Lens{
		CreatedAt: now,
		UpdatedAt: now,
		ID:        "lens-del",
		OwnerID:   "user-1",
		Name:      "Delete Me",
	}

	if err := s.CreateLens(ctx, lens); err != nil {
		t.Fatalf("CreateLens: %v", err)
	}

	// Verify it exists.
	_, err := s.GetLens(ctx, "lens-del")
	if err != nil {
		t.Fatalf("GetLens before delete: %v", err)
	}

	// Hard delete.
	if err := s.DeleteLens(ctx, "lens-del"); err != nil {
		t.Fatalf("DeleteLens: %v", err)
	}

	// Should be gone.
	_, err = s.GetLens(ctx, "lens-del")
	if err == nil {
		t.Fatal("expected not found after delete, got nil")
	}
	var storeErr *store.Error
	if !errors.As(err, &storeErr) {
		t.Fatalf("expected *store.Error, got %T: %v", err, err)
	}
	if storeErr.Code != store.ErrNotFound.Code {
		t.Errorf("expected status %d, got %d", store.ErrNotFound.Code, storeErr.Code)
	}
}

func TestListLensesByOwner(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "user-1")

	now := time.Now()
	lens1 := &domain.Lens{
		CreatedAt: now,
		UpdatedAt: now,
		ID:        "lens-lo-1",
		OwnerID:   "user-1",
		Name:      "First Lens",
	}
	lens2 := &domain.Lens{
		CreatedAt: now.Add(1 * time.Second),
		UpdatedAt: now.Add(1 * time.Second),
		ID:        "lens-lo-2",
		OwnerID:   "user-1",
		Name:      "Second Lens",
	}

	if err := s.CreateLens(ctx, lens1); err != nil {
		t.Fatalf("CreateLens 1: %v", err)
	}
	if err := s.CreateLens(ctx, lens2); err != nil {
		t.Fatalf("CreateLens 2: %v", err)
	}

	lenses, err := s.ListLensesByOwner(ctx, "user-1")
	if err != nil {
		t.Fatalf("ListLensesByOwner: %v", err)
	}

	if len(lenses) != 2 {
		t.Fatalf("got %d lenses, want 2", len(lenses))
	}
	if lenses[0].ID != "lens-lo-1" {
		t.Errorf("lenses[0].ID: got %q, want %q", lenses[0].ID, "lens-lo-1")
	}
	if lenses[1].ID != "lens-lo-2" {
		t.Errorf("lenses[1].ID: got %q, want %q", lenses[1].ID, "lens-lo-2")
	}
}

func TestAddAndRemoveBookFromLens(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "user-1")
	insertTestBook(t, s, "book-1", "Book One", "/books/one")

	now := time.Now()
	lens := &domain.Lens{
		CreatedAt: now,
		UpdatedAt: now,
		ID:        "lens-ab",
		OwnerID:   "user-1",
		Name:      "Add/Remove Test",
	}

	if err := s.CreateLens(ctx, lens); err != nil {
		t.Fatalf("CreateLens: %v", err)
	}

	// Add a book.
	if err := s.AddBookToLens(ctx, "lens-ab", "book-1"); err != nil {
		t.Fatalf("AddBookToLens: %v", err)
	}

	got, err := s.GetLens(ctx, "lens-ab")
	if err != nil {
		t.Fatalf("GetLens after add: %v", err)
	}
	if len(got.BookIDs) != 1 {
		t.Fatalf("BookIDs after add: got %d, want 1", len(got.BookIDs))
	}
	if got.BookIDs[0] != "book-1" {
		t.Errorf("BookIDs[0]: got %q, want %q", got.BookIDs[0], "book-1")
	}

	// Adding the same book again should be idempotent (INSERT OR IGNORE).
	if err := s.AddBookToLens(ctx, "lens-ab", "book-1"); err != nil {
		t.Fatalf("AddBookToLens (idempotent): %v", err)
	}

	got, err = s.GetLens(ctx, "lens-ab")
	if err != nil {
		t.Fatalf("GetLens after idempotent add: %v", err)
	}
	if len(got.BookIDs) != 1 {
		t.Errorf("BookIDs after idempotent add: got %d, want 1", len(got.BookIDs))
	}

	// Remove the book.
	if err := s.RemoveBookFromLens(ctx, "lens-ab", "book-1"); err != nil {
		t.Fatalf("RemoveBookFromLens: %v", err)
	}

	got, err = s.GetLens(ctx, "lens-ab")
	if err != nil {
		t.Fatalf("GetLens after remove: %v", err)
	}
	if len(got.BookIDs) != 0 {
		t.Errorf("BookIDs after remove: got %d, want 0", len(got.BookIDs))
	}
}
