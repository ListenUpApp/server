package id

import (
	gonanoid "github.com/matoous/go-nanoid/v2"
)

// Generate creates a prefixed unique ID using NanoID
// Format: prefix-nanoid (e.g., "lib-V1StGXR8_Z5jdHi6B-myT")
//
// NanoIDs are URL-friendly, compact (21 characters vs UUID's 36),
// and use a larger alphabet for better entropy per character.
func Generate(prefix string) string {
	// Use default NanoID (21 characters, URL-safe alphabet)
	id, err := gonanoid.New()
	if err != nil {
		// Fallback to a simpler generation if crypto fails
		// This should never happen in practice
		id = gonanoid.Must()
	}
	return prefix + "-" + id
}

// MustGenerate is like Generate but panics if ID generation fails.
// Use this only when you're certain the system entropy is available.
func MustGenerate(prefix string) string {
	return prefix + "-" + gonanoid.Must()
}
