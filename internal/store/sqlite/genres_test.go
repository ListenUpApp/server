package sqlite

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// makeTestGenre creates a domain.Genre with sensible defaults for testing.
func makeTestGenre(id, name, slug string) *domain.Genre {
	now := time.Now()
	return &domain.Genre{
		Syncable: domain.Syncable{
			ID:        id,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: name,
		Slug: slug,
		Path: "/" + slug,
	}
}

func TestCreateAndGetGenre(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	g := makeTestGenre("genre-1", "Epic Fantasy", "epic-fantasy")
	g.Description = "Books with expansive worlds and magic systems."
	g.Path = "/fiction/fantasy/epic-fantasy"
	g.Depth = 2
	g.SortOrder = 5
	g.Color = "#FF5733"
	g.Icon = "sword"
	g.IsSystem = true

	if err := s.CreateGenre(ctx, g); err != nil {
		t.Fatalf("CreateGenre: %v", err)
	}

	got, err := s.GetGenre(ctx, "genre-1")
	if err != nil {
		t.Fatalf("GetGenre: %v", err)
	}

	// Verify fields.
	if got.ID != g.ID {
		t.Errorf("ID: got %q, want %q", got.ID, g.ID)
	}
	if got.Name != g.Name {
		t.Errorf("Name: got %q, want %q", got.Name, g.Name)
	}
	if got.Slug != g.Slug {
		t.Errorf("Slug: got %q, want %q", got.Slug, g.Slug)
	}
	if got.Description != g.Description {
		t.Errorf("Description: got %q, want %q", got.Description, g.Description)
	}
	if got.ParentID != g.ParentID {
		t.Errorf("ParentID: got %q, want %q", got.ParentID, g.ParentID)
	}
	if got.Path != g.Path {
		t.Errorf("Path: got %q, want %q", got.Path, g.Path)
	}
	if got.Depth != g.Depth {
		t.Errorf("Depth: got %d, want %d", got.Depth, g.Depth)
	}
	if got.SortOrder != g.SortOrder {
		t.Errorf("SortOrder: got %d, want %d", got.SortOrder, g.SortOrder)
	}
	if got.Color != g.Color {
		t.Errorf("Color: got %q, want %q", got.Color, g.Color)
	}
	if got.Icon != g.Icon {
		t.Errorf("Icon: got %q, want %q", got.Icon, g.Icon)
	}
	if got.IsSystem != g.IsSystem {
		t.Errorf("IsSystem: got %v, want %v", got.IsSystem, g.IsSystem)
	}
	if got.DeletedAt != nil {
		t.Error("DeletedAt: expected nil")
	}

	// Timestamps should round-trip through RFC3339Nano.
	if got.CreatedAt.Unix() != g.CreatedAt.Unix() {
		t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, g.CreatedAt)
	}
	if got.UpdatedAt.Unix() != g.UpdatedAt.Unix() {
		t.Errorf("UpdatedAt: got %v, want %v", got.UpdatedAt, g.UpdatedAt)
	}
}

func TestGetGenre_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetGenre(ctx, "nonexistent")
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

func TestGetGenreBySlug(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	g := makeTestGenre("genre-slug-1", "Science Fiction", "science-fiction")
	g.Description = "Speculative fiction dealing with imaginative concepts."
	if err := s.CreateGenre(ctx, g); err != nil {
		t.Fatalf("CreateGenre: %v", err)
	}

	got, err := s.GetGenreBySlug(ctx, "science-fiction")
	if err != nil {
		t.Fatalf("GetGenreBySlug: %v", err)
	}

	if got.ID != "genre-slug-1" {
		t.Errorf("ID: got %q, want %q", got.ID, "genre-slug-1")
	}
	if got.Name != "Science Fiction" {
		t.Errorf("Name: got %q, want %q", got.Name, "Science Fiction")
	}
	if got.Slug != "science-fiction" {
		t.Errorf("Slug: got %q, want %q", got.Slug, "science-fiction")
	}
	if got.Description != g.Description {
		t.Errorf("Description: got %q, want %q", got.Description, g.Description)
	}
}

