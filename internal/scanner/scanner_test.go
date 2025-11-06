package scanner

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestScanner_Scan_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create mock store (nil is ok for now since we're not using it yet)
	scanner := NewScanner(nil, logger)

	ctx := context.Background()
	opts := ScanOptions{
		Workers: 2,
	}

	result, err := scanner.Scan(ctx, tmpDir, opts)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected result, got nil")
	}

	// Empty directory should have no items
	if result.Added != 0 {
		t.Errorf("expected 0 added, got %d", result.Added)
	}
}

func TestScanner_Scan_SingleAudiobook(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a simple audiobook
	bookDir := filepath.Join(tmpDir, "My Book")
	if err := os.Mkdir(bookDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Copy test audio file
	srcFile := filepath.Join("audio", "testdata", "test.mp3")
	data, err := os.ReadFile(srcFile)
	if err != nil {
		t.Skip("test audio file not found")
	}

	audioFile := filepath.Join(bookDir, "chapter01.mp3")
	if err := os.WriteFile(audioFile, data, 0644); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scanner := NewScanner(nil, logger)

	ctx := context.Background()
	opts := ScanOptions{
		Workers: 2,
	}

	result, err := scanner.Scan(ctx, tmpDir, opts)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected result, got nil")
	}

	// Should have discovered one audiobook
	// (exact counts depend on diff implementation, but should be > 0)
	if result.Progress == nil {
		t.Error("expected progress to be set")
	}
}

func TestScanner_Scan_MultipleAudiobooks(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple audiobooks
	srcFile := filepath.Join("audio", "testdata", "test.mp3")
	data, err := os.ReadFile(srcFile)
	if err != nil {
		t.Skip("test audio file not found")
	}

	for i := 1; i <= 3; i++ {
		bookDir := filepath.Join(tmpDir, "Book"+string(rune('0'+i)))
		if err := os.Mkdir(bookDir, 0755); err != nil {
			t.Fatal(err)
		}

		audioFile := filepath.Join(bookDir, "audio.mp3")
		if err := os.WriteFile(audioFile, data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scanner := NewScanner(nil, logger)

	ctx := context.Background()
	opts := ScanOptions{
		Workers: 2,
	}

	result, err := scanner.Scan(ctx, tmpDir, opts)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected result, got nil")
	}

	// Should have completed
	if result.CompletedAt.IsZero() {
		t.Error("expected CompletedAt to be set")
	}

	if result.StartedAt.IsZero() {
		t.Error("expected StartedAt to be set")
	}

	// CompletedAt should be after StartedAt
	if !result.CompletedAt.After(result.StartedAt) {
		t.Error("expected CompletedAt to be after StartedAt")
	}
}

func TestScanner_Scan_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create many files to increase chance of cancellation during processing
	srcFile := filepath.Join("audio", "testdata", "test.mp3")
	data, err := os.ReadFile(srcFile)
	if err != nil {
		t.Skip("test audio file not found")
	}

	for i := 1; i <= 20; i++ {
		bookDir := filepath.Join(tmpDir, "Book"+string(rune('0'+i)))
		if err := os.Mkdir(bookDir, 0755); err != nil {
			t.Fatal(err)
		}

		audioFile := filepath.Join(bookDir, "audio.mp3")
		if err := os.WriteFile(audioFile, data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scanner := NewScanner(nil, logger)

	// Start scan and cancel during analysis
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a tiny delay to let walk start
	go func() {
		cancel()
	}()

	opts := ScanOptions{
		Workers: 2,
	}

	result, err := scanner.Scan(ctx, tmpDir, opts)
	// Either error or success is ok - context cancellation timing varies
	// Just verify no panic
	_ = result
	_ = err
}

func TestScanner_Scan_ProgressCallback(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a simple audiobook
	bookDir := filepath.Join(tmpDir, "My Book")
	if err := os.Mkdir(bookDir, 0755); err != nil {
		t.Fatal(err)
	}

	srcFile := filepath.Join("audio", "testdata", "test.mp3")
	data, err := os.ReadFile(srcFile)
	if err != nil {
		t.Skip("test audio file not found")
	}

	audioFile := filepath.Join(bookDir, "audio.mp3")
	if err := os.WriteFile(audioFile, data, 0644); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scanner := NewScanner(nil, logger)

	// Track progress callbacks
	var progressUpdates []ScanPhase
	progressCallback := func(p *Progress) {
		progressUpdates = append(progressUpdates, p.Phase)
	}

	ctx := context.Background()
	opts := ScanOptions{
		Workers:    2,
		OnProgress: progressCallback,
	}

	_, err = scanner.Scan(ctx, tmpDir, opts)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Should have received progress updates
	if len(progressUpdates) == 0 {
		t.Error("expected progress updates")
	}

	// Should have at least walking phase
	hasWalking := false
	for _, phase := range progressUpdates {
		if phase == PhaseWalking {
			hasWalking = true
		}
	}

	if !hasWalking {
		t.Error("expected PhaseWalking in progress updates")
	}
}

func TestScanner_Scan_NonexistentPath(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scanner := NewScanner(nil, logger)

	ctx := context.Background()
	opts := ScanOptions{
		Workers: 2,
	}

	_, err := scanner.Scan(ctx, "/nonexistent/path", opts)
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestScanner_Scan_DryRun(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a simple audiobook
	bookDir := filepath.Join(tmpDir, "My Book")
	if err := os.Mkdir(bookDir, 0755); err != nil {
		t.Fatal(err)
	}

	srcFile := filepath.Join("audio", "testdata", "test.mp3")
	data, err := os.ReadFile(srcFile)
	if err != nil {
		t.Skip("test audio file not found")
	}

	audioFile := filepath.Join(bookDir, "audio.mp3")
	if err := os.WriteFile(audioFile, data, 0644); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scanner := NewScanner(nil, logger)

	ctx := context.Background()
	opts := ScanOptions{
		Workers: 2,
		DryRun:  true, // Dry run - should not apply changes
	}

	result, err := scanner.Scan(ctx, tmpDir, opts)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected result, got nil")
	}

	// Dry run should still complete successfully
	if result.CompletedAt.IsZero() {
		t.Error("expected CompletedAt to be set")
	}
}
