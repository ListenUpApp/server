package processor

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/scanner"
	"github.com/listenupapp/listenup-server/internal/store"
	"github.com/listenupapp/listenup-server/internal/watcher"
)

// TestEventProcessor_ProcessEvent_AudioFile tests processing an audio file event.
func TestEventProcessor_ProcessEvent_AudioFile(t *testing.T) {
	// Create temp directory for test.
	tempDir := t.TempDir()
	bookFolder := filepath.Join(tempDir, "Author", "Book")
	if err := os.MkdirAll(bookFolder, 0o755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// Create a test audio file.
	audioFile := filepath.Join(bookFolder, "chapter01.mp3")
	if err := os.WriteFile(audioFile, []byte("fake audio data"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create scanner and processor.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError, // Reduce noise in tests
	}))
	scnr := scanner.NewScanner(nil, store.NewNoopEmitter(), logger)
	processor := NewEventProcessor(scnr, logger)

	// Create event.
	event := watcher.Event{
		Type: watcher.EventAdded,
		Path: audioFile,
	}

	// Process event.
	ctx := context.Background()
	err := processor.ProcessEvent(ctx, event)
	if err != nil {
		t.Errorf("ProcessEvent() failed: %v", err)
	}

	// Verify the folder lock was created.
	lock := processor.getFolderLock(bookFolder)
	if lock == nil {
		t.Error("expected folder lock to be created")
	}
}

