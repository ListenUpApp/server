package scanner

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzer_Analyze_SingleFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test audio file.
	testFile := filepath.Join(tmpDir, "test.m4b")
	// Copy from testdata.
	srcFile := filepath.Join("audio", "testdata", "test.m4b")
	data, err := os.ReadFile(srcFile)
	if err != nil {
		t.Skip("test audio file not found")
	}
	if err := os.WriteFile(testFile, data, 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(testFile)
	if err != nil {
		t.Fatal(err)
	}

	files := []AudioFileData{
		{
			Path:     testFile,
			RelPath:  "test.m4b",
			Filename: "test.m4b",
			Ext:      ".m4b",
			Size:     info.Size(),
			ModTime:  info.ModTime(),
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	analyzer := NewAnalyzer(logger)

	ctx := context.Background()
	opts := AnalyzeOptions{
		Workers: 1,
	}

	result, err := analyzer.Analyze(ctx, files, opts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	// Verify metadata was populated.
	if result[0].Metadata == nil {
		t.Error("expected metadata to be populated")
	}

	if result[0].Metadata.Title != "Test Book" {
		t.Errorf("expected title 'Test Book', got '%s'", result[0].Metadata.Title)
	}
}

func TestAnalyzer_Analyze_MultipleFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Copy test file multiple times.
	srcFile := filepath.Join("audio", "testdata", "test.m4b")
	data, err := os.ReadFile(srcFile)
	if err != nil {
		t.Skip("test audio file not found")
	}

	var files []AudioFileData
	for i := 1; i <= 5; i++ {
		filename := filepath.Join(tmpDir, "test"+string(rune('0'+i))+".m4b")
		if err := os.WriteFile(filename, data, 0o644); err != nil {
			t.Fatal(err)
		}

		info, err := os.Stat(filename)
		if err != nil {
			t.Fatal(err)
		}

		files = append(files, AudioFileData{
			Path:     filename,
			RelPath:  filepath.Base(filename),
			Filename: filepath.Base(filename),
			Ext:      ".m4b",
			Size:     info.Size(),
			ModTime:  info.ModTime(),
		})
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	analyzer := NewAnalyzer(logger)

	ctx := context.Background()
	opts := AnalyzeOptions{
		Workers: 2, // Use multiple workers
	}

	result, err := analyzer.Analyze(ctx, files, opts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if len(result) != 5 {
		t.Fatalf("expected 5 results, got %d", len(result))
	}

	// Verify all have metadata.
	for i, r := range result {
		if r.Metadata == nil {
			t.Errorf("file %d: expected metadata to be populated", i)
		}
	}
}

func TestAnalyzer_Analyze_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	// Copy test file multiple times.
	srcFile := filepath.Join("audio", "testdata", "test.m4b")
	data, err := os.ReadFile(srcFile)
	if err != nil {
		t.Skip("test audio file not found")
	}

	var files []AudioFileData
	for i := 1; i <= 10; i++ {
		filename := filepath.Join(tmpDir, "test"+string(rune('0'+i))+".m4b")
		if err := os.WriteFile(filename, data, 0o644); err != nil {
			t.Fatal(err)
		}

		info, err := os.Stat(filename)
		if err != nil {
			t.Fatal(err)
		}

		files = append(files, AudioFileData{
			Path:     filename,
			RelPath:  filepath.Base(filename),
			Filename: filepath.Base(filename),
			Ext:      ".m4b",
			Size:     info.Size(),
			ModTime:  info.ModTime(),
		})
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	analyzer := NewAnalyzer(logger)

	// Cancel context immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	opts := AnalyzeOptions{
		Workers: 2,
	}

	_, err = analyzer.Analyze(ctx, files, opts)
	if err == nil {
		t.Error("expected error for canceled context, got nil")
	}
}

func TestAnalyzer_Analyze_InvalidFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create invalid audio file.
	invalidFile := filepath.Join(tmpDir, "invalid.mp3")
	if err := os.WriteFile(invalidFile, []byte("not audio"), 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(invalidFile)
	if err != nil {
		t.Fatal(err)
	}

	files := []AudioFileData{
		{
			Path:     invalidFile,
			RelPath:  "invalid.mp3",
			Filename: "invalid.mp3",
			Ext:      ".mp3",
			Size:     info.Size(),
			ModTime:  info.ModTime(),
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	analyzer := NewAnalyzer(logger)

	ctx := context.Background()
	opts := AnalyzeOptions{
		Workers: 1,
	}

	result, err := analyzer.Analyze(ctx, files, opts)
	if err != nil {
		t.Fatalf("Analyze should not fail on invalid file: %v", err)
	}

	// Should return the file but without metadata.
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	// Metadata should be nil for invalid file.
	if result[0].Metadata != nil {
		t.Error("expected metadata to be nil for invalid file")
	}
}

func TestAnalyzer_Analyze_EmptyList(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	analyzer := NewAnalyzer(logger)

	ctx := context.Background()
	opts := AnalyzeOptions{
		Workers: 1,
	}

	result, err := analyzer.Analyze(ctx, []AudioFileData{}, opts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}
}

func TestAnalyzer_Analyze_DefaultWorkers(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test file.
	srcFile := filepath.Join("audio", "testdata", "test.m4b")
	data, err := os.ReadFile(srcFile)
	if err != nil {
		t.Skip("test audio file not found")
	}

	testFile := filepath.Join(tmpDir, "test.m4b")
	if err := os.WriteFile(testFile, data, 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(testFile)
	if err != nil {
		t.Fatal(err)
	}

	files := []AudioFileData{
		{
			Path:     testFile,
			RelPath:  "test.m4b",
			Filename: "test.m4b",
			Ext:      ".m4b",
			Size:     info.Size(),
			ModTime:  info.ModTime(),
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	analyzer := NewAnalyzer(logger)

	ctx := context.Background()
	opts := AnalyzeOptions{
		Workers: 0, // Should default to runtime.NumCPU()
	}

	result, err := analyzer.Analyze(ctx, files, opts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	if result[0].Metadata == nil {
		t.Error("expected metadata to be populated")
	}
}

func TestAnalyzer_Analyze_PreservesOrder(t *testing.T) {
	tmpDir := t.TempDir()

	// Copy test file multiple times.
	srcFile := filepath.Join("audio", "testdata", "test.m4b")
	data, err := os.ReadFile(srcFile)
	if err != nil {
		t.Skip("test audio file not found")
	}

	var files []AudioFileData
	for i := 1; i <= 5; i++ {
		filename := filepath.Join(tmpDir, "test"+string(rune('0'+i))+".m4b")
		if err := os.WriteFile(filename, data, 0o644); err != nil {
			t.Fatal(err)
		}

		info, err := os.Stat(filename)
		if err != nil {
			t.Fatal(err)
		}

		files = append(files, AudioFileData{
			Path:     filename,
			RelPath:  filepath.Base(filename),
			Filename: filepath.Base(filename),
			Ext:      ".m4b",
			Size:     info.Size(),
			ModTime:  info.ModTime(),
		})
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	analyzer := NewAnalyzer(logger)

	ctx := context.Background()
	opts := AnalyzeOptions{
		Workers: 3, // Multiple workers to test ordering
	}

	result, err := analyzer.Analyze(ctx, files, opts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Verify order is preserved.
	for i, r := range result {
		if r.Path != files[i].Path {
			t.Errorf("order not preserved: expected %s at position %d, got %s",
				files[i].Path, i, r.Path)
		}
	}
}

// Benchmark to test performance.
func BenchmarkAnalyzer_Analyze(b *testing.B) {
	tmpDir := b.TempDir()

	// Copy test file.
	srcFile := filepath.Join("audio", "testdata", "test.m4b")
	data, err := os.ReadFile(srcFile)
	if err != nil {
		b.Skip("test audio file not found")
	}

	testFile := filepath.Join(tmpDir, "test.m4b")
	if err := os.WriteFile(testFile, data, 0o644); err != nil {
		b.Fatal(err)
	}

	info, err := os.Stat(testFile)
	if err != nil {
		b.Fatal(err)
	}

	files := []AudioFileData{
		{
			Path:     testFile,
			RelPath:  "test.m4b",
			Filename: "test.m4b",
			Ext:      ".m4b",
			Size:     info.Size(),
			ModTime:  info.ModTime(),
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	analyzer := NewAnalyzer(logger)

	ctx := context.Background()
	opts := AnalyzeOptions{
		Workers: 2,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := analyzer.Analyze(ctx, files, opts)
		if err != nil {
			b.Fatal(err)
		}
	}
}
package scanner

import (
	"testing"
)

// TestBuildBookMetadata_UsesAlbumForTitle verifies that buildBookMetadata
// uses the Album tag (book title) instead of Title tag (track/chapter title).
func TestBuildBookMetadata_UsesAlbumForTitle(t *testing.T) {
	tests := []struct {
		name          string
		audioMetadata *AudioMetadata
		expectedTitle string
	}{
		{
			name: "Album tag should be used for book title",
			audioMetadata: &AudioMetadata{
				Title: "01 - Prologue", // Track title (wrong)
				Album: "A Clash of Kings", // Book title (correct)
				Artist: "George R.R. Martin",
			},
			expectedTitle: "A Clash of Kings",
		},
		{
			name: "Handle missing Title tag (null)",
			audioMetadata: &AudioMetadata{
				Title: "", // Empty track title
				Album: "North! Or Be Eaten", // Book title
				Artist: "Andrew Peterson",
			},
			expectedTitle: "North! Or Be Eaten",
		},
		{
			name: "Chapter title should not be used",
			audioMetadata: &AudioMetadata{
				Title: "Chapter Four", // Chapter title (wrong)
				Album: "The Tower of Swallows", // Book title (correct)
				Artist: "Andrzej Sapkowski",
			},
			expectedTitle: "The Tower of Swallows",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildBookMetadata(tt.audioMetadata)
			if result == nil {
				t.Fatal("buildBookMetadata returned nil")
			}
			if result.Title != tt.expectedTitle {
				t.Errorf("Title = %q, want %q", result.Title, tt.expectedTitle)
			}
		})
	}
}