func TestCreateGenre_DuplicateSlug(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	g1 := makeTestGenre("genre-dup-1", "Fantasy", "fantasy")
	if err := s.CreateGenre(ctx, g1); err != nil {
		t.Fatalf("CreateGenre g1: %v", err)
	}

	// Different ID, same slug should fail.
	g2 := makeTestGenre("genre-dup-2", "Fantasy Duplicate", "fantasy")
	err := s.CreateGenre(ctx, g2)
	if err == nil {
		t.Fatal("expected error for duplicate slug, got nil")
	}
	if !errors.Is(err, store.ErrAlreadyExists) {
		t.Errorf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestListGenres(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create 3 genres with paths that produce a known sort order.
	genres := []struct {
		id   string
		name string
		slug string
		path string
	}{
		{"genre-l1", "Mystery", "mystery", "/mystery"},
		{"genre-l2", "Fantasy", "fantasy", "/fantasy"},
		{"genre-l3", "Biography", "biography", "/biography"},
	}
	// Expected path sort order: /biography, /fantasy, /mystery

	for _, gd := range genres {
		g := makeTestGenre(gd.id, gd.name, gd.slug)
		g.Path = gd.path
		if err := s.CreateGenre(ctx, g); err != nil {
			t.Fatalf("CreateGenre(%s): %v", gd.id, err)
		}
	}

	got, err := s.ListGenres(ctx)
	if err != nil {
		t.Fatalf("ListGenres: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 genres, got %d", len(got))
	}

	// Verify sorted by path ASC.
	if got[0].Slug != "biography" {
		t.Errorf("item 0: got slug %q, want %q", got[0].Slug, "biography")
	}
	if got[1].Slug != "fantasy" {
		t.Errorf("item 1: got slug %q, want %q", got[1].Slug, "fantasy")
	}
	if got[2].Slug != "mystery" {
		t.Errorf("item 2: got slug %q, want %q", got[2].Slug, "mystery")
	}
}

func TestUpdateGenre(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	g := makeTestGenre("genre-update", "Thriller", "thriller")
	if err := s.CreateGenre(ctx, g); err != nil {
		t.Fatalf("CreateGenre: %v", err)
	}

	// Modify fields.
	g.Name = "Psychological Thriller"
	g.Description = "Thrillers focused on the mental states of characters."
	g.Color = "#00FF00"
	g.Icon = "brain"
	g.Touch()

	if err := s.UpdateGenre(ctx, g); err != nil {
		t.Fatalf("UpdateGenre: %v", err)
	}

	got, err := s.GetGenre(ctx, "genre-update")
	if err != nil {
		t.Fatalf("GetGenre after update: %v", err)
	}

	if got.Name != "Psychological Thriller" {
		t.Errorf("Name: got %q, want %q", got.Name, "Psychological Thriller")
	}
	if got.Description != "Thrillers focused on the mental states of characters." {
		t.Errorf("Description: got %q, want %q", got.Description, "Thrillers focused on the mental states of characters.")
	}
	if got.Color != "#00FF00" {
		t.Errorf("Color: got %q, want %q", got.Color, "#00FF00")
	}
	if got.Icon != "brain" {
		t.Errorf("Icon: got %q, want %q", got.Icon, "brain")
	}
	if got.UpdatedAt.Unix() != g.UpdatedAt.Unix() {
		t.Errorf("UpdatedAt: got %v, want %v", got.UpdatedAt, g.UpdatedAt)
	}
}

func TestDeleteGenre(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	g := makeTestGenre("genre-del", "Romance", "romance")
	if err := s.CreateGenre(ctx, g); err != nil {
		t.Fatalf("CreateGenre: %v", err)
	}

	// Verify it exists before deletion.
	_, err := s.GetGenre(ctx, "genre-del")
	if err != nil {
		t.Fatalf("GetGenre before delete: %v", err)
	}

	// Soft delete.
	if err := s.DeleteGenre(ctx, "genre-del"); err != nil {
		t.Fatalf("DeleteGenre: %v", err)
	}

	// GetGenre should return not found.
	_, err = s.GetGenre(ctx, "genre-del")
	if err == nil {
		t.Fatal("expected not found after soft delete, got nil")
	}
	var storeErr *store.Error
	if !errors.As(err, &storeErr) {
		t.Fatalf("expected *store.Error, got %T: %v", err, err)
	}
	if storeErr.Code != store.ErrNotFound.Code {
		t.Errorf("expected status %d, got %d", store.ErrNotFound.Code, storeErr.Code)
	}

	// ListGenres should not include the soft-deleted genre.
	list, err := s.ListGenres(ctx)
	if err != nil {
		t.Fatalf("ListGenres: %v", err)
	}
	for _, item := range list {
		if item.ID == "genre-del" {
			t.Error("soft-deleted genre should not appear in list")
		}
	}

	// Deleting again should return not found.
	err = s.DeleteGenre(ctx, "genre-del")
	if err == nil {
		t.Fatal("expected not found on second delete, got nil")
	}
	if !errors.As(err, &storeErr) {
		t.Fatalf("expected *store.Error on second delete, got %T: %v", err, err)
	}
	if storeErr.Code != store.ErrNotFound.Code {
		t.Errorf("expected status %d on second delete, got %d", store.ErrNotFound.Code, storeErr.Code)
	}
}

func TestSetAndGetBookGenres(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Insert a test book (uses helper from book_contributors_test.go).
	insertTestBook(t, s, "book-g1", "The Name of the Wind", "/books/notw")

	// Create two genres.
	g1 := makeTestGenre("genre-bg1", "Fantasy", "fantasy")
	g2 := makeTestGenre("genre-bg2", "Adventure", "adventure")
	if err := s.CreateGenre(ctx, g1); err != nil {
		t.Fatalf("CreateGenre g1: %v", err)
	}
	if err := s.CreateGenre(ctx, g2); err != nil {
		t.Fatalf("CreateGenre g2: %v", err)
	}

	// Set genres for the book.
	if err := s.SetBookGenres(ctx, "book-g1", []string{"genre-bg1", "genre-bg2"}); err != nil {
		t.Fatalf("SetBookGenres: %v", err)
	}

	got, err := s.GetBookGenres(ctx, "book-g1")
	if err != nil {
		t.Fatalf("GetBookGenres: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 genre IDs, got %d", len(got))
	}

	// Sort for order-independent comparison.
	sort.Strings(got)
	if got[0] != "genre-bg1" {
		t.Errorf("genre[0]: got %q, want %q", got[0], "genre-bg1")
	}
	if got[1] != "genre-bg2" {
		t.Errorf("genre[1]: got %q, want %q", got[1], "genre-bg2")
	}

	// Replace with a single genre to verify old associations are removed.
	if err := s.SetBookGenres(ctx, "book-g1", []string{"genre-bg2"}); err != nil {
		t.Fatalf("SetBookGenres (replace): %v", err)
	}

	got, err = s.GetBookGenres(ctx, "book-g1")
	if err != nil {
		t.Fatalf("GetBookGenres after replace: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 genre ID after replace, got %d", len(got))
	}
	if got[0] != "genre-bg2" {
		t.Errorf("genre after replace: got %q, want %q", got[0], "genre-bg2")
	}
}

func TestGetOrCreateGenreBySlug(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// First call should create a new genre.
	created, err := s.GetOrCreateGenreBySlug(ctx, "horror", "Horror", "")
	if err != nil {
		t.Fatalf("GetOrCreateGenreBySlug (create): %v", err)
	}

	if created.ID == "" {
		t.Error("expected non-empty ID for created genre")
	}
	if created.Name != "Horror" {
		t.Errorf("Name: got %q, want %q", created.Name, "Horror")
	}
	if created.Slug != "horror" {
		t.Errorf("Slug: got %q, want %q", created.Slug, "horror")
	}
	if created.Path != "/horror" {
		t.Errorf("Path: got %q, want %q", created.Path, "/horror")
	}
	if created.Depth != 0 {
		t.Errorf("Depth: got %d, want 0", created.Depth)
	}
	if created.CreatedAt.IsZero() {
		t.Error("CreatedAt: expected non-zero")
	}

	// Verify it was persisted by fetching directly.
	fetched, err := s.GetGenre(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetGenre after create: %v", err)
	}
	if fetched.Slug != "horror" {
		t.Errorf("persisted Slug: got %q, want %q", fetched.Slug, "horror")
	}

	// Second call with the same slug should find the existing genre.
	found, err := s.GetOrCreateGenreBySlug(ctx, "horror", "Horror Duplicate Name", "")
	if err != nil {
		t.Fatalf("GetOrCreateGenreBySlug (find): %v", err)
	}
	if found.ID != created.ID {
		t.Errorf("expected same ID %q, got %q", created.ID, found.ID)
	}
	// The original name should be preserved (not overwritten).
	if found.Name != "Horror" {
		t.Errorf("Name should remain %q, got %q", "Horror", found.Name)
	}
}
