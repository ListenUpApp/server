// Package images provides cover image extraction, processing, and storage.
package images

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Storage manages image filesystem operations.
// Thread-safe for concurrent operations.
// Used for book covers, contributor photos, and other image types.
type Storage struct {
	basePath string
	mu       sync.RWMutex // Protects file operations
}

// NewStorage creates a new Storage instance for book covers.
// basePath should be the metadata directory (e.g., ~/ListenUp/metadata).
// Covers will be stored in {basePath}/covers/
// This is a convenience wrapper around NewStorageWithSubdir.
func NewStorage(basePath string) (*Storage, error) {
	return NewStorageWithSubdir(basePath, "covers")
}

// NewStorageWithSubdir creates a new Storage instance with a custom subdirectory.
// basePath should be the metadata directory (e.g., ~/ListenUp/metadata).
// Images will be stored in {basePath}/{subdir}/.
// Example: NewStorageWithSubdir("/data", "contributors") -> /data/contributors/.
func NewStorageWithSubdir(basePath, subdir string) (*Storage, error) {
	if basePath == "" {
		return nil, fmt.Errorf("base path cannot be empty")
	}
	if subdir == "" {
		return nil, fmt.Errorf("subdirectory cannot be empty")
	}

	storagePath := filepath.Join(basePath, subdir)

	// Create directory if it doesn't exist.
	if err := os.MkdirAll(storagePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create %s directory: %w", subdir, err)
	}

	return &Storage{
		basePath: storagePath,
	}, nil
}

// Save stores image data for an entity.
// Filename format: {id}.jpg.
func (s *Storage) Save(id string, imgData []byte) error {
	if id == "" {
		return fmt.Errorf("ID cannot be empty")
	}

	if len(imgData) == 0 {
		return fmt.Errorf("image data cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.Path(id)

	// Write file with appropriate permissions.
	if err := os.WriteFile(path, imgData, 0644); err != nil {
		return fmt.Errorf("failed to write image file: %w", err)
	}

	return nil
}

// Get retrieves image data for an entity.
func (s *Storage) Get(id string) ([]byte, error) {
	if id == "" {
		return nil, fmt.Errorf("ID cannot be empty")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	path := s.Path(id)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("image not found for %s: %w", id, err)
		}
		return nil, fmt.Errorf("failed to read image file: %w", err)
	}

	return data, nil
}

// Exists checks if an image exists for an entity.
func (s *Storage) Exists(id string) bool {
	if id == "" {
		return false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	path := s.Path(id)
	_, err := os.Stat(path)
	return err == nil
}

// Delete removes an image for an entity.
func (s *Storage) Delete(id string) error {
	if id == "" {
		return fmt.Errorf("ID cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.Path(id)

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			// Already deleted, not an error.
			return nil
		}
		return fmt.Errorf("failed to delete image file: %w", err)
	}

	return nil
}

// Hash computes SHA256 hash of an image.
// Returns hex-encoded string for ETag/cache validation.
func (s *Storage) Hash(id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("ID cannot be empty")
	}

	data, err := s.Get(id)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash), nil
}

// Path returns the full filesystem path for an entity's image.
func (s *Storage) Path(id string) string {
	return filepath.Join(s.basePath, fmt.Sprintf("%s.jpg", id))
}
