// Package images provides cover image extraction, processing, and storage.
package images

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Storage manages cover image filesystem operations.
// Thread-safe for concurrent scanner operations.
type Storage struct {
	basePath string
	mu       sync.RWMutex // Protects file operations
}

// NewStorage creates a new Storage instance.
// basePath should be the metadata directory (e.g., ~/ListenUp/metadata).
// Covers will be stored in {basePath}/covers/
func NewStorage(basePath string) (*Storage, error) {
	if basePath == "" {
		return nil, fmt.Errorf("base path cannot be empty")
	}

	coversPath := filepath.Join(basePath, "covers")

	// Create covers directory if it doesn't exist.
	if err := os.MkdirAll(coversPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create covers directory: %w", err)
	}

	return &Storage{
		basePath: coversPath,
	}, nil
}

// Save stores cover image data for a book.
// Filename format: {bookID}.jpg
func (s *Storage) Save(bookID string, imgData []byte) error {
	if bookID == "" {
		return fmt.Errorf("book ID cannot be empty")
	}

	if len(imgData) == 0 {
		return fmt.Errorf("image data cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.Path(bookID)

	// Write file with appropriate permissions.
	if err := os.WriteFile(path, imgData, 0644); err != nil {
		return fmt.Errorf("failed to write cover file: %w", err)
	}

	return nil
}

// Get retrieves cover image data for a book.
func (s *Storage) Get(bookID string) ([]byte, error) {
	if bookID == "" {
		return nil, fmt.Errorf("book ID cannot be empty")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	path := s.Path(bookID)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("cover not found for book %s: %w", bookID, err)
		}
		return nil, fmt.Errorf("failed to read cover file: %w", err)
	}

	return data, nil
}

// Exists checks if a cover image exists for a book.
func (s *Storage) Exists(bookID string) bool {
	if bookID == "" {
		return false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	path := s.Path(bookID)
	_, err := os.Stat(path)
	return err == nil
}

// Delete removes a cover image for a book.
func (s *Storage) Delete(bookID string) error {
	if bookID == "" {
		return fmt.Errorf("book ID cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.Path(bookID)

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			// Already deleted, not an error.
			return nil
		}
		return fmt.Errorf("failed to delete cover file: %w", err)
	}

	return nil
}

// Hash computes SHA256 hash of a cover image.
// Returns hex-encoded string for ETag/cache validation.
func (s *Storage) Hash(bookID string) (string, error) {
	if bookID == "" {
		return "", fmt.Errorf("book ID cannot be empty")
	}

	data, err := s.Get(bookID)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash), nil
}

// Path returns the full filesystem path for a book's cover.
func (s *Storage) Path(bookID string) string {
	return filepath.Join(s.basePath, fmt.Sprintf("%s.jpg", bookID))
}
