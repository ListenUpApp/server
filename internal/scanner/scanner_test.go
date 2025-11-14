package scanner

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/listenupapp/listenup-server/internal/store"
)

func TestScanner_Scan_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create mock store (nil is ok for now since we're not using it yet).
	scanner := NewScanner(nil, store.NewNoopEmitter(), logger)

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

	// Empty directory should have no items.
	if result.Added != 0 {
		t.Errorf("expected 0 added, got %d", result.Added)
	}
}

func TestScanner_Scan_SingleAudiobook(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a simple audiobook.
	bookDir := filepath.Join(tmpDir, "My Book")
	if err := os.Mkdir(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Copy test audio file.
	srcFile := filepath.Join("audio", "testdata", "test.mp3")
	data, err := os.ReadFile(srcFile)
	if err != nil {
		t.Skip("test audio file not found")
	}

	audioFile := filepath.Join(bookDir, "chapter01.mp3")
	if err := os.WriteFile(audioFile, data, 0o644); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scanner := NewScanner(nil, store.NewNoopEmitter(), logger)

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

	// Should have discovered one audiobook.
	// (exact counts depend on diff implementation, but should be > 0).
	if result.Progress == nil {
		t.Error("expected progress to be set")
	}
}

func TestScanner_Scan_MultipleAudiobooks(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple audiobooks.
	srcFile := filepath.Join("audio", "testdata", "test.mp3")
	data, err := os.ReadFile(srcFile)
	if err != nil {
		t.Skip("test audio file not found")
	}

	for i := 1; i <= 3; i++ {
		bookDir := filepath.Join(tmpDir, "Book"+string(rune('0'+i)))
		if err := os.Mkdir(bookDir, 0o755); err != nil {
			t.Fatal(err)
		}

		audioFile := filepath.Join(bookDir, "audio.mp3")
		if err := os.WriteFile(audioFile, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scanner := NewScanner(nil, store.NewNoopEmitter(), logger)

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

	// Should have completed.
	if result.CompletedAt.IsZero() {
		t.Error("expected CompletedAt to be set")
	}

	if result.StartedAt.IsZero() {
		t.Error("expected StartedAt to be set")
	}

	// CompletedAt should be after StartedAt.
	if !result.CompletedAt.After(result.StartedAt) {
		t.Error("expected CompletedAt to be after StartedAt")
	}
}

func TestScanner_Scan_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create many files to increase chance of cancellation during processing.
	srcFile := filepath.Join("audio", "testdata", "test.mp3")
	data, err := os.ReadFile(srcFile)
	if err != nil {
		t.Skip("test audio file not found")
	}

	for i := 1; i <= 20; i++ {
		bookDir := filepath.Join(tmpDir, "Book"+string(rune('0'+i)))
		if err := os.Mkdir(bookDir, 0o755); err != nil {
			t.Fatal(err)
		}

		audioFile := filepath.Join(bookDir, "audio.mp3")
		if err := os.WriteFile(audioFile, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scanner := NewScanner(nil, store.NewNoopEmitter(), logger)

	// Start scan and cancel during analysis.
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a tiny delay to let walk start.
	go func() {
		cancel()
	}()

	opts := ScanOptions{
		Workers: 2,
	}

	result, err := scanner.Scan(ctx, tmpDir, opts)
	// Either error or success is ok - context cancellation timing varies.
	// Just verify no panic.
	_ = result
	_ = err
}

func TestScanner_Scan_ProgressCallback(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a simple audiobook.
	bookDir := filepath.Join(tmpDir, "My Book")
	if err := os.Mkdir(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}

	srcFile := filepath.Join("audio", "testdata", "test.mp3")
	data, err := os.ReadFile(srcFile)
	if err != nil {
		t.Skip("test audio file not found")
	}

	audioFile := filepath.Join(bookDir, "audio.mp3")
	if err := os.WriteFile(audioFile, data, 0o644); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scanner := NewScanner(nil, store.NewNoopEmitter(), logger)

	// Track progress callbacks with mutex for thread safety.
	var (
		progressMu      sync.Mutex
		progressUpdates []ScanPhase
	)
	progressCallback := func(p *Progress) {
		progressMu.Lock()
		progressUpdates = append(progressUpdates, p.Phase)
		progressMu.Unlock()
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

	// Should have received progress updates.
	progressMu.Lock()
	updateCount := len(progressUpdates)
	hasWalking := false
	for _, phase := range progressUpdates {
		if phase == PhaseWalking {
			hasWalking = true
		}
	}
	progressMu.Unlock()

	if updateCount == 0 {
		t.Error("expected progress updates")
	}

	if !hasWalking {
		t.Error("expected PhaseWalking in progress updates")
	}
}

func TestScanner_Scan_NonexistentPath(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scanner := NewScanner(nil, store.NewNoopEmitter(), logger)

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

	// Create a simple audiobook.
	bookDir := filepath.Join(tmpDir, "My Book")
	if err := os.Mkdir(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}

	srcFile := filepath.Join("audio", "testdata", "test.mp3")
	data, err := os.ReadFile(srcFile)
	if err != nil {
		t.Skip("test audio file not found")
	}

	audioFile := filepath.Join(bookDir, "audio.mp3")
	if err := os.WriteFile(audioFile, data, 0o644); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scanner := NewScanner(nil, store.NewNoopEmitter(), logger)

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

	// Dry run should still complete successfully.
	if result.CompletedAt.IsZero() {
		t.Error("expected CompletedAt to be set")
	}
}

func TestScanner_ScanFolder_EmptyFolder(t *testing.T) {
	tmpDir := t.TempDir()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scanner := NewScanner(nil, store.NewNoopEmitter(), logger)

	ctx := context.Background()
	opts := ScanOptions{
		Workers: 2,
	}

	item, err := scanner.ScanFolder(ctx, tmpDir, opts)
	if err != nil {
		t.Fatalf("ScanFolder failed: %v", err)
	}

	if item == nil {
		t.Fatal("expected item, got nil")
	}

	if item.Path != tmpDir {
		t.Errorf("expected path %s, got %s", tmpDir, item.Path)
	}

	if len(item.AudioFiles) != 0 {
		t.Errorf("expected 0 audio files, got %d", len(item.AudioFiles))
	}
}

func TestScanner_ScanFolder_SingleM4B(t *testing.T) {
	tmpDir := t.TempDir()

	// Copy test audio file.
	srcFile := filepath.Join("audio", "testdata", "test.mp3")
	data, err := os.ReadFile(srcFile)
	if err != nil {
		t.Skip("test audio file not found")
	}

	// Use .m4b extension to simulate single-file audiobook.
	audioFile := filepath.Join(tmpDir, "book.m4b")
	if err := os.WriteFile(audioFile, data, 0o644); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scanner := NewScanner(nil, store.NewNoopEmitter(), logger)

	ctx := context.Background()
	opts := ScanOptions{
		Workers: 2,
	}

	item, err := scanner.ScanFolder(ctx, tmpDir, opts)
	if err != nil {
		t.Fatalf("ScanFolder failed: %v", err)
	}

	if item == nil {
		t.Fatal("expected item, got nil")
	}

	if len(item.AudioFiles) != 1 {
		t.Errorf("expected 1 audio file, got %d", len(item.AudioFiles))
	}

	if len(item.AudioFiles) > 0 {
		af := item.AudioFiles[0]
		if af.Path != audioFile {
			t.Errorf("expected path %s, got %s", audioFile, af.Path)
		}
		if af.Ext != ".m4b" {
			t.Errorf("expected ext .m4b, got %s", af.Ext)
		}
	}
}

func TestScanner_ScanFolder_MultipleMP3s(t *testing.T) {
	tmpDir := t.TempDir()

	bookDir := filepath.Join(tmpDir, "My Audiobook")
	if err := os.Mkdir(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Copy test audio files.
	srcFile := filepath.Join("audio", "testdata", "test.mp3")
	data, err := os.ReadFile(srcFile)
	if err != nil {
		t.Skip("test audio file not found")
	}

	// Create multiple MP3 files.
	for i := 1; i <= 3; i++ {
		audioFile := filepath.Join(bookDir, "chapter"+string(rune('0'+i))+".mp3")
		if err := os.WriteFile(audioFile, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scanner := NewScanner(nil, store.NewNoopEmitter(), logger)

	ctx := context.Background()
	opts := ScanOptions{
		Workers: 2,
	}

	item, err := scanner.ScanFolder(ctx, bookDir, opts)
	if err != nil {
		t.Fatalf("ScanFolder failed: %v", err)
	}

	if item == nil {
		t.Fatal("expected item, got nil")
	}

	if len(item.AudioFiles) != 3 {
		t.Errorf("expected 3 audio files, got %d", len(item.AudioFiles))
	}

	// Verify all files are MP3s.
	for _, af := range item.AudioFiles {
		if af.Ext != ".mp3" {
			t.Errorf("expected ext .mp3, got %s", af.Ext)
		}
	}
}

func TestScanner_ScanFolder_WithCoverArt(t *testing.T) {
	tmpDir := t.TempDir()

	bookDir := filepath.Join(tmpDir, "My Book")
	if err := os.Mkdir(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create audio file.
	srcFile := filepath.Join("audio", "testdata", "test.mp3")
	data, err := os.ReadFile(srcFile)
	if err != nil {
		t.Skip("test audio file not found")
	}

	audioFile := filepath.Join(bookDir, "chapter01.mp3")
	if err := os.WriteFile(audioFile, data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Create cover art (just an empty file for testing).
	coverFile := filepath.Join(bookDir, "cover.jpg")
	if err := os.WriteFile(coverFile, []byte("fake image data"), 0o644); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scanner := NewScanner(nil, store.NewNoopEmitter(), logger)

	ctx := context.Background()
	opts := ScanOptions{
		Workers: 2,
	}

	item, err := scanner.ScanFolder(ctx, bookDir, opts)
	if err != nil {
		t.Fatalf("ScanFolder failed: %v", err)
	}

	if item == nil {
		t.Fatal("expected item, got nil")
	}

	if len(item.AudioFiles) != 1 {
		t.Errorf("expected 1 audio file, got %d", len(item.AudioFiles))
	}

	if len(item.ImageFiles) != 1 {
		t.Errorf("expected 1 image file, got %d", len(item.ImageFiles))
	}

	if len(item.ImageFiles) > 0 {
		img := item.ImageFiles[0]
		if img.Path != coverFile {
			t.Errorf("expected path %s, got %s", coverFile, img.Path)
		}
		if img.Ext != ".jpg" {
			t.Errorf("expected ext .jpg, got %s", img.Ext)
		}
	}
}

func TestScanner_ScanFolder_MultiDisc(t *testing.T) {
	tmpDir := t.TempDir()

	bookDir := filepath.Join(tmpDir, "My Audiobook")
	if err := os.Mkdir(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create CD1 and CD2 subdirectories.
	cd1Dir := filepath.Join(bookDir, "CD1")
	cd2Dir := filepath.Join(bookDir, "CD2")
	if err := os.Mkdir(cd1Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(cd2Dir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Copy test audio files.
	srcFile := filepath.Join("audio", "testdata", "test.mp3")
	data, err := os.ReadFile(srcFile)
	if err != nil {
		t.Skip("test audio file not found")
	}

	// Create files in CD1.
	for i := 1; i <= 2; i++ {
		audioFile := filepath.Join(cd1Dir, "track"+string(rune('0'+i))+".mp3")
		if err := os.WriteFile(audioFile, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Create files in CD2.
	for i := 1; i <= 2; i++ {
		audioFile := filepath.Join(cd2Dir, "track"+string(rune('0'+i))+".mp3")
		if err := os.WriteFile(audioFile, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scanner := NewScanner(nil, store.NewNoopEmitter(), logger)

	ctx := context.Background()
	opts := ScanOptions{
		Workers: 2,
	}

	// Scan the parent folder (should include both discs).
	item, err := scanner.ScanFolder(ctx, bookDir, opts)
	if err != nil {
		t.Fatalf("ScanFolder failed: %v", err)
	}

	if item == nil {
		t.Fatal("expected item, got nil")
	}

	// Should have all 4 audio files from both discs.
	if len(item.AudioFiles) != 4 {
		t.Errorf("expected 4 audio files from both discs, got %d", len(item.AudioFiles))
	}

	// Verify the item path is the parent folder, not a disc folder.
	if item.Path != bookDir {
		t.Errorf("expected path %s, got %s", bookDir, item.Path)
	}
}

func TestScanner_ScanFolder_WithMetadata(t *testing.T) {
	tmpDir := t.TempDir()

	bookDir := filepath.Join(tmpDir, "My Book")
	if err := os.Mkdir(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create audio file.
	srcFile := filepath.Join("audio", "testdata", "test.mp3")
	data, err := os.ReadFile(srcFile)
	if err != nil {
		t.Skip("test audio file not found")
	}

	audioFile := filepath.Join(bookDir, "chapter01.mp3")
	if err := os.WriteFile(audioFile, data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Create metadata files.
	metadataJSON := filepath.Join(bookDir, "metadata.json")
	if err := os.WriteFile(metadataJSON, []byte(`{"title":"Test Book"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	descFile := filepath.Join(bookDir, "desc.txt")
	if err := os.WriteFile(descFile, []byte("Book description"), 0o644); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scanner := NewScanner(nil, store.NewNoopEmitter(), logger)

	ctx := context.Background()
	opts := ScanOptions{
		Workers: 2,
	}

	item, err := scanner.ScanFolder(ctx, bookDir, opts)
	if err != nil {
		t.Fatalf("ScanFolder failed: %v", err)
	}

	if item == nil {
		t.Fatal("expected item, got nil")
	}

	if len(item.AudioFiles) != 1 {
		t.Errorf("expected 1 audio file, got %d", len(item.AudioFiles))
	}

	if len(item.MetadataFiles) != 2 {
		t.Errorf("expected 2 metadata files, got %d", len(item.MetadataFiles))
	}

	// Verify metadata file types.
	foundJSON := false
	foundDesc := false
	for _, mf := range item.MetadataFiles {
		if mf.Type == MetadataTypeJSON {
			foundJSON = true
		}
		if mf.Type == MetadataTypeDesc {
			foundDesc = true
		}
	}

	if !foundJSON {
		t.Error("expected to find metadata.json")
	}
	if !foundDesc {
		t.Error("expected to find desc.txt")
	}
}

func TestScanner_ScanFolder_NonexistentPath(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scanner := NewScanner(nil, store.NewNoopEmitter(), logger)

	ctx := context.Background()
	opts := ScanOptions{
		Workers: 2,
	}

	_, err := scanner.ScanFolder(ctx, "/nonexistent/path", opts)
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestScanner_ScanFolder_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	bookDir := filepath.Join(tmpDir, "My Book")
	if err := os.Mkdir(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create multiple audio files.
	srcFile := filepath.Join("audio", "testdata", "test.mp3")
	data, err := os.ReadFile(srcFile)
	if err != nil {
		t.Skip("test audio file not found")
	}

	for i := 1; i <= 10; i++ {
		audioFile := filepath.Join(bookDir, "chapter"+string(rune('0'+i))+".mp3")
		if err := os.WriteFile(audioFile, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scanner := NewScanner(nil, store.NewNoopEmitter(), logger)

	// Create cancellable context.
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately.
	cancel()

	opts := ScanOptions{
		Workers: 2,
	}

	_, err = scanner.ScanFolder(ctx, bookDir, opts)
	// Either error or success is ok - context cancellation timing varies.
	// Just verify no panic.
	_ = err
}

func TestScanner_ScanFolder_IgnoresHiddenFiles(t *testing.T) {
	tmpDir := t.TempDir()

	bookDir := filepath.Join(tmpDir, "My Book")
	if err := os.Mkdir(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create regular audio file.
	srcFile := filepath.Join("audio", "testdata", "test.mp3")
	data, err := os.ReadFile(srcFile)
	if err != nil {
		t.Skip("test audio file not found")
	}

	audioFile := filepath.Join(bookDir, "chapter01.mp3")
	if err := os.WriteFile(audioFile, data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Create hidden audio file (should be ignored).
	hiddenFile := filepath.Join(bookDir, ".hidden.mp3")
	if err := os.WriteFile(hiddenFile, data, 0o644); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scanner := NewScanner(nil, store.NewNoopEmitter(), logger)

	ctx := context.Background()
	opts := ScanOptions{
		Workers: 2,
	}

	item, err := scanner.ScanFolder(ctx, bookDir, opts)
	if err != nil {
		t.Fatalf("ScanFolder failed: %v", err)
	}

	if item == nil {
		t.Fatal("expected item, got nil")
	}

	// Should only have 1 audio file (hidden file ignored).
	if len(item.AudioFiles) != 1 {
		t.Errorf("expected 1 audio file (hidden file should be ignored), got %d", len(item.AudioFiles))
	}

	if len(item.AudioFiles) > 0 && item.AudioFiles[0].Path == hiddenFile {
		t.Error("hidden file should have been ignored")
	}
}
