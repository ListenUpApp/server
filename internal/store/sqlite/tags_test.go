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

// makeTestTag creates a domain.Tag with sensible defaults for testing.
func makeTestTag(id, slug string) *domain.Tag {
	now := time.Now()
	return &domain.Tag{
		ID:        id,
		Slug:      slug,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestCreateAndGetTag(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tag := makeTestTag("tag-1", "slow-burn")

	if err := s.CreateTag(ctx, tag); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	got, err := s.GetTagByID(ctx, "tag-1")
	if err != nil {
		t.Fatalf("GetTagByID: %v", err)
	}

	// Verify fields.
	if got.ID != tag.ID {
		t.Errorf("ID: got %q, want %q", got.ID, tag.ID)
	}
	if got.Slug != tag.Slug {
		t.Errorf("Slug: got %q, want %q", got.Slug, tag.Slug)
	}

	// Timestamps should round-trip through RFC3339Nano.
	if got.CreatedAt.Unix() != tag.CreatedAt.Unix() {
		t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, tag.CreatedAt)
	}
	if got.UpdatedAt.Unix() != tag.UpdatedAt.Unix() {
		t.Errorf("UpdatedAt: got %v, want %v", got.UpdatedAt, tag.UpdatedAt)
	}
}

func TestGetTagBySlug(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tag := makeTestTag("tag-slug-1", "found-family")
	if err := s.CreateTag(ctx, tag); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	got, err := s.GetTagBySlug(ctx, "found-family")
	if err != nil {
		t.Fatalf("GetTagBySlug: %v", err)
	}

	if got.ID != "tag-slug-1" {
		t.Errorf("ID: got %q, want %q", got.ID, "tag-slug-1")
	}
	if got.Slug != "found-family" {
		t.Errorf("Slug: got %q, want %q", got.Slug, "found-family")
	}
}

func TestGetTag_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetTagByID(ctx, "nonexistent")
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

	// Also verify GetTagBySlug returns not found.
	_, err = s.GetTagBySlug(ctx, "nonexistent-slug")
	if err == nil {
		t.Fatal("expected error for slug lookup, got nil")
	}
	if !errors.As(err, &storeErr) {
		t.Fatalf("expected *store.Error for slug lookup, got %T: %v", err, err)
	}
	if storeErr.Code != store.ErrNotFound.Code {
		t.Errorf("expected status %d for slug lookup, got %d", store.ErrNotFound.Code, storeErr.Code)
	}
}

func TestCreateTag_DuplicateSlug(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	t1 := makeTestTag("tag-dup-1", "enemies-to-lovers")
	if err := s.CreateTag(ctx, t1); err != nil {
		t.Fatalf("CreateTag t1: %v", err)
	}

	// Different ID, same slug should fail.
	t2 := makeTestTag("tag-dup-2", "enemies-to-lovers")
	err := s.CreateTag(ctx, t2)
	if err == nil {
		t.Fatal("expected error for duplicate slug, got nil")
	}
	if !errors.Is(err, store.ErrAlreadyExists) {
		t.Errorf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestListTags(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create 3 tags with slugs that sort alphabetically.
	slugs := []struct {
		id   string
		slug string
	}{
		{"tag-l1", "zombies"},
		{"tag-l2", "adventure"},
		{"tag-l3", "magic-system"},
	}
	// Expected slug sort order: adventure, magic-system, zombies

	for _, td := range slugs {
		tag := makeTestTag(td.id, td.slug)
		if err := s.CreateTag(ctx, tag); err != nil {
			t.Fatalf("CreateTag(%s): %v", td.id, err)
		}
	}

	got, err := s.ListTags(ctx)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 tags, got %d", len(got))
	}

	// Verify sorted by slug ASC.
	if got[0].Slug != "adventure" {
		t.Errorf("item 0: got slug %q, want %q", got[0].Slug, "adventure")
	}
	if got[1].Slug != "magic-system" {
		t.Errorf("item 1: got slug %q, want %q", got[1].Slug, "magic-system")
	}
	if got[2].Slug != "zombies" {
		t.Errorf("item 2: got slug %q, want %q", got[2].Slug, "zombies")
	}
}

func TestFindOrCreateTagBySlug(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// First call should create a new tag.
	tag1, created, err := s.FindOrCreateTagBySlug(ctx, "unreliable-narrator")
	if err != nil {
		t.Fatalf("FindOrCreateTagBySlug (create): %v", err)
	}
	if !created {
		t.Error("expected created=true for new tag")
	}
	if tag1.ID == "" {
		t.Error("expected non-empty ID for created tag")
	}
	if tag1.Slug != "unreliable-narrator" {
		t.Errorf("Slug: got %q, want %q", tag1.Slug, "unreliable-narrator")
	}
	if tag1.CreatedAt.IsZero() {
		t.Error("CreatedAt: expected non-zero")
	}
	if tag1.UpdatedAt.IsZero() {
		t.Error("UpdatedAt: expected non-zero")
	}

	// Verify it was persisted.
	fetched, err := s.GetTagBySlug(ctx, "unreliable-narrator")
	if err != nil {
		t.Fatalf("GetTagBySlug after create: %v", err)
	}
	if fetched.ID != tag1.ID {
		t.Errorf("persisted ID: got %q, want %q", fetched.ID, tag1.ID)
	}

	// Second call with the same slug should find the existing tag.
	tag2, created2, err := s.FindOrCreateTagBySlug(ctx, "unreliable-narrator")
	if err != nil {
		t.Fatalf("FindOrCreateTagBySlug (find): %v", err)
	}
	if created2 {
		t.Error("expected created=false for existing tag")
	}
	if tag2.ID != tag1.ID {
		t.Errorf("expected same ID %q, got %q", tag1.ID, tag2.ID)
	}
}

func TestSetAndGetBookTags(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Insert a test book (uses helper from book_contributors_test.go).
	insertTestBook(t, s, "book-t1", "Project Hail Mary", "/books/phm")

	// Create two tags.
	t1 := makeTestTag("tag-bt1", "space")
	t2 := makeTestTag("tag-bt2", "survival")
	if err := s.CreateTag(ctx, t1); err != nil {
		t.Fatalf("CreateTag t1: %v", err)
	}
	if err := s.CreateTag(ctx, t2); err != nil {
		t.Fatalf("CreateTag t2: %v", err)
	}

	// Set tags for the book.
	if err := s.SetBookTags(ctx, "book-t1", []string{"tag-bt1", "tag-bt2"}); err != nil {
		t.Fatalf("SetBookTags: %v", err)
	}

	got, err := s.GetBookTags(ctx, "book-t1")
	if err != nil {
		t.Fatalf("GetBookTags: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 tag IDs, got %d", len(got))
	}

	// Sort for order-independent comparison.
	sort.Strings(got)
	if got[0] != "tag-bt1" {
		t.Errorf("tag[0]: got %q, want %q", got[0], "tag-bt1")
	}
	if got[1] != "tag-bt2" {
		t.Errorf("tag[1]: got %q, want %q", got[1], "tag-bt2")
	}

	// Replace with a single tag to verify old associations are removed.
	if err := s.SetBookTags(ctx, "book-t1", []string{"tag-bt2"}); err != nil {
		t.Fatalf("SetBookTags (replace): %v", err)
	}

	got, err = s.GetBookTags(ctx, "book-t1")
	if err != nil {
		t.Fatalf("GetBookTags after replace: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 tag ID after replace, got %d", len(got))
	}
	if got[0] != "tag-bt2" {
		t.Errorf("tag after replace: got %q, want %q", got[0], "tag-bt2")
	}
}
