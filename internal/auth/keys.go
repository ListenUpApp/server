// Package auth provides authentication and authorization functionality.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// PASETO v4 requires a 256-bit (32-byte) symmetric key.
	keyLength = 32
	// Expected hex-encoded length (32 bytes = 64 hex characters).
	keyHexLength = 64
)

// LoadOrGenerateKey loads or generates the PASETO v4 symmetric key for access tokens.
// The key is stored in <metadataPath>/auth.key as a hex-encoded string.
// If the file doesn't exist, a new key is generated and saved.
// Returns the decoded 32-byte key ready for use.
func LoadOrGenerateKey(metadataPath string) ([]byte, error) {
	keyPath := filepath.Join(metadataPath, "auth.key")

	// Try to load existing key.
	//#nosec G304 -- Auth key path is derived from validated metadata path
	if keyBytes, err := os.ReadFile(keyPath); err == nil {
		keyHex := strings.TrimSpace(string(keyBytes))

		// Validate hex format (should be 64 hex chars = 32 bytes).
		if len(keyHex) != keyHexLength {
			return nil, fmt.Errorf("invalid auth key length: expected %d hex chars, got %d", keyHexLength, len(keyHex))
		}

		// Decode hex to bytes.
		key, err := hex.DecodeString(keyHex)
		if err != nil {
			return nil, fmt.Errorf("invalid auth key format: not valid hex: %w", err)
		}

		return key, nil
	}

	// Generate new key (32 bytes = 256 bits for PASETO v4).
	key := make([]byte, keyLength)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate auth key: %w", err)
	}

	// Encode as hex for storage.
	keyHex := hex.EncodeToString(key)

	// Ensure metadata directory exists.
	if err := os.MkdirAll(metadataPath, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create metadata directory: %w", err)
	}

	// Save key to file with restricted permissions.
	if err := os.WriteFile(keyPath, []byte(keyHex), 0o600); err != nil {
		return nil, fmt.Errorf("failed to save auth key: %w", err)
	}

	return key, nil
}
