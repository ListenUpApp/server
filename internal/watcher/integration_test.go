//go:build integration

package watcher

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_LargeFileDetection tests detection of large files
func TestIntegration_LargeFileDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	w, err := New(logger, Options{})
	require.NoError(t, err)
	defer w.Stop()

	tmpDir := t.TempDir()
	err = w.Watch(tmpDir)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go w.Start(ctx)

	// Create a "large" file (10MB)
	testFile := filepath.Join(tmpDir, "large.m4b")
	largeContent := make([]byte, 10*1024*1024) // 10MB

	// Write in chunks to simulate real file transfer
	f, err := os.Create(testFile)
	require.NoError(t, err)

	chunkSize := 1024 * 1024 // 1MB chunks
	for i := 0; i < len(largeContent); i += chunkSize {
		end := i + chunkSize
		if end > len(largeContent) {
			end = len(largeContent)
		}
		_, err := f.Write(largeContent[i:end])
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond) // Simulate transfer delay
	}
	f.Close()

	// Wait for event
	select {
	case event := <-w.Events():
		assert.Equal(t, testFile, event.Path)
		assert.Equal(t, int64(len(largeContent)), event.Size)
		t.Logf("Event received after: %v", time.Since(time.Now()))
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for large file event")
	}
}

// TestIntegration_MultipleRapidChanges tests handling of rapid file changes
func TestIntegration_MultipleRapidChanges(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	opts := Options{
		SettleDelay: 100 * time.Millisecond,
	}

	w, err := New(logger, opts)
	require.NoError(t, err)
	defer w.Stop()

	tmpDir := t.TempDir()
	err = w.Watch(tmpDir)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go w.Start(ctx)

	testFile := filepath.Join(tmpDir, "rapid.txt")

	// Make rapid changes
	numWrites := 10
	for i := 0; i < numWrites; i++ {
		err = os.WriteFile(testFile, []byte(fmt.Sprintf("content %d", i)), 0644)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)
	}

	// Collect events
	eventCount := 0
	timeout := time.After(1 * time.Second)

	for {
		select {
		case event := <-w.Events():
			eventCount++
			assert.Equal(t, testFile, event.Path)
			t.Logf("Event %d received", eventCount)
		case <-timeout:
			// Platform-specific expectations:
			// - Linux (IN_CLOSE_WRITE): Gets all 10 events immediately (no debouncing)
			// - Fallback (fsnotify): Gets 1 debounced event after settling
			// Both behaviors are correct for their platform!
			if eventCount == 1 {
				t.Logf("Fallback backend: received 1 debounced event (expected)")
			} else if eventCount == numWrites {
				t.Logf("Linux backend: received %d events (expected with IN_CLOSE_WRITE)", numWrites)
			} else {
				t.Fatalf("unexpected event count: got %d, expected either 1 (fallback) or %d (Linux)", eventCount, numWrites)
			}
			return
		}
	}
}

// TestIntegration_NewDirectoryDetection tests automatic watching of new directories
func TestIntegration_NewDirectoryDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	w, err := New(logger, Options{})
	require.NoError(t, err)
	defer w.Stop()

	tmpDir := t.TempDir()
	err = w.Watch(tmpDir)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go w.Start(ctx)

	// Create new subdirectory
	subDir := filepath.Join(tmpDir, "newdir")
	err = os.Mkdir(subDir, 0755)
	require.NoError(t, err)

	// Wait a bit for directory watch to be added
	time.Sleep(100 * time.Millisecond)

	// Create file in new subdirectory
	testFile := filepath.Join(subDir, "file.txt")
	err = os.WriteFile(testFile, []byte("content"), 0644)
	require.NoError(t, err)

	// Should receive event for file in new directory
	select {
	case event := <-w.Events():
		assert.Equal(t, testFile, event.Path)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for event in new directory")
	}
}
