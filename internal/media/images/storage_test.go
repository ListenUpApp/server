package images

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStorage(t *testing.T) {
	t.Run("creates storage with valid path", func(t *testing.T) {
		tmpDir := t.TempDir()

		storage, err := NewStorage(tmpDir)
		require.NoError(t, err)
		require.NotNil(t, storage)

		// Verify covers directory was created.
		coversPath := filepath.Join(tmpDir, "covers")
		info, err := os.Stat(coversPath)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("returns error for empty path", func(t *testing.T) {
		storage, err := NewStorage("")
		assert.Error(t, err)
		assert.Nil(t, storage)
		assert.Contains(t, err.Error(), "base path cannot be empty")
	})

	t.Run("creates nested directories if needed", func(t *testing.T) {
		tmpDir := t.TempDir()
		nestedPath := filepath.Join(tmpDir, "nested", "path")

		storage, err := NewStorage(nestedPath)
		require.NoError(t, err)
		require.NotNil(t, storage)

		coversPath := filepath.Join(nestedPath, "covers")
		info, err := os.Stat(coversPath)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})
}

func TestStorage_Save(t *testing.T) {
	t.Run("saves image data successfully", func(t *testing.T) {
		storage := setupTestStorage(t)
		testData := []byte("test image data")

		err := storage.Save("book-123", testData)
		require.NoError(t, err)

		// Verify file was created.
		path := storage.Path("book-123")
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, testData, data)
	})

	t.Run("returns error for empty book ID", func(t *testing.T) {
		storage := setupTestStorage(t)
		testData := []byte("test image data")

		err := storage.Save("", testData)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "book ID cannot be empty")
	})

	t.Run("returns error for empty image data", func(t *testing.T) {
		storage := setupTestStorage(t)

		err := storage.Save("book-123", []byte{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "image data cannot be empty")
	})

	t.Run("overwrites existing file", func(t *testing.T) {
		storage := setupTestStorage(t)
		bookID := "book-123"

		// Save initial data.
		err := storage.Save(bookID, []byte("initial data"))
		require.NoError(t, err)

		// Overwrite with new data.
		newData := []byte("updated data")
		err = storage.Save(bookID, newData)
		require.NoError(t, err)

		// Verify new data was saved.
		data, err := storage.Get(bookID)
		require.NoError(t, err)
		assert.Equal(t, newData, data)
	})
}

func TestStorage_Get(t *testing.T) {
	t.Run("retrieves saved image data", func(t *testing.T) {
		storage := setupTestStorage(t)
		testData := []byte("test image data")
		bookID := "book-123"

		err := storage.Save(bookID, testData)
		require.NoError(t, err)

		data, err := storage.Get(bookID)
		require.NoError(t, err)
		assert.Equal(t, testData, data)
	})

	t.Run("returns error for non-existent cover", func(t *testing.T) {
		storage := setupTestStorage(t)

		data, err := storage.Get("non-existent-book")
		assert.Error(t, err)
		assert.Nil(t, data)
		assert.Contains(t, err.Error(), "cover not found")
	})

	t.Run("returns error for empty book ID", func(t *testing.T) {
		storage := setupTestStorage(t)

		data, err := storage.Get("")
		assert.Error(t, err)
		assert.Nil(t, data)
		assert.Contains(t, err.Error(), "book ID cannot be empty")
	})
}

func TestStorage_Exists(t *testing.T) {
	t.Run("returns true for existing cover", func(t *testing.T) {
		storage := setupTestStorage(t)
		bookID := "book-123"

		err := storage.Save(bookID, []byte("test data"))
		require.NoError(t, err)

		assert.True(t, storage.Exists(bookID))
	})

	t.Run("returns false for non-existent cover", func(t *testing.T) {
		storage := setupTestStorage(t)

		assert.False(t, storage.Exists("non-existent-book"))
	})

	t.Run("returns false for empty book ID", func(t *testing.T) {
		storage := setupTestStorage(t)

		assert.False(t, storage.Exists(""))
	})
}

func TestStorage_Delete(t *testing.T) {
	t.Run("deletes existing cover", func(t *testing.T) {
		storage := setupTestStorage(t)
		bookID := "book-123"

		err := storage.Save(bookID, []byte("test data"))
		require.NoError(t, err)
		require.True(t, storage.Exists(bookID))

		err = storage.Delete(bookID)
		require.NoError(t, err)
		assert.False(t, storage.Exists(bookID))
	})

	t.Run("succeeds when cover does not exist", func(t *testing.T) {
		storage := setupTestStorage(t)

		err := storage.Delete("non-existent-book")
		assert.NoError(t, err) // Not an error to delete non-existent file.
	})

	t.Run("returns error for empty book ID", func(t *testing.T) {
		storage := setupTestStorage(t)

		err := storage.Delete("")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "book ID cannot be empty")
	})
}

