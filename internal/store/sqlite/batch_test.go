package sqlite

import (
	"context"
	"testing"
)

func TestBatchWriter_FlushPersistsBooks(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	ctx := context.Background()
	bw := s.NewBatchWriter(100)

	book1 := makeTestBook("batch-persist-1", "Book One", "/books/one")
	book2 := makeTestBook("batch-persist-2", "Book Two", "/books/two")

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
	got1, err := s.GetBook(ctx, "batch-persist-1", "")
	if err != nil {
		t.Fatalf("GetBook 1: %v", err)
	}
	if got1.Title != "Book One" {
		t.Errorf("book 1 title: got %q, want %q", got1.Title, "Book One")
	}

	got2, err := s.GetBook(ctx, "batch-persist-2", "")
	if err != nil {
		t.Fatalf("GetBook 2: %v", err)
	}
	if got2.Title != "Book Two" {
		t.Errorf("book 2 title: got %q, want %q", got2.Title, "Book Two")
	}
}

func TestBatchWriter_EmptyFlushNoError(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	ctx := context.Background()
	bw := s.NewBatchWriter(100)

	// Flush with no books should not error.
	if err := bw.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
}
