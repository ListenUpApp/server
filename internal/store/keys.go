package store

import "sync"

// keyPool provides reusable byte slices for building database keys.
// This reduces allocations on the hot path of database operations.
var keyPool = sync.Pool{
	New: func() any {
		// Pre-allocate 256 bytes which covers most key sizes:
		// - Prefix (10-20 bytes)
		// - "idx:" (4 bytes)
		// - Index name (10-30 bytes)
		// - ":" (1 byte)
		// - Value/ID (21+ bytes for NanoID)
		return make([]byte, 0, 256)
	},
}

// buildKey constructs a database key from prefix and suffix using a pooled buffer.
// The returned slice is valid until releaseKey is called.
// Callers MUST call releaseKey when done with the key.
//
// Usage:
//
//	key := buildKey("book:", bookID)
//	defer releaseKey(key)
//	item, err := txn.Get(key)
func buildKey(prefix, suffix string) []byte {
	buf, _ := keyPool.Get().([]byte)
	buf = buf[:0] // Reset length, keep capacity
	buf = append(buf, prefix...)
	buf = append(buf, suffix...)
	return buf
}

// buildIndexKey constructs an index key from prefix, index name, and value.
// The returned slice is valid until releaseKey is called.
// Callers MUST call releaseKey when done with the key.
//
// Usage:
//
//	key := buildIndexKey("book:", "author", authorID)
//	defer releaseKey(key)
//	item, err := txn.Get(key)
func buildIndexKey(prefix, indexName, value string) []byte {
	buf, _ := keyPool.Get().([]byte)
	buf = buf[:0] // Reset length, keep capacity
	buf = append(buf, prefix...)
	buf = append(buf, "idx:"...)
	buf = append(buf, indexName...)
	buf = append(buf, ':')
	buf = append(buf, value...)
	return buf
}

// releaseKey returns a key buffer to the pool for reuse.
// After calling this, the key slice must not be used.
func releaseKey(key []byte) {
	// Only pool buffers that have reasonable capacity
	// Avoids keeping oversized buffers in the pool
	if cap(key) <= 512 {
		keyPool.Put(key[:0])
	}
}
