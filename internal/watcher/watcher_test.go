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

func TestNew(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	w, err := New(logger, Options{})
	require.NoError(t, err)
	require.NotNil(t, w)

	err = w.Stop()
	assert.NoError(t, err)
}

func TestWatcher_Watch(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	w, err := New(logger, Options{})
	require.NoError(t, err)
	defer w.Stop() //nolint:errcheck // Test cleanup

	tmpDir := t.TempDir()
	err = w.Watch(tmpDir)
	assert.NoError(t, err)
}

func TestWatcher_FileCreation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	opts := Options{
		SettleDelay: 50 * time.Millisecond,
	}

	w, err := New(logger, opts)
	require.NoError(t, err)
	defer w.Stop() //nolint:errcheck // Test cleanup

	tmpDir := t.TempDir()
	err = w.Watch(tmpDir)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go w.Start(ctx) //nolint:errcheck // Test goroutine

	// Create a test file.
	testFile := filepath.Join(tmpDir, "test.m4b")
	err = os.WriteFile(testFile, []byte("test audiobook content"), 0o644)
	require.NoError(t, err)

	// Wait for event.
	select {
	case event := <-w.Events():
		assert.Equal(t, testFile, event.Path)
		assert.Equal(t, int64(22), event.Size)
		assert.NotZero(t, event.Inode)
	case err := <-w.Errors():
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestWatcher_FileDeletion(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	w, err := New(logger, Options{})
	require.NoError(t, err)
	defer w.Stop() //nolint:errcheck // Test cleanup

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	// Create file first.
	err = os.WriteFile(testFile, []byte("content"), 0o644)
	require.NoError(t, err)

	err = w.Watch(tmpDir)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go w.Start(ctx) //nolint:errcheck // Test goroutine

	// Delete the file.
	err = os.Remove(testFile)
	require.NoError(t, err)

	// Wait for event.
	select {
	case event := <-w.Events():
		assert.Equal(t, EventRemoved, event.Type)
		assert.Equal(t, testFile, event.Path)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for deletion event")
	}
}

func TestWatcher_IgnoreHidden(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	opts := Options{
		IgnoreHidden: true,
		SettleDelay:  50 * time.Millisecond,
	}

	w, err := New(logger, opts)
	require.NoError(t, err)
	defer w.Stop() //nolint:errcheck // Test cleanup

	tmpDir := t.TempDir()
	err = w.Watch(tmpDir)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go w.Start(ctx) //nolint:errcheck // Test goroutine

	// Create hidden file.
	hiddenFile := filepath.Join(tmpDir, ".hidden")
	err = os.WriteFile(hiddenFile, []byte("secret"), 0o644)
	require.NoError(t, err)

	// Create normal file.
	normalFile := filepath.Join(tmpDir, "normal.txt")
	err = os.WriteFile(normalFile, []byte("content"), 0o644)
	require.NoError(t, err)

	// Should only get event for normal file.
	select {
	case event := <-w.Events():
		assert.Equal(t, normalFile, event.Path)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for event")
	}

	// Should not get event for hidden file.
	select {
	case event := <-w.Events():
		t.Fatalf("unexpected event for hidden file: %+v", event)
	case <-time.After(200 * time.Millisecond):
		// Good, no event for hidden file.
	}
}
