package scanner

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWalker_Walk_EmptyDirectory(t *testing.T) {
	// Setup: Create empty directory
	tmpDir := t.TempDir()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	walker := NewWalker(logger)

	ctx := context.Background()
	resultsCh := walker.Walk(ctx, tmpDir)

	// Collect results
	var results []WalkResult
	for result := range resultsCh {
		if result.Error != nil {
			t.Errorf("unexpected error: %v", result.Error)
		}
		results = append(results, result)
	}

	// Should have no results (empty directory)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestWalker_Walk_SingleFile(t *testing.T) {
	// Setup: Create directory with one file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	walker := NewWalker(logger)

	ctx := context.Background()
	resultsCh := walker.Walk(ctx, tmpDir)

	// Collect results
	var results []WalkResult
	for result := range resultsCh {
		if result.Error != nil {
			t.Errorf("unexpected error: %v", result.Error)
		}
		results = append(results, result)
	}

	// Should have exactly 1 file
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	result := results[0]

	// Verify path
	if result.Path != testFile {
		t.Errorf("expected path %s, got %s", testFile, result.Path)
	}

	// Verify it's not a directory
	if result.IsDir {
		t.Error("expected file, got directory")
	}

	// Verify size
	if result.Size != 5 {
		t.Errorf("expected size 5, got %d", result.Size)
	}

	// Verify inode is set
	if result.Inode == 0 {
		t.Error("expected inode to be set")
	}
}

func TestWalker_Walk_SkipsHiddenFiles(t *testing.T) {
	// Setup: Create directory with hidden file
	tmpDir := t.TempDir()

	// Regular file
	regularFile := filepath.Join(tmpDir, "regular.txt")
	if err := os.WriteFile(regularFile, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	// Hidden file (starts with .)
	hiddenFile := filepath.Join(tmpDir, ".hidden.txt")
	if err := os.WriteFile(hiddenFile, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	walker := NewWalker(logger)

	ctx := context.Background()
	resultsCh := walker.Walk(ctx, tmpDir)

	// Collect results
	var results []WalkResult
	for result := range resultsCh {
		if result.Error != nil {
			t.Errorf("unexpected error: %v", result.Error)
		}
		results = append(results, result)
	}

	// Should have exactly 1 file (hidden file should be skipped)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Verify it's the regular file, not the hidden one
	if results[0].Path != regularFile {
		t.Errorf("expected regular file, got %s", results[0].Path)
	}
}

func TestWalker_Walk_NestedDirectories(t *testing.T) {
	// Setup: Create nested directory structure
	tmpDir := t.TempDir()

	// Create structure:
	// tmpDir/
	//   file1.txt
	//   subdir/
	//     file2.txt
	//     deep/
	//       file3.txt

	file1 := filepath.Join(tmpDir, "file1.txt")
	if err := os.WriteFile(file1, []byte("1"), 0644); err != nil {
		t.Fatal(err)
	}

	subdir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatal(err)
	}

	file2 := filepath.Join(subdir, "file2.txt")
	if err := os.WriteFile(file2, []byte("2"), 0644); err != nil {
		t.Fatal(err)
	}

	deep := filepath.Join(subdir, "deep")
	if err := os.Mkdir(deep, 0755); err != nil {
		t.Fatal(err)
	}

	file3 := filepath.Join(deep, "file3.txt")
	if err := os.WriteFile(file3, []byte("3"), 0644); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	walker := NewWalker(logger)

	ctx := context.Background()
	resultsCh := walker.Walk(ctx, tmpDir)

	// Collect results
	var results []WalkResult
	for result := range resultsCh {
		if result.Error != nil {
			t.Errorf("unexpected error: %v", result.Error)
		}
		results = append(results, result)
	}

	// Should have 3 files (directories are not included)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Verify all files are present
	paths := make(map[string]bool)
	for _, r := range results {
		paths[r.Path] = true
	}

	if !paths[file1] {
		t.Error("missing file1.txt")
	}
	if !paths[file2] {
		t.Error("missing file2.txt")
	}
	if !paths[file3] {
		t.Error("missing file3.txt")
	}
}

func TestWalker_Walk_ContextCancellation(t *testing.T) {
	// Setup: Create directory with files
	tmpDir := t.TempDir()

	for i := 0; i < 10; i++ {
		filename := filepath.Join(tmpDir, filepath.FromSlash("file"+string(rune('0'+i))+".txt"))
		if err := os.WriteFile(filename, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	walker := NewWalker(logger)

	// Create context that cancels immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	resultsCh := walker.Walk(ctx, tmpDir)

	// Collect results (should be none or very few)
	var results []WalkResult
	for result := range resultsCh {
		results = append(results, result)
	}

	// With immediate cancellation, we should get 0 results or very few
	// (depends on timing, but definitely not all 10)
	if len(results) > 5 {
		t.Errorf("expected few or no results due to cancellation, got %d", len(results))
	}
}

func TestWalker_Walk_RelativePath(t *testing.T) {
	// Setup: Create directory structure
	tmpDir := t.TempDir()

	subdir := filepath.Join(tmpDir, "books")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatal(err)
	}

	file := filepath.Join(subdir, "book.mp3")
	if err := os.WriteFile(file, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	walker := NewWalker(logger)

	ctx := context.Background()
	resultsCh := walker.Walk(ctx, tmpDir)

	// Collect results
	var results []WalkResult
	for result := range resultsCh {
		if result.Error != nil {
			t.Errorf("unexpected error: %v", result.Error)
		}
		results = append(results, result)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	result := results[0]

	// RelPath should be relative to root
	expectedRelPath := filepath.Join("books", "book.mp3")
	if result.RelPath != expectedRelPath {
		t.Errorf("expected RelPath %s, got %s", expectedRelPath, result.RelPath)
	}
}

func TestWalker_Walk_ModTime(t *testing.T) {
	// Setup: Create file and verify modtime is captured
	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "test.txt")
	beforeWrite := time.Now()
	if err := os.WriteFile(testFile, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	afterWrite := time.Now()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	walker := NewWalker(logger)

	ctx := context.Background()
	resultsCh := walker.Walk(ctx, tmpDir)

	// Collect results
	var results []WalkResult
	for result := range resultsCh {
		if result.Error != nil {
			t.Errorf("unexpected error: %v", result.Error)
		}
		results = append(results, result)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	result := results[0]

	// ModTime should be in Unix milliseconds and within the time window
	modTime := time.UnixMilli(result.ModTime)

	// Allow 2 seconds of tolerance (file systems can have varying precision)
	if modTime.Before(beforeWrite.Add(-time.Second)) || modTime.After(afterWrite.Add(2*time.Second)) {
		t.Errorf("modTime %v not in expected range [%v, %v]", modTime, beforeWrite, afterWrite)
	}
}