package audio

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// getTestFile returns the path to the test audio file
func getTestFile(t *testing.T) string {
	// Try different possible locations
	candidates := []string{
		filepath.Join("..", "testdata", "test.mp3"),
		filepath.Join("internal", "scanner", "testdata", "test.mp3"),
		filepath.Join("testdata", "test.mp3"),
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	t.Skip("test file not found, skipping test")
	return ""
}

func TestFFprobeParser_Parse_BasicMetadata(t *testing.T) {
	testFile := getTestFile(t)

	parser := NewFFprobeParser()
	ctx := context.Background()

	metadata, err := parser.Parse(ctx, testFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Verify basic metadata
	if metadata == nil {
		t.Fatal("expected metadata, got nil")
	}

	// Check title
	if metadata.Title != "Test Book" {
		t.Errorf("expected title 'Test Book', got '%s'", metadata.Title)
	}

	// Check artist
	if metadata.Artist != "Test Author" {
		t.Errorf("expected artist 'Test Author', got '%s'", metadata.Artist)
	}

	// Check album
	if metadata.Album != "Test Album" {
		t.Errorf("expected album 'Test Album', got '%s'", metadata.Album)
	}

	// Check genre
	if metadata.Genre != "Fiction" {
		t.Errorf("expected genre 'Fiction', got '%s'", metadata.Genre)
	}

	// Check year
	if metadata.Year != 2023 {
		t.Errorf("expected year 2023, got %d", metadata.Year)
	}

	// Check narrator (from composer tag)
	if metadata.Composer != "Test Narrator" {
		t.Errorf("expected composer 'Test Narrator', got '%s'", metadata.Composer)
	}

	// Check description (from comment tag)
	if metadata.Description == "" {
		t.Error("expected description to be set")
	}
}

func TestFFprobeParser_Parse_AudioFormat(t *testing.T) {
	testFile := getTestFile(t)

	parser := NewFFprobeParser()
	ctx := context.Background()

	metadata, err := parser.Parse(ctx, testFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Check format
	if metadata.Format != "mp3" {
		t.Errorf("expected format 'mp3', got '%s'", metadata.Format)
	}

	// Check codec
	if metadata.Codec == "" {
		t.Error("expected codec to be set")
	}

	// Check sample rate
	if metadata.SampleRate != 44100 {
		t.Errorf("expected sample rate 44100, got %d", metadata.SampleRate)
	}

	// Check channels
	if metadata.Channels != 1 {
		t.Errorf("expected 1 channel (mono), got %d", metadata.Channels)
	}

	// Check duration (should be ~1 second)
	if metadata.Duration < 900*time.Millisecond || metadata.Duration > 1100*time.Millisecond {
		t.Errorf("expected duration ~1s, got %v", metadata.Duration)
	}

	// Check bitrate is set
	if metadata.Bitrate == 0 {
		t.Error("expected bitrate to be set")
	}
}

func TestFFprobeParser_Parse_NonexistentFile(t *testing.T) {
	parser := NewFFprobeParser()
	ctx := context.Background()

	_, err := parser.Parse(ctx, "/nonexistent/file.mp3")
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}

func TestFFprobeParser_Parse_ContextCancellation(t *testing.T) {
	testFile := getTestFile(t)

	parser := NewFFprobeParser()

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := parser.Parse(ctx, testFile)
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

func TestFFprobeParser_Parse_InvalidFile(t *testing.T) {
	// Create a non-audio file
	tmpDir := t.TempDir()
	invalidFile := filepath.Join(tmpDir, "invalid.mp3")
	if err := os.WriteFile(invalidFile, []byte("not an audio file"), 0644); err != nil {
		t.Fatal(err)
	}

	parser := NewFFprobeParser()
	ctx := context.Background()

	_, err := parser.Parse(ctx, invalidFile)
	if err == nil {
		t.Error("expected error for invalid audio file, got nil")
	}
}
