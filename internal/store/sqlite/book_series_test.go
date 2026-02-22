package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
)

func insertTestSeries(t *testing.T, s *Store, id, name string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.Exec(`INSERT INTO series (id, created_at, updated_at, name) VALUES (?,?,?,?)`,
		id, now, now, name)
	if err != nil {
		t.Fatalf("insert test series: %v", err)
	}
}

func TestSetAndGetBookSeries(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestBook(t, s, "book-s1", "The Final Empire", "/books/tfe")
	insertTestSeries(t, s, "series-1", "Mistborn")
	insertTestSeries(t, s, "series-2", "The Cosmere")

	series := []domain.BookSeries{
		{SeriesID: "series-1", Sequence: "1"},
		{SeriesID: "series-2", Sequence: "6"},
	}

	if err := s.setBookSeriesInternal(ctx, "book-s1", series); err != nil {
		t.Fatalf("SetBookSeries: %v", err)
	}

	got, err := s.GetBookSeries(ctx, "book-s1")
	if err != nil {
		t.Fatalf("GetBookSeries: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 series, got %d", len(got))
	}

	// Build a map for order-independent comparison.
	byID := make(map[string]domain.BookSeries)
	for _, bs := range got {
		byID[bs.SeriesID] = bs
	}

	s1, ok := byID["series-1"]
	if !ok {
		t.Fatal("series-1 not found in results")
	}
	if s1.Sequence != "1" {
		t.Errorf("series-1 sequence: got %q, want %q", s1.Sequence, "1")
	}

	s2, ok := byID["series-2"]
	if !ok {
		t.Fatal("series-2 not found in results")
	}
	if s2.Sequence != "6" {
		t.Errorf("series-2 sequence: got %q, want %q", s2.Sequence, "6")
	}
}

func TestSetBookSeries_Replace(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestBook(t, s, "book-sr", "The Well of Ascension", "/books/twoa")
	insertTestSeries(t, s, "series-a", "Mistborn")
	insertTestSeries(t, s, "series-b", "The Cosmere")
	insertTestSeries(t, s, "series-c", "Mistborn Era 1")

	// First set: series-a and series-b.
	first := []domain.BookSeries{
		{SeriesID: "series-a", Sequence: "2"},
		{SeriesID: "series-b", Sequence: "7"},
	}
	if err := s.setBookSeriesInternal(ctx, "book-sr", first); err != nil {
		t.Fatalf("SetBookSeries (first): %v", err)
	}

	// Replace with: series-a and series-c.
	second := []domain.BookSeries{
		{SeriesID: "series-a", Sequence: "2"},
		{SeriesID: "series-c", Sequence: "2"},
	}
	if err := s.setBookSeriesInternal(ctx, "book-sr", second); err != nil {
		t.Fatalf("SetBookSeries (second): %v", err)
	}

	got, err := s.GetBookSeries(ctx, "book-sr")
	if err != nil {
		t.Fatalf("GetBookSeries: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 series after replace, got %d", len(got))
	}

	byID := make(map[string]domain.BookSeries)
	for _, bs := range got {
		byID[bs.SeriesID] = bs
	}

	if _, ok := byID["series-b"]; ok {
		t.Error("series-b should have been removed after replace")
	}

	if bs, ok := byID["series-c"]; !ok {
		t.Error("series-c not found after replace")
	} else if bs.Sequence != "2" {
		t.Errorf("series-c sequence: got %q, want %q", bs.Sequence, "2")
	}

	if _, ok := byID["series-a"]; !ok {
		t.Error("series-a should still be present after replace")
	}
}

func TestGetBookSeries_Empty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestBook(t, s, "book-se", "Elantris", "/books/elantris")

	got, err := s.GetBookSeries(ctx, "book-se")
	if err != nil {
		t.Fatalf("GetBookSeries: %v", err)
	}

	if got != nil {
		t.Errorf("expected nil for empty series, got %v", got)
	}
}
