package mp3

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/pkg/audiometa"
)

func TestExtractChapterIndex(t *testing.T) {
	tests := []struct {
		filename string
		expected int
	}{
		{"01 - The Boy Who Lived.mp3", 1},
		{"02 - Chapter 2.mp3", 2},
		{"Chapter 03 - Title.mp3", 3},
		{"Track04.mp3", 4},
		{"Part 5.mp3", 5},
		{"10.mp3", 10},
		{"random.mp3", 0}, // No number
	}

	for _, tt := range tests {
		result := extractChapterIndex(tt.filename)
		if result != tt.expected {
			t.Errorf("extractChapterIndex(%q) = %d, expected %d",
				tt.filename, result, tt.expected)
		}
	}
}

func TestExtractChapterTitle(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{"01 - The Boy Who Lived.mp3", "The Boy Who Lived"},
		{"Chapter 02 - The Vanishing Glass.mp3", "The Vanishing Glass"},
		{"03-Title.mp3", "Title"},
		{"Track04.mp3", "Chapter 04"}, // Returns padded number
		{"05.mp3", "Chapter 05"},       // Returns padded number
		{"NoNumber.mp3", "NoNumber"},
	}

	for _, tt := range tests {
		result := extractChapterTitle(tt.filename)
		if result != tt.expected {
			t.Errorf("extractChapterTitle(%q) = %q, expected %q",
				tt.filename, result, tt.expected)
		}
	}
}

func TestNaturalLess(t *testing.T) {
	tests := []struct {
		a        string
		b        string
		expected bool
	}{
		{"1.mp3", "2.mp3", true},
		{"2.mp3", "10.mp3", true},
		{"10.mp3", "2.mp3", false},
		{"a.mp3", "b.mp3", true},
		{"file1.mp3", "file2.mp3", true},
		{"file2.mp3", "file10.mp3", true},
	}

	for _, tt := range tests {
		result := naturalLess(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("naturalLess(%q, %q) = %v, expected %v",
				tt.a, tt.b, result, tt.expected)
		}
	}
}

func TestParseMultiFile(t *testing.T) {
	// Create temporary directory with test MP3 files
	tmpDir, err := os.MkdirTemp("", "mp3test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test files
	testFiles := []string{
		"01 - Chapter One.mp3",
		"02 - Chapter Two.mp3",
		"03 - Chapter Three.mp3",
	}

	paths := make([]string, len(testFiles))
	for i, name := range testFiles {
		path := filepath.Join(tmpDir, name)
		data := createMinimalMP3WithID3()
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
		paths[i] = path
	}

	// Parse multi-file
	meta, err := ParseMultiFile(paths)
	if err != nil {
		t.Fatalf("ParseMultiFile failed: %v", err)
	}

	// Check results
	if meta.Format != audiometa.FormatMP3 {
		t.Errorf("expected FormatMP3, got %v", meta.Format)
	}

	if len(meta.Chapters) != 3 {
		t.Errorf("expected 3 chapters, got %d", len(meta.Chapters))
	}

	// Check chapter titles derived from filenames
	expectedTitles := []string{"Chapter One", "Chapter Two", "Chapter Three"}
	for i, ch := range meta.Chapters {
		if ch.Index != i+1 {
			t.Errorf("chapter %d: expected index %d, got %d", i, i+1, ch.Index)
		}
		if ch.Title != expectedTitles[i] {
			t.Errorf("chapter %d: expected title %q, got %q", i, expectedTitles[i], ch.Title)
		}
	}

	// Check duration is aggregated
	if meta.Duration == 0 {
		t.Error("expected non-zero aggregated duration")
	}

	// Check file size is aggregated
	if meta.FileSize == 0 {
		t.Error("expected non-zero aggregated file size")
	}

	// Check track total
	if meta.TrackTotal != 3 {
		t.Errorf("expected TrackTotal = 3, got %d", meta.TrackTotal)
	}
}

func TestParseMultiFile_Empty(t *testing.T) {
	_, err := ParseMultiFile([]string{})
	if err == nil {
		t.Error("expected error for empty file list")
	}
}

func TestParseMultiFile_SingleFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test*.mp3")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	data := createMinimalMP3WithID3()
	tmpFile.Write(data)
	tmpFile.Close()

	meta, err := ParseMultiFile([]string{tmpFile.Name()})
	if err != nil {
		t.Fatalf("ParseMultiFile failed: %v", err)
	}

	if len(meta.Chapters) != 1 {
		t.Errorf("expected 1 chapter, got %d", len(meta.Chapters))
	}
}

