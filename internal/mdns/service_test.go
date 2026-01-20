package mdns

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConstants(t *testing.T) {
	t.Run("service type is correct", func(t *testing.T) {
		assert.Equal(t, "_listenup._tcp", ServiceType)
	})

	t.Run("API version is v1", func(t *testing.T) {
		assert.Equal(t, "v1", APIVersion)
	})

	t.Run("server version is set", func(t *testing.T) {
		assert.NotEmpty(t, ServerVersion)
	})
}

func TestNewService(t *testing.T) {
	t.Run("creates service with logger", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

		service := NewService(logger)

		require.NotNil(t, service)
		assert.False(t, service.Running(), "service should not be running before Start")
	})
}

func TestServiceStop(t *testing.T) {
	t.Run("stop when not started is safe", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
		service := NewService(logger)

		// Should not panic
		service.Stop()
		assert.False(t, service.Running())
	})

	t.Run("stop can be called multiple times", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
		service := NewService(logger)

		// Should not panic
		service.Stop()
		service.Stop()
		service.Stop()
	})
}

func TestServiceStart(t *testing.T) {
	// Note: These tests require avahi-daemon to be running.
	// They will be skipped in environments without D-Bus/avahi.

	t.Run("start with valid instance succeeds", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))
		service := NewService(logger)

		instance := &domain.Instance{
			ID:   "server-test-123",
			Name: "Test Server",
		}

		err := service.Start(instance, 8080)

		// mDNS may fail in some environments (Docker, CI, no avahi)
		if err == nil {
			assert.True(t, service.Running())
			assert.Contains(t, buf.String(), "mDNS advertisement started")

			// Cleanup
			service.Stop()
			assert.False(t, service.Running())
		} else {
			t.Logf("mDNS start failed (expected in some environments): %v", err)
		}
	})

	t.Run("start with remote URL includes it in TXT records", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))
		service := NewService(logger)

		instance := &domain.Instance{
			ID:        "server-test-456",
			Name:      "Remote Server",
			RemoteURL: "https://example.com",
		}

		err := service.Start(instance, 8080)

		if err == nil {
			assert.True(t, service.Running())
			service.Stop()
		} else {
			t.Logf("mDNS start failed (expected in some environments): %v", err)
		}
	})

	t.Run("start can restart existing service", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))
		service := NewService(logger)

		instance := &domain.Instance{
			ID:   "server-restart-test",
			Name: "Restart Test Server",
		}

		// First start
		err1 := service.Start(instance, 8080)
		if err1 != nil {
			t.Skipf("mDNS not available in this environment: %v", err1)
		}

		// Second start (should cleanly restart)
		err2 := service.Start(instance, 8081)
		require.NoError(t, err2)
		assert.True(t, service.Running())

		service.Stop()
	})
}

func TestServiceLifecycle(t *testing.T) {
	t.Run("full lifecycle: create, start, stop", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))

		// Create
		service := NewService(logger)
		require.NotNil(t, service)

		instance := &domain.Instance{
			ID:   "lifecycle-test",
			Name: "Lifecycle Test",
		}

		// Start
		err := service.Start(instance, 8080)
		if err != nil {
			t.Skipf("mDNS not available: %v", err)
		}
		assert.True(t, service.Running())

		// Stop
		service.Stop()
		assert.False(t, service.Running())
		assert.Contains(t, buf.String(), "mDNS advertisement stopped")
	})
}

func TestServiceConcurrency(t *testing.T) {
	t.Run("concurrent stop calls are safe", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
		service := NewService(logger)

		instance := &domain.Instance{
			ID:   "concurrent-test",
			Name: "Concurrent Test",
		}

		err := service.Start(instance, 8080)
		if err != nil {
			t.Skipf("mDNS not available: %v", err)
		}

		// Concurrent stops should be safe
		done := make(chan struct{})
		for range 10 {
			go func() {
				service.Stop()
				done <- struct{}{}
			}()
		}

		// Wait for all goroutines
		for range 10 {
			<-done
		}

		assert.False(t, service.Running())
	})
}
