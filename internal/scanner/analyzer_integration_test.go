package scanner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestAnalyzeItems_SingleFileChapteredMP3(t *testing.T) {
	dir := "/home/simonh/Music/Libation/Books/No Life Forsaken [B0DBJC226L]"

	// Find the MP3 file
	var mp3Path string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".mp3") {
			mp3Path = path
		}
		return nil
	})

	if mp3Path == "" {
		t.Skip("Test file not found")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	analyzer := NewAnalyzer(logger)

	// Create library item with single file
	item := LibraryItemData{
		Path: dir,
		AudioFiles: []AudioFileData{
			{Path: mp3Path},
		},
	}

	ctx := context.Background()
	results, err := analyzer.AnalyzeItems(ctx, []LibraryItemData{item})
	if err != nil {
		t.Fatalf("AnalyzeItems failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("No results returned")
	}

	if len(results[0].AudioFiles) == 0 || results[0].AudioFiles[0].Metadata == nil {
		t.Fatal("No metadata returned")
	}

	meta := results[0].AudioFiles[0].Metadata

	// Print results
	fmt.Printf("\n=== Single-File Chaptered MP3 ===\n")
	fmt.Printf("Title:    %s\n", meta.Title)
	fmt.Printf("Author:   %s\n", meta.Artist)
	fmt.Printf("Narrator: %s\n", meta.Narrator)
	fmt.Printf("Duration: %s\n", meta.Duration)
	fmt.Printf("Chapters: %d\n", len(meta.Chapters))

	if len(meta.Chapters) > 0 {
		fmt.Printf("\nFirst 5 chapters:\n")
		for i := 0; i < 5 && i < len(meta.Chapters); i++ {
			ch := meta.Chapters[i]
			fmt.Printf("  [%2d] %s - %s: %s\n", ch.ID, ch.StartTime, ch.EndTime, ch.Title)
		}
	}

	// Verify expectations
	if meta.Title == "" {
		t.Error("Expected title to be set")
	}
	if len(meta.Chapters) == 0 {
		t.Error("Expected chapters to be present")
	}
	if meta.Duration == 0 {
		t.Error("Expected non-zero duration")
	}

	fmt.Printf("\nClassification: Single-file ✓\n")
}

func TestAnalyzeItems_MultiFileAudiobook(t *testing.T) {
	dir := "/home/simonh/Music/Libation/Books/Harry Potter and the Philosopher’s Stone (Full-Cast Edition) [B0F14Y2YW7]"

	// Find all MP3 files
	var mp3Files []string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".mp3") {
			mp3Files = append(mp3Files, path)
		}
		return nil
	})

	if len(mp3Files) == 0 {
		t.Skip("Test files not found")
	}

	sort.Strings(mp3Files)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	analyzer := NewAnalyzer(logger)

	// Create library item with multiple files
	audioFiles := make([]AudioFileData, len(mp3Files))
	for i, path := range mp3Files {
		audioFiles[i] = AudioFileData{Path: path}
	}

	item := LibraryItemData{
		Path:       dir,
		AudioFiles: audioFiles,
	}

	ctx := context.Background()
	results, err := analyzer.AnalyzeItems(ctx, []LibraryItemData{item})
	if err != nil {
		t.Fatalf("AnalyzeItems failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("No results returned")
	}

	if len(results[0].AudioFiles) == 0 || results[0].AudioFiles[0].Metadata == nil {
		t.Fatal("No metadata returned")
	}

	meta := results[0].AudioFiles[0].Metadata

	// Print results
	fmt.Printf("\n=== Multi-File MP3 Audiobook ===\n")
	fmt.Printf("Title:      %s\n", meta.Album)
	fmt.Printf("Author:     %s\n", meta.Artist)
	fmt.Printf("Files:      %d\n", len(mp3Files))
	fmt.Printf("Duration:   %s\n", meta.Duration)
	fmt.Printf("Chapters:   %d\n", len(meta.Chapters))

	if len(meta.Chapters) > 0 {
		fmt.Printf("\nFirst 5 chapters:\n")
		for i := 0; i < 5 && i < len(meta.Chapters); i++ {
			ch := meta.Chapters[i]
			fmt.Printf("  [%2d] %s - %s: %s\n", ch.ID, ch.StartTime, ch.EndTime, ch.Title)
		}
	}

	// Verify expectations
	if meta.Album == "" {
		t.Error("Expected album to be set")
	}
	if len(meta.Chapters) == 0 {
		t.Error("Expected chapters to be present")
	}
	if meta.Duration == 0 {
		t.Error("Expected non-zero duration")
	}
	if len(meta.Chapters) != len(mp3Files) {
		t.Logf("Warning: Chapter count (%d) doesn't match file count (%d)", len(meta.Chapters), len(mp3Files))
	}

	fmt.Printf("\nClassification: Multi-file ✓\n")
	fmt.Printf("Aggregation:    %d files → 1 audiobook with %d chapters\n", len(mp3Files), len(meta.Chapters))
}

func TestAnalyzeItems_MusicAlbum(t *testing.T) {
	dir := "/home/simonh/Downloads/Happiness (2010)-20251108T221125Z-1-001/Happiness (2010)"

	// Find all MP3 files
	var mp3Files []string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".mp3") {
			mp3Files = append(mp3Files, path)
		}
		return nil
	})

	if len(mp3Files) == 0 {
		t.Skip("Test files not found")
	}

	sort.Strings(mp3Files)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	analyzer := NewAnalyzer(logger)

	// Create library item with multiple files
	audioFiles := make([]AudioFileData, len(mp3Files))
	for i, path := range mp3Files {
		audioFiles[i] = AudioFileData{Path: path}
	}

	item := LibraryItemData{
		Path:       dir,
		AudioFiles: audioFiles,
	}

	ctx := context.Background()
	results, err := analyzer.AnalyzeItems(ctx, []LibraryItemData{item})
	if err != nil {
		t.Fatalf("AnalyzeItems failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("No results returned")
	}

	if len(results[0].AudioFiles) == 0 || results[0].AudioFiles[0].Metadata == nil {
		t.Fatal("No metadata returned")
	}

	meta := results[0].AudioFiles[0].Metadata

	// Print results
	fmt.Printf("\n=== Multi-File MP3 Music Album ===\n")
	fmt.Printf("Album:    %s\n", meta.Album)
	fmt.Printf("Artist:   %s\n", meta.Artist)
	fmt.Printf("Year:     %d\n", meta.Year)
	fmt.Printf("Genre:    %s\n", meta.Genre)
	fmt.Printf("Tracks:   %d\n", len(mp3Files))
	fmt.Printf("Duration: %s\n", meta.Duration)

	if len(meta.Chapters) > 0 {
		fmt.Printf("\nTracks (as chapters):\n")
		for i := 0; i < len(meta.Chapters) && i < 11; i++ {
			ch := meta.Chapters[i]
			fmt.Printf("  [%2d] %s: %s\n", ch.ID, formatDuration(ch.EndTime-ch.StartTime), ch.Title)
		}
	}

	// Verify expectations
	if meta.Album == "" {
		t.Error("Expected album to be set")
	}
	if meta.Artist == "" {
		t.Error("Expected artist to be set")
	}
	if len(meta.Chapters) == 0 {
		t.Error("Expected chapters to be present")
	}
	if meta.Duration == 0 {
		t.Error("Expected non-zero duration")
	}

	fmt.Printf("\nClassification: Multi-file ✓\n")
	fmt.Printf("Works for music: YES ✓\n")
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
