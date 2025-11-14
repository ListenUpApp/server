package scanner

import (
	"context"
	"log/slog"
	"os"
	"testing"
)

func TestGrouper_Group_SingleFile(t *testing.T) {
	// Single audio file in root should be its own audiobook.
	files := []WalkResult{
		{
			Path:    "/library/book.mp3",
			RelPath: "book.mp3",
			IsDir:   false,
			Size:    1000,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	grouper := NewGrouper(logger)

	ctx := context.Background()
	grouped := grouper.Group(ctx, files, GroupOptions{})

	// Should have one group with the file itself as the key.
	if len(grouped) != 1 {
		t.Fatalf("expected 1 group, got %d", len(grouped))
	}

	// The group key should be the file path.
	group, exists := grouped["/library/book.mp3"]
	if !exists {
		t.Fatal("expected group with key '/library/book.mp3'")
	}

	if len(group) != 1 {
		t.Errorf("expected 1 file in group, got %d", len(group))
	}
}

func TestGrouper_Group_MultipleFilesInDirectory(t *testing.T) {
	// Multiple audio files in same directory = one audiobook.
	files := []WalkResult{
		{
			Path:    "/library/book/01.mp3",
			RelPath: "book/01.mp3",
			IsDir:   false,
			Size:    1000,
		},
		{
			Path:    "/library/book/02.mp3",
			RelPath: "book/02.mp3",
			IsDir:   false,
			Size:    1000,
		},
		{
			Path:    "/library/book/cover.jpg",
			RelPath: "book/cover.jpg",
			IsDir:   false,
			Size:    5000,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	grouper := NewGrouper(logger)

	ctx := context.Background()
	grouped := grouper.Group(ctx, files, GroupOptions{})

	// Should have one group.
	if len(grouped) != 1 {
		t.Fatalf("expected 1 group, got %d", len(grouped))
	}

	// The group key should be the directory.
	group, exists := grouped["/library/book"]
	if !exists {
		t.Fatal("expected group with key '/library/book'")
	}

	// Should contain all 3 files.
	if len(group) != 3 {
		t.Errorf("expected 3 files in group, got %d", len(group))
	}
}

func TestGrouper_Group_MultipleBooks(t *testing.T) {
	// Multiple separate books should be in separate groups.
	files := []WalkResult{
		{
			Path:    "/library/book1/audio.mp3",
			RelPath: "book1/audio.mp3",
			IsDir:   false,
		},
		{
			Path:    "/library/book2/audio.mp3",
			RelPath: "book2/audio.mp3",
			IsDir:   false,
		},
		{
			Path:    "/library/book3.mp3",
			RelPath: "book3.mp3",
			IsDir:   false,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	grouper := NewGrouper(logger)

	ctx := context.Background()
	grouped := grouper.Group(ctx, files, GroupOptions{})

	// Should have 3 groups.
	if len(grouped) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(grouped))
	}

	// Verify each group.
	if _, exists := grouped["/library/book1"]; !exists {
		t.Error("expected group '/library/book1'")
	}
	if _, exists := grouped["/library/book2"]; !exists {
		t.Error("expected group '/library/book2'")
	}
	if _, exists := grouped["/library/book3.mp3"]; !exists {
		t.Error("expected group '/library/book3.mp3'")
	}
}

func TestGrouper_Group_MultiDisc(t *testing.T) {
	// Multi-disc structure should be grouped together.
	files := []WalkResult{
		{
			Path:    "/library/book/CD1/01.mp3",
			RelPath: "book/CD1/01.mp3",
			IsDir:   false,
		},
		{
			Path:    "/library/book/CD1/02.mp3",
			RelPath: "book/CD1/02.mp3",
			IsDir:   false,
		},
		{
			Path:    "/library/book/CD2/01.mp3",
			RelPath: "book/CD2/01.mp3",
			IsDir:   false,
		},
		{
			Path:    "/library/book/CD2/02.mp3",
			RelPath: "book/CD2/02.mp3",
			IsDir:   false,
		},
		{
			Path:    "/library/book/cover.jpg",
			RelPath: "book/cover.jpg",
			IsDir:   false,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	grouper := NewGrouper(logger)

	ctx := context.Background()
	grouped := grouper.Group(ctx, files, GroupOptions{})

	// Should have one group (all discs together).
	if len(grouped) != 1 {
		t.Fatalf("expected 1 group, got %d", len(grouped))
	}

	// The group key should be the parent directory.
	group, exists := grouped["/library/book"]
	if !exists {
		t.Fatal("expected group with key '/library/book'")
	}

	// Should contain all 5 files.
	if len(group) != 5 {
		t.Errorf("expected 5 files in group, got %d", len(group))
	}
}

func TestGrouper_Group_NestedAuthorBook(t *testing.T) {
	// Author/Book nested structure.
	files := []WalkResult{
		{
			Path:    "/library/Author Name/Book Title/01.mp3",
			RelPath: "Author Name/Book Title/01.mp3",
			IsDir:   false,
		},
		{
			Path:    "/library/Author Name/Book Title/02.mp3",
			RelPath: "Author Name/Book Title/02.mp3",
			IsDir:   false,
		},
		{
			Path:    "/library/Author Name/Another Book/audio.mp3",
			RelPath: "Author Name/Another Book/audio.mp3",
			IsDir:   false,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	grouper := NewGrouper(logger)

	ctx := context.Background()
	grouped := grouper.Group(ctx, files, GroupOptions{})

	// Should have 2 groups (one per book).
	if len(grouped) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(grouped))
	}

	// Verify groups.
	group1, exists := grouped["/library/Author Name/Book Title"]
	if !exists {
		t.Fatal("expected group '/library/Author Name/Book Title'")
	}
	if len(group1) != 2 {
		t.Errorf("expected 2 files in first book, got %d", len(group1))
	}

	group2, exists := grouped["/library/Author Name/Another Book"]
	if !exists {
		t.Fatal("expected group '/library/Author Name/Another Book'")
	}
	if len(group2) != 1 {
		t.Errorf("expected 1 file in second book, got %d", len(group2))
	}
}

func TestGrouper_Group_EmptyInput(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	grouper := NewGrouper(logger)

	ctx := context.Background()
	grouped := grouper.Group(ctx, []WalkResult{}, GroupOptions{})

	if len(grouped) != 0 {
		t.Errorf("expected 0 groups, got %d", len(grouped))
	}
}

func TestGrouper_Group_MixedDiscFormats(t *testing.T) {
	// Test different disc naming conventions.
	files := []WalkResult{
		{
			Path:    "/library/book/Disc 1/01.mp3",
			RelPath: "book/Disc 1/01.mp3",
			IsDir:   false,
		},
		{
			Path:    "/library/book/Disc 2/01.mp3",
			RelPath: "book/Disc 2/01.mp3",
			IsDir:   false,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	grouper := NewGrouper(logger)

	ctx := context.Background()
	grouped := grouper.Group(ctx, files, GroupOptions{})

	// Should be grouped together.
	if len(grouped) != 1 {
		t.Fatalf("expected 1 group, got %d", len(grouped))
	}

	group, exists := grouped["/library/book"]
	if !exists {
		t.Fatal("expected group with key '/library/book'")
	}

	if len(group) != 2 {
		t.Errorf("expected 2 files in group, got %d", len(group))
	}
}

func TestGrouper_Group_IgnoresNonAudioFiles(t *testing.T) {
	// Non-audio files should still be included in groups.
	// (they might be cover art, metadata, etc.).
	files := []WalkResult{
		{
			Path:    "/library/book/audio.mp3",
			RelPath: "book/audio.mp3",
			IsDir:   false,
		},
		{
			Path:    "/library/book/cover.jpg",
			RelPath: "book/cover.jpg",
			IsDir:   false,
		},
		{
			Path:    "/library/book/metadata.json",
			RelPath: "book/metadata.json",
			IsDir:   false,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	grouper := NewGrouper(logger)

	ctx := context.Background()
	grouped := grouper.Group(ctx, files, GroupOptions{})

	if len(grouped) != 1 {
		t.Fatalf("expected 1 group, got %d", len(grouped))
	}

	group := grouped["/library/book"]
	// All files should be included.
	if len(group) != 3 {
		t.Errorf("expected 3 files in group, got %d", len(group))
	}
}