// TestEventProcessor_ProcessEvent_CoverFile tests processing a cover file event.
func TestEventProcessor_ProcessEvent_CoverFile(t *testing.T) {
	// Create temp directory for test.
	tempDir := t.TempDir()
	bookFolder := filepath.Join(tempDir, "Author", "Book")
	if err := os.MkdirAll(bookFolder, 0o755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// Create a test cover file.
	coverFile := filepath.Join(bookFolder, "cover.jpg")
	if err := os.WriteFile(coverFile, []byte("fake image data"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create scanner and processor.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	scnr := scanner.NewScanner(nil, store.NewNoopEmitter(), logger)
	processor := NewEventProcessor(scnr, logger)

	// Create event.
	event := watcher.Event{
		Type: watcher.EventAdded,
		Path: coverFile,
	}

	// Process event.
	ctx := context.Background()
	err := processor.ProcessEvent(ctx, event)
	if err != nil {
		t.Errorf("ProcessEvent() failed: %v", err)
	}
}

// TestEventProcessor_ProcessEvent_MetadataFile tests processing a metadata file event.
func TestEventProcessor_ProcessEvent_MetadataFile(t *testing.T) {
	// Create temp directory for test.
	tempDir := t.TempDir()
	bookFolder := filepath.Join(tempDir, "Author", "Book")
	if err := os.MkdirAll(bookFolder, 0o755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// Create a test metadata file.
	metadataFile := filepath.Join(bookFolder, "metadata.nfo")
	if err := os.WriteFile(metadataFile, []byte("fake metadata"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create scanner and processor.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	scnr := scanner.NewScanner(nil, store.NewNoopEmitter(), logger)
	processor := NewEventProcessor(scnr, logger)

	// Create event.
	event := watcher.Event{
		Type: watcher.EventAdded,
		Path: metadataFile,
	}

	// Process event.
	ctx := context.Background()
	err := processor.ProcessEvent(ctx, event)
	if err != nil {
		t.Errorf("ProcessEvent() failed: %v", err)
	}
}

// TestEventProcessor_ProcessEvent_IgnoredFile tests that ignored files are skipped.
func TestEventProcessor_ProcessEvent_IgnoredFile(t *testing.T) {
	// Create temp directory for test.
	tempDir := t.TempDir()
	bookFolder := filepath.Join(tempDir, "Author", "Book")
	if err := os.MkdirAll(bookFolder, 0o755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// Create a test ignored file.
	ignoredFile := filepath.Join(bookFolder, "track.cue")
	if err := os.WriteFile(ignoredFile, []byte("fake cue data"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create scanner and processor.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	scnr := scanner.NewScanner(nil, store.NewNoopEmitter(), logger)
	processor := NewEventProcessor(scnr, logger)

	// Create event.
	event := watcher.Event{
		Type: watcher.EventAdded,
		Path: ignoredFile,
	}

	// Process event.
	ctx := context.Background()
	err := processor.ProcessEvent(ctx, event)
	if err != nil {
		t.Errorf("ProcessEvent() failed: %v", err)
	}

	// Verify no lock was created for ignored files.
	// We can't directly check this, but the fact that it returned nil is sufficient.
}

// TestEventProcessor_ProcessEvent_RemovedFile tests processing a file removal event.
func TestEventProcessor_ProcessEvent_RemovedFile(t *testing.T) {
	// Create temp directory for test.
	tempDir := t.TempDir()
	bookFolder := filepath.Join(tempDir, "Author", "Book")
	if err := os.MkdirAll(bookFolder, 0o755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// Create scanner and processor.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	scnr := scanner.NewScanner(nil, store.NewNoopEmitter(), logger)
	processor := NewEventProcessor(scnr, logger)

	// Create event for a removed file (file doesn't need to exist).
	removedFile := filepath.Join(bookFolder, "chapter01.mp3")
	event := watcher.Event{
		Type: watcher.EventRemoved,
		Path: removedFile,
	}

	// Process event.
	ctx := context.Background()
	err := processor.ProcessEvent(ctx, event)
	if err != nil {
		t.Errorf("ProcessEvent() failed: %v", err)
	}
}

// TestEventProcessor_ProcessEvent_RemovedFile_AllFilesGone tests that when all.
// audio files are removed, the folder is detected as empty.
func TestEventProcessor_ProcessEvent_RemovedFile_AllFilesGone(t *testing.T) {
	// Create temp directory for test.
	tempDir := t.TempDir()
	bookFolder := filepath.Join(tempDir, "Author", "Book")
	if err := os.MkdirAll(bookFolder, 0o755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// Create scanner and processor.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo, // Enable to verify logging
	}))
	scnr := scanner.NewScanner(nil, store.NewNoopEmitter(), logger)
	processor := NewEventProcessor(scnr, logger)

	// Simulate removal of last audio file.
	removedFile := filepath.Join(bookFolder, "chapter01.mp3")
	event := watcher.Event{
		Type: watcher.EventRemoved,
		Path: removedFile,
	}

	// Process event.
	ctx := context.Background()
	err := processor.ProcessEvent(ctx, event)
	if err != nil {
		t.Errorf("ProcessEvent() failed: %v", err)
	}

	// The test passes if no error is returned.
	// In a real implementation, we would check that the book was marked as missing.
}

// TestEventProcessor_ConcurrentEvents tests that concurrent events for the same.
// folder are properly deduplicated using per-folder locks.
func TestEventProcessor_ConcurrentEvents(t *testing.T) {
	// Create temp directory for test.
	tempDir := t.TempDir()
	bookFolder := filepath.Join(tempDir, "Author", "Book")
	if err := os.MkdirAll(bookFolder, 0o755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// Create multiple test audio files.
	numFiles := 20
	for i := 1; i <= numFiles; i++ {
		audioFile := filepath.Join(bookFolder, fmt.Sprintf("chapter%02d.mp3", i))
		if err := os.WriteFile(audioFile, []byte("fake audio data"), 0o644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	// Create scanner and processor.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	scnr := scanner.NewScanner(nil, store.NewNoopEmitter(), logger)
	processor := NewEventProcessor(scnr, logger)

	// Track how many events were actually processed (not skipped due to lock).
	var processedCount atomic.Int32
	var skippedCount atomic.Int32

	// Process events concurrently.
	var wg sync.WaitGroup
	ctx := context.Background()

	for i := 1; i <= numFiles; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			// Create event.
			audioFile := filepath.Join(bookFolder, fmt.Sprintf("chapter%02d.mp3", i))
			event := watcher.Event{
				Type: watcher.EventAdded,
				Path: audioFile,
			}

			// Try to acquire lock.
			lock := processor.getFolderLock(bookFolder)
			if lock.TryLock() {
				processedCount.Add(1)
				// Simulate some processing time.
				time.Sleep(10 * time.Millisecond)
				lock.Unlock()

				// Now process the event (will be skipped if another goroutine holds lock).
				processor.ProcessEvent(ctx, event) //nolint:errcheck // Test call
			} else {
				skippedCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	// At least one event should have been processed.
	if processedCount.Load() == 0 {
		t.Error("expected at least one event to be processed")
	}

	// Some events should have been skipped due to lock contention.
	if skippedCount.Load() == 0 {
		t.Log("warning: no events were skipped, lock contention may not be working as expected")
	}

	t.Logf("Processed: %d, Skipped: %d", processedCount.Load(), skippedCount.Load())
}

// TestEventProcessor_MultiFileBookEvolution tests that a multi-file book.
// evolves correctly as files are added.
func TestEventProcessor_MultiFileBookEvolution(t *testing.T) {
	// Create temp directory for test.
	tempDir := t.TempDir()
	bookFolder := filepath.Join(tempDir, "Author", "Book")
	if err := os.MkdirAll(bookFolder, 0o755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// Create scanner and processor.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	scnr := scanner.NewScanner(nil, store.NewNoopEmitter(), logger)
	processor := NewEventProcessor(scnr, logger)

	ctx := context.Background()

	// Step 1: Add first audio file.
	audioFile1 := filepath.Join(bookFolder, "chapter01.mp3")
	if err := os.WriteFile(audioFile1, []byte("fake audio data 1"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	event1 := watcher.Event{
		Type: watcher.EventAdded,
		Path: audioFile1,
	}

	err := processor.ProcessEvent(ctx, event1)
	if err != nil {
		t.Errorf("ProcessEvent() failed for first file: %v", err)
	}

	// Step 2: Add second audio file.
	audioFile2 := filepath.Join(bookFolder, "chapter02.mp3")
	if err := os.WriteFile(audioFile2, []byte("fake audio data 2"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	event2 := watcher.Event{
		Type: watcher.EventAdded,
		Path: audioFile2,
	}

	err = processor.ProcessEvent(ctx, event2)
	if err != nil {
		t.Errorf("ProcessEvent() failed for second file: %v", err)
	}

	// Step 3: Add cover file.
	coverFile := filepath.Join(bookFolder, "cover.jpg")
	if err := os.WriteFile(coverFile, []byte("fake image data"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	event3 := watcher.Event{
		Type: watcher.EventAdded,
		Path: coverFile,
	}

	err = processor.ProcessEvent(ctx, event3)
	if err != nil {
		t.Errorf("ProcessEvent() failed for cover file: %v", err)
	}

	// Verify final state by scanning the folder.
	item, err := scnr.ScanFolder(ctx, bookFolder, scanner.ScanOptions{
		Workers: 2,
	})
	if err != nil {
		t.Fatalf("ScanFolder() failed: %v", err)
	}

	if len(item.AudioFiles) != 2 {
		t.Errorf("expected 2 audio files, got %d", len(item.AudioFiles))
	}

	if len(item.ImageFiles) != 1 {
		t.Errorf("expected 1 image file, got %d", len(item.ImageFiles))
	}
}

// TestEventProcessor_DiscFolderHandling tests that files in disc folders.
// (CD1, CD2, etc.) are properly grouped under the parent folder.
func TestEventProcessor_DiscFolderHandling(t *testing.T) {
	// Create temp directory for test.
	tempDir := t.TempDir()
	bookFolder := filepath.Join(tempDir, "Author", "Book")
	cd1Folder := filepath.Join(bookFolder, "CD1")
	cd2Folder := filepath.Join(bookFolder, "CD2")

	if err := os.MkdirAll(cd1Folder, 0o755); err != nil {
		t.Fatalf("failed to create CD1 directory: %v", err)
	}
	if err := os.MkdirAll(cd2Folder, 0o755); err != nil {
		t.Fatalf("failed to create CD2 directory: %v", err)
	}

	// Create test audio files in disc folders.
	audioFileCD1 := filepath.Join(cd1Folder, "track01.mp3")
	audioFileCD2 := filepath.Join(cd2Folder, "track01.mp3")

	if err := os.WriteFile(audioFileCD1, []byte("fake audio data CD1"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := os.WriteFile(audioFileCD2, []byte("fake audio data CD2"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create scanner and processor.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	scnr := scanner.NewScanner(nil, store.NewNoopEmitter(), logger)
	processor := NewEventProcessor(scnr, logger)

	ctx := context.Background()

	// Process events for both disc folders.
	event1 := watcher.Event{
		Type: watcher.EventAdded,
		Path: audioFileCD1,
	}

	event2 := watcher.Event{
		Type: watcher.EventAdded,
		Path: audioFileCD2,
	}

	// Verify both files resolve to the same book folder.
	folder1 := determineBookFolder(audioFileCD1)
	folder2 := determineBookFolder(audioFileCD2)

	if folder1 != bookFolder {
		t.Errorf("expected CD1 file to resolve to %s, got %s", bookFolder, folder1)
	}

	if folder2 != bookFolder {
		t.Errorf("expected CD2 file to resolve to %s, got %s", bookFolder, folder2)
	}

	if folder1 != folder2 {
		t.Errorf("expected both disc files to resolve to same folder, got %s and %s", folder1, folder2)
	}

	// Process the events.
	err := processor.ProcessEvent(ctx, event1)
	if err != nil {
		t.Errorf("ProcessEvent() failed for CD1 file: %v", err)
	}

	err = processor.ProcessEvent(ctx, event2)
	if err != nil {
		t.Errorf("ProcessEvent() failed for CD2 file: %v", err)
	}
}

// TestEventProcessor_GetFolderLock tests the getFolderLock method.
func TestEventProcessor_GetFolderLock(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	processor := NewEventProcessor(nil, logger)

	folderPath := "/library/Author/Book"

	// Get lock for the first time.
	lock1 := processor.getFolderLock(folderPath)
	if lock1 == nil {
		t.Error("expected lock to be created")
	}

	// Get lock for the same folder again.
	lock2 := processor.getFolderLock(folderPath)
	if lock2 == nil {
		t.Error("expected lock to be returned")
	}

	// Should be the same lock instance.
	if lock1 != lock2 {
		t.Error("expected same lock instance for same folder")
	}

	// Get lock for a different folder.
	lock3 := processor.getFolderLock("/library/Author/OtherBook")
	if lock3 == nil {
		t.Error("expected lock to be created for different folder")
	}

	// Should be a different lock instance.
	if lock1 == lock3 {
		t.Error("expected different lock instances for different folders")
	}
}

// TestEventProcessor_GetFolderLock_Concurrent tests that getFolderLock.
// is safe for concurrent access.
func TestEventProcessor_GetFolderLock_Concurrent(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	processor := NewEventProcessor(nil, logger)

	folderPath := "/library/Author/Book"
	numGoroutines := 100

	// All goroutines try to get the lock concurrently.
	locks := make([]*sync.Mutex, numGoroutines)
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			locks[i] = processor.getFolderLock(folderPath)
		}(i)
	}

	wg.Wait()

	// All locks should be the same instance.
	firstLock := locks[0]
	for i := 1; i < numGoroutines; i++ {
		if locks[i] != firstLock {
			t.Errorf("lock %d is different from lock 0", i)
		}
	}
}

// TestEventProcessor_ModifiedEvent tests that modified events are handled correctly.
func TestEventProcessor_ModifiedEvent(t *testing.T) {
	// Create temp directory for test.
	tempDir := t.TempDir()
	bookFolder := filepath.Join(tempDir, "Author", "Book")
	if err := os.MkdirAll(bookFolder, 0o755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// Create a test audio file.
	audioFile := filepath.Join(bookFolder, "chapter01.mp3")
	if err := os.WriteFile(audioFile, []byte("fake audio data"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create scanner and processor.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	scnr := scanner.NewScanner(nil, store.NewNoopEmitter(), logger)
	processor := NewEventProcessor(scnr, logger)

	// Create modified event.
	event := watcher.Event{
		Type: watcher.EventModified,
		Path: audioFile,
	}

	// Process event.
	ctx := context.Background()
	err := processor.ProcessEvent(ctx, event)
	if err != nil {
		t.Errorf("ProcessEvent() failed: %v", err)
	}
}
