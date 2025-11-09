//go:build linux

package watcher

import (
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLinuxBackend(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	opts := Options{}
	opts.setDefaults()

	backend, err := newLinuxBackend(logger, opts)
	require.NoError(t, err)
	require.NotNil(t, backend)

	// Clean up
	err = backend.Stop()
	assert.NoError(t, err)
}

func TestLinuxBackend_Channels(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	opts := Options{}
	opts.setDefaults()

	backend, err := newLinuxBackend(logger, opts)
	require.NoError(t, err)
	defer backend.Stop()

	assert.NotNil(t, backend.Events(), "Events channel should not be nil")
	assert.NotNil(t, backend.Errors(), "Errors channel should not be nil")
}
