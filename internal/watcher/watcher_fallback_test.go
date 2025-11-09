//go:build !linux

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

func TestNewFallbackBackend(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	opts := Options{}
	opts.setDefaults()

	backend, err := newFallbackBackend(logger, opts)
	require.NoError(t, err)
	require.NotNil(t, backend)

	err = backend.Stop()
	assert.NoError(t, err)
}

func TestFallbackBackend_Watch(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	opts := Options{}
	opts.setDefaults()

	backend, err := newFallbackBackend(logger, opts)
	require.NoError(t, err)
	defer backend.Stop()

	// Create a temporary directory
	tmpDir := t.TempDir()

	// Watch the directory
	err = backend.Watch(tmpDir)
	assert.NoError(t, err)
}

func TestFallbackBackend_Debouncing(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	opts := Options{
		SettleDelay: 50 * time.Millisecond,
	}
	opts.setDefaults()

	backend, err := newFallbackBackend(logger, opts)
	require.NoError(t, err)
	defer backend.Stop()

	tmpDir := t.TempDir()
	err = backend.Watch(tmpDir)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go backend.Start(ctx)

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	err = os.WriteFile(testFile, []byte("initial content"), 0644)
	require.NoError(t, err)

	// Wait for debounced event
	select {
	case event := <-backend.Events():
		assert.Equal(t, testFile, event.Path)
		assert.NotZero(t, event.Size)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for event")
	}
}
