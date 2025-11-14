package processor

import "sync"

// SyncMap is a type-safe concurrent map using generics.
// It provides a simpler, type-safe alternative to sync.Map for cases.
// where the key and value types are known at compile time.
//
// This implementation uses a RWMutex for safe concurrent access,.
// which is more efficient than sync.Map for workloads with frequent reads.
// and infrequent writes.
type SyncMap[K comparable, V any] struct {
	m  map[K]V
	mu sync.RWMutex
}

// NewSyncMap creates a new type-safe concurrent map.
func NewSyncMap[K comparable, V any]() *SyncMap[K, V] {
	return &SyncMap[K, V]{
		m: make(map[K]V),
	}
}

// Load returns the value stored in the map for a key, or the zero value.
// if no value is present. The ok result indicates whether value was found.
func (sm *SyncMap[K, V]) Load(key K) (value V, ok bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	value, ok = sm.m[key]
	return
}

// Store sets the value for a key.
func (sm *SyncMap[K, V]) Store(key K, value V) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.m[key] = value
}

// LoadOrStore returns the existing value for the key if present.
// Otherwise, it stores and returns the given value.
// The loaded result is true if the value was loaded, false if stored.
func (sm *SyncMap[K, V]) LoadOrStore(key K, value V) (actual V, loaded bool) {
	// First try with read lock.
	sm.mu.RLock()
	actual, loaded = sm.m[key]
	sm.mu.RUnlock()
	if loaded {
		return actual, true
	}

	// Need to store - acquire write lock.
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check again in case another goroutine stored a value.
	// between releasing RLock and acquiring Lock.
	actual, loaded = sm.m[key]
	if loaded {
		return actual, true
	}

	// Store the new value.
	sm.m[key] = value
	return value, false
}

// Delete deletes the value for a key.
func (sm *SyncMap[K, V]) Delete(key K) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.m, key)
}

// Len returns the number of items in the map.
func (sm *SyncMap[K, V]) Len() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.m)
}
