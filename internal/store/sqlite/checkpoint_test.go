package sqlite

import (
	"context"
	"testing"
	"time"
)

func TestGetLibraryCheckpoint_Empty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	got, err := s.GetLibraryCheckpoint(ctx)
	if err != nil {
		t.Fatalf("GetLibraryCheckpoint: %v", err)
	}

	if !got.IsZero() {
		t.Errorf("expected zero time for empty database, got %v", got)
	}
}

func TestGetLibraryCheckpoint_WithData(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Insert data at different timestamps.
	// Books: oldest.
	bookTime := time.Date(2025, 1, 10, 12, 0, 0, 0, time.UTC)
	bookTimeStr := bookTime.Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO books (id, created_at, updated_at, scanned_at, title, path)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"book-cp", bookTimeStr, bookTimeStr, bookTimeStr, "Checkpoint Book", "/books/cp")
	if err != nil {
		t.Fatalf("insert book: %v", err)
	}

	// Contributors: middle.
	contribTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	contribTimeStr := contribTime.Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO contributors (id, created_at, updated_at, name)
		VALUES (?, ?, ?, ?)`,
		"contrib-cp", contribTimeStr, contribTimeStr, "Checkpoint Author")
	if err != nil {
		t.Fatalf("insert contributor: %v", err)
	}

	// Series: latest (should be the checkpoint).
	seriesTime := time.Date(2025, 12, 25, 12, 0, 0, 0, time.UTC)
	seriesTimeStr := seriesTime.Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO series (id, created_at, updated_at, name)
		VALUES (?, ?, ?, ?)`,
		"series-cp", seriesTimeStr, seriesTimeStr, "Checkpoint Series")
	if err != nil {
		t.Fatalf("insert series: %v", err)
	}

	got, err := s.GetLibraryCheckpoint(ctx)
	if err != nil {
		t.Fatalf("GetLibraryCheckpoint: %v", err)
	}

	if got.IsZero() {
		t.Fatal("expected non-zero checkpoint time, got zero")
	}

	// The checkpoint should match the series time (the latest).
	if !got.Equal(seriesTime) {
		t.Errorf("checkpoint: got %v, want %v", got, seriesTime)
	}
}

func TestGetLibraryCheckpoint_ExcludesDeleted(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Insert a non-deleted book with an older time.
	olderTime := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	olderStr := olderTime.Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO books (id, created_at, updated_at, scanned_at, title, path)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"book-alive", olderStr, olderStr, olderStr, "Alive Book", "/books/alive")
	if err != nil {
		t.Fatalf("insert alive book: %v", err)
	}

	// Insert a soft-deleted contributor with the latest time.
	newerTime := time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC)
	newerStr := newerTime.Format(time.RFC3339Nano)
	deletedStr := newerStr
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO contributors (id, created_at, updated_at, deleted_at, name)
		VALUES (?, ?, ?, ?, ?)`,
		"contrib-deleted", newerStr, newerStr, deletedStr, "Deleted Author")
	if err != nil {
		t.Fatalf("insert deleted contributor: %v", err)
	}

	got, err := s.GetLibraryCheckpoint(ctx)
	if err != nil {
		t.Fatalf("GetLibraryCheckpoint: %v", err)
	}

	// Should return the book time since the contributor is deleted.
	if !got.Equal(olderTime) {
		t.Errorf("checkpoint: got %v, want %v (deleted contributor should be excluded)", got, olderTime)
	}
}
