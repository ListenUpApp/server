package images

import (
	"bytes"
	"context"
	"image"
	_ "image/jpeg" // Register JPEG decoder
	_ "image/png"  // Register PNG decoder
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessor_ExtractAndProcess(t *testing.T) {
	t.Run("extracts and saves cover from M4B file", func(t *testing.T) {
		// This test requires a real M4B file with embedded artwork.
		// We'll skip if the test file doesn't exist.
		testFile := findTestAudioFile(t)
		if testFile == "" {
			t.Skip("No test audio file with embedded cover available")
		}

		ctx := context.Background()
		processor := setupTestProcessor(t)
		bookID := "book-test-001"

		hash, err := processor.ExtractAndProcess(ctx, testFile, bookID)
		require.NoError(t, err)
		assert.NotEmpty(t, hash, "hash should not be empty")
		assert.Len(t, hash, 64, "hash should be 64 characters (SHA256)")

		// Verify file was created.
		assert.True(t, processor.storage.Exists(bookID))

		// Verify file size is reasonable (original artwork, typically 100KB-5MB).
		data, err := processor.storage.Get(bookID)
		require.NoError(t, err)
		assert.Greater(t, len(data), 10*1024, "cover should be at least 10KB")
		assert.Less(t, len(data), 10*1024*1024, "cover should be less than 10MB")

		// Verify it's a valid image (JPEG or PNG).
		_, format, err := image.Decode(bytes.NewReader(data))
		assert.NoError(t, err, "cover should be valid image")
		assert.Contains(t, []string{"jpeg", "png"}, format, "cover should be JPEG or PNG")
	})

	t.Run("returns empty hash for file without cover", func(t *testing.T) {
		// Create a minimal M4B file without cover art.
		testFile := createTestAudioFileWithoutCover(t)

		ctx := context.Background()
		processor := setupTestProcessor(t)
		bookID := "book-no-cover"

		hash, err := processor.ExtractAndProcess(ctx, testFile, bookID)
		require.NoError(t, err)
		assert.Empty(t, hash, "hash should be empty for file without cover")

		// Verify no file was created.
		assert.False(t, processor.storage.Exists(bookID))
	})

	t.Run("returns error for non-existent file", func(t *testing.T) {
		ctx := context.Background()
		processor := setupTestProcessor(t)

		hash, err := processor.ExtractAndProcess(ctx, "/non/existent/file.m4b", "book-123")
		assert.Error(t, err)
		assert.Empty(t, hash)
		assert.Contains(t, err.Error(), "failed to open audio file")
	})

	t.Run("returns error for invalid file", func(t *testing.T) {
		// Create an invalid audio file.
		tmpDir := t.TempDir()
		invalidFile := filepath.Join(tmpDir, "invalid.m4b")
		err := os.WriteFile(invalidFile, []byte("not an audio file"), 0644)
		require.NoError(t, err)

		ctx := context.Background()
		processor := setupTestProcessor(t)

		hash, err := processor.ExtractAndProcess(ctx, invalidFile, "book-invalid")
		assert.Error(t, err)
		assert.Empty(t, hash)
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		testFile := findTestAudioFile(t)
		if testFile == "" {
			t.Skip("No test audio file available")
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately.

		processor := setupTestProcessor(t)

		hash, err := processor.ExtractAndProcess(ctx, testFile, "book-cancelled")
		assert.Error(t, err)
		assert.Empty(t, hash)
		assert.Contains(t, err.Error(), "context canceled")
	})

	t.Run("handles timeout context", func(t *testing.T) {
		testFile := findTestAudioFile(t)
		if testFile == "" {
			t.Skip("No test audio file available")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		time.Sleep(2 * time.Millisecond) // Ensure timeout fires.

		processor := setupTestProcessor(t)

		hash, err := processor.ExtractAndProcess(ctx, testFile, "book-timeout")
		assert.Error(t, err)
		assert.Empty(t, hash)
	})
}

func TestProcessor_HashConsistency(t *testing.T) {
	t.Run("same input produces same hash", func(t *testing.T) {
		testFile := findTestAudioFile(t)
		if testFile == "" {
			t.Skip("No test audio file available")
		}

		ctx := context.Background()
		processor := setupTestProcessor(t)
		bookID := "book-hash-test"

		// Extract twice.
		hash1, err := processor.ExtractAndProcess(ctx, testFile, bookID)
		require.NoError(t, err)

		hash2, err := processor.ExtractAndProcess(ctx, testFile, bookID)
		require.NoError(t, err)

		// Hashes should be identical.
		assert.Equal(t, hash1, hash2)
	})
}

func TestProcessor_ErrorHandling(t *testing.T) {
	t.Run("handles corrupted image data gracefully", func(t *testing.T) {
		// This would require creating an audio file with corrupted image data.
		// For now, we'll test with an invalid file.
		tmpDir := t.TempDir()
		invalidFile := filepath.Join(tmpDir, "corrupted.m4b")
		err := os.WriteFile(invalidFile, []byte("corrupted"), 0644)
		require.NoError(t, err)

		ctx := context.Background()
		processor := setupTestProcessor(t)

		hash, err := processor.ExtractAndProcess(ctx, invalidFile, "book-corrupted")
		assert.Error(t, err)
		assert.Empty(t, hash)
	})

	t.Run("handles permission errors on storage", func(t *testing.T) {
		// Create storage in a read-only directory.
		tmpDir := t.TempDir()
		readOnlyDir := filepath.Join(tmpDir, "readonly")
		err := os.Mkdir(readOnlyDir, 0555) // Read-only.
		require.NoError(t, err)
		t.Cleanup(func() {
			os.Chmod(readOnlyDir, 0755) // Restore permissions for cleanup.
		})

		storage, err := NewStorage(readOnlyDir)
		if err == nil {
			// If storage creation succeeded (mkdir worked), that's fine.
			// The save operation should fail.
			log := logger.New(logger.Config{Level: slog.LevelDebug})
			processor := NewProcessor(storage, log.Logger)

			// Try to save - should fail due to permissions.
			testFile := findTestAudioFile(t)
			if testFile == "" {
				t.Skip("No test audio file available")
			}

			ctx := context.Background()
			_, err := processor.ExtractAndProcess(ctx, testFile, "book-permission")
			assert.Error(t, err)
		}
	})
}

// Helper functions.

// setupTestProcessor creates a Processor with a temporary storage.
func setupTestProcessor(t *testing.T) *Processor {
	t.Helper()
	tmpDir := t.TempDir()
	storage, err := NewStorage(tmpDir)
	require.NoError(t, err)

	log := logger.New(logger.Config{Level: slog.LevelDebug})
	return NewProcessor(storage, log.Logger)
}

// findTestAudioFile looks for a test audio file in common locations.
// Returns empty string if no test file is found.
func findTestAudioFile(t *testing.T) string {
	t.Helper()

	// Check common test locations.
	possiblePaths := []string{
		"testdata/sample.m4b",
		"testdata/sample.mp3",
		"../../../testdata/sample.m4b",
		"../../../testdata/sample.mp3",
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Check if user has audiobook path configured (from environment).
	if audiobookPath := os.Getenv("TEST_AUDIOBOOK_PATH"); audiobookPath != "" {
		if _, err := os.Stat(audiobookPath); err == nil {
			return audiobookPath
		}
	}

	return ""
}

// createTestAudioFileWithoutCover creates a minimal audio file without cover art.
// This is a placeholder - actual implementation would require generating a valid audio file.
func createTestAudioFileWithoutCover(t *testing.T) string {
	t.Helper()
	t.Skip("Creating minimal audio files not yet implemented - requires audio encoding")
	return ""
}
