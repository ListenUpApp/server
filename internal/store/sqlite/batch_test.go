package sqlite

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
)

// spySearchIndexer records calls to IndexBook for testing.
type spySearchIndexer struct {
	mu      sync.Mutex
	indexed []string // book IDs that were indexed
}

func (s *spySearchIndexer) IndexBook(_ context.Context, book *domain.Book) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.indexed = append(s.indexed, book.ID)
	return nil
}

func (s *spySearchIndexer) DeleteBook(_ context.Context, _ string) error               { return nil }
func (s *spySearchIndexer) IndexContributor(_ context.Context, _ *domain.Contributor) error {
	return nil
}
func (s *spySearchIndexer) DeleteContributor(_ context.Context, _ string) error { return nil }
func (s *spySearchIndexer) IndexSeries(_ context.Context, _ *domain.Series) error {
	return nil
}
func (s *spySearchIndexer) DeleteSeries(_ context.Context, _ string) error { return nil }

func (s *spySearchIndexer) indexedIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.indexed...)
}

func TestBatchWriter_FlushIndexesBooks(t *testing.T) {
	s := newTestStore(t)
	spy := &spySearchIndexer{}
	s.SetSearchIndexer(spy)

	ctx := context.Background()
	bw := s.NewBatchWriter(100)

	book1 := makeTestBook("batch-idx-1", "Book One", "/books/one")
	book2 := makeTestBook("batch-idx-2", "Book Two", "/books/two")

	if err := bw.CreateBook(ctx, book1); err != nil {
		t.Fatalf("CreateBook 1: %v", err)
	}
	if err := bw.CreateBook(ctx, book2); err != nil {
		t.Fatalf("CreateBook 2: %v", err)
	}

	if err := bw.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// Verify books were persisted.
	got1, err := s.GetBook(ctx, "batch-idx-1", "")
	if err != nil {
		t.Fatalf("GetBook 1: %v", err)
	}
	if got1.Title != "Book One" {
		t.Errorf("book 1 title: got %q, want %q", got1.Title, "Book One")
	}

	// indexBookAsync fires goroutines; give them a moment to complete.
	// In practice the goroutines finish nearly instantly, but we need
	// to wait briefly to avoid a flaky assertion.
	waitForCondition(t, func() bool {
		return len(spy.indexedIDs()) >= 2
	})

	ids := spy.indexedIDs()
	if len(ids) != 2 {
		t.Fatalf("expected 2 indexed books, got %d: %v", len(ids), ids)
	}

	idSet := map[string]bool{}
	for _, id := range ids {
		idSet[id] = true
	}
	if !idSet["batch-idx-1"] || !idSet["batch-idx-2"] {
		t.Errorf("expected both book IDs indexed, got %v", ids)
	}
}

func TestBatchWriter_EmptyFlushNoIndex(t *testing.T) {
	s := newTestStore(t)
	spy := &spySearchIndexer{}
	s.SetSearchIndexer(spy)

	ctx := context.Background()
	bw := s.NewBatchWriter(100)

	// Flush with no books should not index anything.
	if err := bw.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	if len(spy.indexedIDs()) != 0 {
		t.Errorf("expected no indexed books, got %v", spy.indexedIDs())
	}
}

// waitForCondition polls fn every 1ms up to 500ms.
func waitForCondition(t *testing.T, fn func() bool) {
	t.Helper()
	for range 500 {
		if fn() {
			return
		}
		time.Sleep(time.Millisecond)
	}
}
