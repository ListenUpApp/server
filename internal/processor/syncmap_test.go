package processor

import (
	"sync"
	"testing"
)

// TestSyncMap_BasicOperations tests basic Load, Store operations.
func TestSyncMap_BasicOperations(t *testing.T) {
	sm := NewSyncMap[string, int]()

	// Test Store and Load.
	sm.Store("one", 1)
	sm.Store("two", 2)

	if val, ok := sm.Load("one"); !ok || val != 1 {
		t.Errorf("Load(one) = %v, %v; want 1, true", val, ok)
	}

	if val, ok := sm.Load("two"); !ok || val != 2 {
		t.Errorf("Load(two) = %v, %v; want 2, true", val, ok)
	}

	// Test Load for non-existent key.
	if val, ok := sm.Load("three"); ok {
		t.Errorf("Load(three) = %v, %v; want 0, false", val, ok)
	}
}

// TestSyncMap_LoadOrStore tests LoadOrStore functionality.
func TestSyncMap_LoadOrStore(t *testing.T) {
	sm := NewSyncMap[string, int]()

	// First store - should return the stored value with loaded=false.
	actual, loaded := sm.LoadOrStore("key1", 100)
	if actual != 100 || loaded {
		t.Errorf("LoadOrStore(key1, 100) = %v, %v; want 100, false", actual, loaded)
	}

	// Second store with same key - should return existing value with loaded=true.
	actual, loaded = sm.LoadOrStore("key1", 200)
	if actual != 100 || !loaded {
		t.Errorf("LoadOrStore(key1, 200) = %v, %v; want 100, true", actual, loaded)
	}

	// Verify the value wasn't overwritten.
	if val, _ := sm.Load("key1"); val != 100 {
		t.Errorf("Load(key1) = %v; want 100", val)
	}
}

// TestSyncMap_Delete tests Delete functionality.
func TestSyncMap_Delete(t *testing.T) {
	sm := NewSyncMap[string, int]()

	sm.Store("key1", 1)
	sm.Store("key2", 2)

	if sm.Len() != 2 {
		t.Errorf("Len() = %v; want 2", sm.Len())
	}

	sm.Delete("key1")

	if _, ok := sm.Load("key1"); ok {
		t.Error("Load(key1) should return false after Delete")
	}

	if sm.Len() != 1 {
		t.Errorf("Len() = %v; want 1", sm.Len())
	}

	// Delete non-existent key should not panic.
	sm.Delete("nonexistent")
}

// TestSyncMap_Len tests Len functionality.
func TestSyncMap_Len(t *testing.T) {
	sm := NewSyncMap[string, int]()

	if sm.Len() != 0 {
		t.Errorf("Len() = %v; want 0", sm.Len())
	}

	sm.Store("a", 1)
	sm.Store("b", 2)
	sm.Store("c", 3)

	if sm.Len() != 3 {
		t.Errorf("Len() = %v; want 3", sm.Len())
	}

	sm.Delete("b")

	if sm.Len() != 2 {
		t.Errorf("Len() = %v; want 2", sm.Len())
	}
}

// TestSyncMap_ConcurrentAccess tests concurrent access safety.
func TestSyncMap_ConcurrentAccess(t *testing.T) {
	sm := NewSyncMap[int, int]()
	numGoroutines := 100
	numOperations := 1000

	var wg sync.WaitGroup

	// Concurrent writes.
	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range numOperations {
				key := id*numOperations + j
				sm.Store(key, key*2)
			}
		}(i)
	}

	// Concurrent reads.
	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range numOperations {
				key := id*numOperations + j
				sm.Load(key)
			}
		}(i)
	}

	// Concurrent LoadOrStore.
	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range numOperations {
				key := id*numOperations + j
				sm.LoadOrStore(key, key*3)
			}
		}(i)
	}

	wg.Wait()

	// Verify map is in a valid state.
	expectedLen := numGoroutines * numOperations
	if sm.Len() != expectedLen {
		t.Errorf("Len() = %v; want %v", sm.Len(), expectedLen)
	}
}

// TestSyncMap_LoadOrStore_Concurrent tests LoadOrStore under concurrent access.
func TestSyncMap_LoadOrStore_Concurrent(t *testing.T) {
	sm := NewSyncMap[string, *sync.Mutex]()
	numGoroutines := 100
	key := "shared-key"

	// All goroutines try to LoadOrStore the same key.
	results := make([]*sync.Mutex, numGoroutines)
	var wg sync.WaitGroup

	for i := range numGoroutines {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			newMutex := &sync.Mutex{}
			actual, _ := sm.LoadOrStore(key, newMutex)
			results[idx] = actual
		}(i)
	}

	wg.Wait()

	// All goroutines should have received the same mutex instance.
	firstMutex := results[0]
	for i := 1; i < numGoroutines; i++ {
		if results[i] != firstMutex {
			t.Errorf("results[%d] != results[0]; expected all goroutines to get the same mutex", i)
		}
	}

	// Verify only one entry in the map.
	if sm.Len() != 1 {
		t.Errorf("Len() = %v; want 1", sm.Len())
	}
}

// TestSyncMap_TypeSafety demonstrates type safety with different types.
func TestSyncMap_TypeSafety(t *testing.T) {
	// String -> Int.
	intMap := NewSyncMap[string, int]()
	intMap.Store("count", 42)
	if val, _ := intMap.Load("count"); val != 42 {
		t.Errorf("int map: got %v, want 42", val)
	}

	// String -> String.
	stringMap := NewSyncMap[string, string]()
	stringMap.Store("name", "ListenUp")
	if val, _ := stringMap.Load("name"); val != "ListenUp" {
		t.Errorf("string map: got %v, want ListenUp", val)
	}

	// String -> *sync.Mutex (like our use case).
	mutexMap := NewSyncMap[string, *sync.Mutex]()
	mutex := &sync.Mutex{}
	mutexMap.Store("lock", mutex)
	if val, ok := mutexMap.Load("lock"); !ok || val != mutex {
		t.Error("mutex map: failed to retrieve correct mutex")
	}
}

// TestSyncMap_ZeroValue tests that Load returns zero value for missing keys.
func TestSyncMap_ZeroValue(t *testing.T) {
	intMap := NewSyncMap[string, int]()
	if val, ok := intMap.Load("missing"); ok || val != 0 {
		t.Errorf("Load(missing) = %v, %v; want 0, false", val, ok)
	}

	stringMap := NewSyncMap[string, string]()
	if val, ok := stringMap.Load("missing"); ok || val != "" {
		t.Errorf("Load(missing) = %v, %v; want empty string, false", val, ok)
	}

	ptrMap := NewSyncMap[string, *int]()
	if val, ok := ptrMap.Load("missing"); ok || val != nil {
		t.Errorf("Load(missing) = %v, %v; want nil, false", val, ok)
	}
}