func TestStorage_Hash(t *testing.T) {
	t.Run("computes consistent hash", func(t *testing.T) {
		storage := setupTestStorage(t)
		bookID := "book-123"
		testData := []byte("test image data")

		err := storage.Save(bookID, testData)
		require.NoError(t, err)

		hash1, err := storage.Hash(bookID)
		require.NoError(t, err)
		assert.NotEmpty(t, hash1)

		// Hash should be consistent.
		hash2, err := storage.Hash(bookID)
		require.NoError(t, err)
		assert.Equal(t, hash1, hash2)

		// Hash should be 64 characters (SHA256 hex).
		assert.Len(t, hash1, 64)
	})

	t.Run("different data produces different hash", func(t *testing.T) {
		storage := setupTestStorage(t)

		err := storage.Save("book-1", []byte("data1"))
		require.NoError(t, err)

		err = storage.Save("book-2", []byte("data2"))
		require.NoError(t, err)

		hash1, err := storage.Hash("book-1")
		require.NoError(t, err)

		hash2, err := storage.Hash("book-2")
		require.NoError(t, err)

		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("returns error for non-existent cover", func(t *testing.T) {
		storage := setupTestStorage(t)

		hash, err := storage.Hash("non-existent-book")
		assert.Error(t, err)
		assert.Empty(t, hash)
	})

	t.Run("returns error for empty book ID", func(t *testing.T) {
		storage := setupTestStorage(t)

		hash, err := storage.Hash("")
		assert.Error(t, err)
		assert.Empty(t, hash)
	})
}

func TestStorage_Path(t *testing.T) {
	t.Run("generates correct path", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, err := NewStorage(tmpDir)
		require.NoError(t, err)

		path := storage.Path("book-123")
		expected := filepath.Join(tmpDir, "covers", "book-123.jpg")
		assert.Equal(t, expected, path)
	})

	t.Run("handles various book IDs", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, err := NewStorage(tmpDir)
		require.NoError(t, err)

		testCases := []struct {
			bookID   string
			expected string
		}{
			{"book-123", "book-123.jpg"},
			{"book_abc_def", "book_abc_def.jpg"},
			{"123", "123.jpg"},
		}

		for _, tc := range testCases {
			path := storage.Path(tc.bookID)
			assert.Contains(t, path, tc.expected)
		}
	})
}

func TestStorage_Concurrent(t *testing.T) {
	t.Run("handles concurrent writes safely", func(t *testing.T) {
		storage := setupTestStorage(t)
		bookID := "book-123"

		// Run multiple concurrent writes.
		const goroutines = 10
		done := make(chan bool, goroutines)

		for i := 0; i < goroutines; i++ {
			go func(n int) {
				data := []byte{byte(n)}
				err := storage.Save(bookID, data)
				assert.NoError(t, err)
				done <- true
			}(i)
		}

		// Wait for all goroutines.
		for i := 0; i < goroutines; i++ {
			<-done
		}

		// Verify file exists and can be read.
		assert.True(t, storage.Exists(bookID))
		data, err := storage.Get(bookID)
		assert.NoError(t, err)
		assert.NotEmpty(t, data)
	})

	t.Run("handles concurrent reads safely", func(t *testing.T) {
		storage := setupTestStorage(t)
		bookID := "book-123"
		testData := []byte("test data")

		err := storage.Save(bookID, testData)
		require.NoError(t, err)

		// Run multiple concurrent reads.
		const goroutines = 10
		done := make(chan bool, goroutines)

		for i := 0; i < goroutines; i++ {
			go func() {
				data, err := storage.Get(bookID)
				assert.NoError(t, err)
				assert.Equal(t, testData, data)
				done <- true
			}()
		}

		// Wait for all goroutines.
		for i := 0; i < goroutines; i++ {
			<-done
		}
	})
}

// setupTestStorage creates a Storage instance with a temporary directory.
func setupTestStorage(t *testing.T) *Storage {
	t.Helper()
	tmpDir := t.TempDir()
	storage, err := NewStorage(tmpDir)
	require.NoError(t, err)
	return storage
}
