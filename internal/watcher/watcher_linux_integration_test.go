//go:build linux

package watcher

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLinuxBackend_FileCreation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	opts := Options{
		IgnoreHidden: true,
	}
	opts.setDefaults()

	backend, err := newLinuxBackend(logger, opts)
	require.NoError(t, err)
	defer backend.Stop()

	tmpDir := t.TempDir()
	err = backend.Watch(tmpDir)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go backend.Start(ctx)

	// Give the backend a moment to start
	time.Sleep(50 * time.Millisecond)

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.m4b")
	err = os.WriteFile(testFile, []byte("test audiobook content"), 0644)
	require.NoError(t, err)

	// Wait for event - should be fast on Linux with IN_CLOSE_WRITE
	select {
	case event := <-backend.Events():
		assert.Equal(t, EventAdded, event.Type)
		assert.Equal(t, testFile, event.Path)
		assert.Equal(t, int64(22), event.Size)
		assert.NotZero(t, event.Inode)
		t.Logf("Event received: %+v", event)
	case err := <-backend.Errors():
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for event - IN_CLOSE_WRITE should be instant")
	}
}

func TestLinuxBackend_FileDeletion(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	opts := Options{}
	opts.setDefaults()

	backend, err := newLinuxBackend(logger, opts)
	require.NoError(t, err)
	defer backend.Stop()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	// Create file first
	err = os.WriteFile(testFile, []byte("content"), 0644)
	require.NoError(t, err)

	err = backend.Watch(tmpDir)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go backend.Start(ctx)

	time.Sleep(50 * time.Millisecond)

	// Delete the file
	err = os.Remove(testFile)
	require.NoError(t, err)

	// Wait for deletion event
	select {
	case event := <-backend.Events():
		assert.Equal(t, EventRemoved, event.Type)
		assert.Equal(t, testFile, event.Path)
		t.Logf("Deletion event received: %+v", event)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for deletion event")
	}
}

func TestLinuxBackend_NewDirectoryWatching(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	opts := Options{}
	opts.setDefaults()

	backend, err := newLinuxBackend(logger, opts)
	require.NoError(t, err)
	defer backend.Stop()

	tmpDir := t.TempDir()
	err = backend.Watch(tmpDir)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go backend.Start(ctx)

	time.Sleep(50 * time.Millisecond)

	// Create new subdirectory
	subDir := filepath.Join(tmpDir, "newdir")
	err = os.Mkdir(subDir, 0755)
	require.NoError(t, err)

	// Give time for the directory to be watched
	time.Sleep(100 * time.Millisecond)

	// Create file in new subdirectory
	testFile := filepath.Join(subDir, "file.txt")
	err = os.WriteFile(testFile, []byte("content in new dir"), 0644)
	require.NoError(t, err)

	// Should receive event for file in new directory
	select {
	case event := <-backend.Events():
		assert.Equal(t, testFile, event.Path)
		t.Logf("Event in new directory: %+v", event)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for event in new directory")
	}
}

func TestLinuxBackend_IgnoreHidden(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	opts := Options{
		IgnoreHidden: true,
	}
	opts.setDefaults()

	backend, err := newLinuxBackend(logger, opts)
	require.NoError(t, err)
	defer backend.Stop()

	tmpDir := t.TempDir()
	err = backend.Watch(tmpDir)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go backend.Start(ctx)

	time.Sleep(50 * time.Millisecond)

	// Create hidden file
	hiddenFile := filepath.Join(tmpDir, ".hidden")
	err = os.WriteFile(hiddenFile, []byte("secret"), 0644)
	require.NoError(t, err)

	// Create normal file
	normalFile := filepath.Join(tmpDir, "normal.txt")
	err = os.WriteFile(normalFile, []byte("content"), 0644)
	require.NoError(t, err)

	// Should only get event for normal file
	select {
	case event := <-backend.Events():
		assert.Equal(t, normalFile, event.Path)
		t.Logf("Event received: %+v", event)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for event")
	}

	// Should not get event for hidden file
	select {
	case event := <-backend.Events():
		t.Fatalf("unexpected event for hidden file: %+v", event)
	case <-time.After(200 * time.Millisecond):
		// Good, no event for hidden file
		t.Log("Correctly ignored hidden file")
	}
}