func TestSortFiles(t *testing.T) {
	files := []fileMetadata{
		{Path: "file2.mp3", Index: 0},
		{Path: "file10.mp3", Index: 0},
		{Path: "file1.mp3", Index: 0},
	}

	sortFiles(files)

	// Check natural sort order
	expected := []string{"file1.mp3", "file2.mp3", "file10.mp3"}
	for i, f := range files {
		base := filepath.Base(f.Path)
		if base != expected[i] {
			t.Errorf("index %d: expected %q, got %q", i, expected[i], base)
		}
		// Check indices are renumbered
		if f.Index != i+1 {
			t.Errorf("index %d: expected index %d, got %d", i, i+1, f.Index)
		}
	}
}

func TestSortFiles_WithTrackNumbers(t *testing.T) {
	files := []fileMetadata{
		{Path: "file2.mp3", Index: 2, Metadata: &audiometa.Metadata{TrackNumber: 2}},
		{Path: "file1.mp3", Index: 1, Metadata: &audiometa.Metadata{TrackNumber: 1}},
		{Path: "file3.mp3", Index: 3, Metadata: &audiometa.Metadata{TrackNumber: 3}},
	}

	sortFiles(files)

	// Should sort by track number
	for i, f := range files {
		if f.Index != i+1 {
			t.Errorf("index %d: expected index %d, got %d", i, i+1, f.Index)
		}
	}
}

func TestCreateChaptersFromFiles(t *testing.T) {
	files := []fileMetadata{
		{
			Path:  "01 - Chapter One.mp3",
			Index: 1,
			Metadata: &audiometa.Metadata{
				Duration: 10 * time.Minute,
			},
		},
		{
			Path:  "02 - Chapter Two.mp3",
			Index: 2,
			Metadata: &audiometa.Metadata{
				Duration: 15 * time.Minute,
			},
		},
	}

	chapters := createChaptersFromFiles(files)

	if len(chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(chapters))
	}

	// Chapter 1
	if chapters[0].Index != 1 {
		t.Errorf("chapter 0: expected index 1, got %d", chapters[0].Index)
	}
	if chapters[0].StartTime != 0 {
		t.Errorf("chapter 0: expected start time 0, got %v", chapters[0].StartTime)
	}
	if chapters[0].EndTime != 10*time.Minute {
		t.Errorf("chapter 0: expected end time 10m, got %v", chapters[0].EndTime)
	}
	if chapters[0].Title != "Chapter One" {
		t.Errorf("chapter 0: expected title 'Chapter One', got %q", chapters[0].Title)
	}

	// Chapter 2
	if chapters[1].Index != 2 {
		t.Errorf("chapter 1: expected index 2, got %d", chapters[1].Index)
	}
	if chapters[1].StartTime != 10*time.Minute {
		t.Errorf("chapter 1: expected start time 10m, got %v", chapters[1].StartTime)
	}
	if chapters[1].EndTime != 25*time.Minute {
		t.Errorf("chapter 1: expected end time 25m, got %v", chapters[1].EndTime)
	}
}

func TestAggregateMetadata(t *testing.T) {
	files := []fileMetadata{
		{
			Path:  "file1.mp3",
			Index: 1,
			Metadata: &audiometa.Metadata{
				Title:      "Book Title",
				Artist:     "Author Name",
				Duration:   10 * time.Minute,
				FileSize:   1000,
				BitRate:    128000,
				SampleRate: 44100,
			},
		},
		{
			Path:  "file2.mp3",
			Index: 2,
			Metadata: &audiometa.Metadata{
				Title:      "Different Title", // Inconsistent
				Artist:     "Author Name",
				Duration:   15 * time.Minute,
				FileSize:   1500,
				BitRate:    128000,
				SampleRate: 44100,
			},
		},
	}

	meta := aggregateMetadata(files)

	// Should use first file's title
	if meta.Title != "Book Title" {
		t.Errorf("expected title 'Book Title', got %q", meta.Title)
	}

	// Should aggregate duration
	if meta.Duration != 25*time.Minute {
		t.Errorf("expected duration 25m, got %v", meta.Duration)
	}

	// Should aggregate file size
	if meta.FileSize != 2500 {
		t.Errorf("expected file size 2500, got %d", meta.FileSize)
	}

	// Should set track total
	if meta.TrackTotal != 2 {
		t.Errorf("expected track total 2, got %d", meta.TrackTotal)
	}

	// Should have warning about inconsistent title
	foundWarning := false
	for _, w := range meta.Warnings {
		if contains(w, "inconsistent title") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Error("expected warning about inconsistent title")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr || len(s) > len(substr) && containsHelper(s, substr)
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}