package id

import (
	"fmt"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// Generate creates a prefixed unique ID using NanoID
// Format: prefix-nanoid (e.g., "lib-V1StGXR8_Z5jdHi6B-myT")
//
// NanoIDs are URL-friendly, compact (21 characters vs UUID's 36),
// and use a larger alphabet for better entropy per character.
//
// Returns an error if the system has insufficient entropy for secure random generation.
func Generate(prefix string) (string, error) {
	// Use default NanoID (21 characters, URL-safe alphabet)
	id, err := gonanoid.New()
	if err != nil {
		return "", fmt.Errorf("generate nanoid: %w", err)
	}
	return prefix + "-" + id, nil
}

// MustGenerate is like Generate but panics if ID generation fails.
// Use this only when you're certain the system entropy is available,
// or when failure should crash the program (e.g., during initialization).
func MustGenerate(prefix string) string {
	id, err := Generate(prefix)
	if err != nil {
		panic(fmt.Sprintf("failed to generate ID: %v", err))
	}
	return id
}
