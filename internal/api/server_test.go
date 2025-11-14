package api

import (
	"encoding/json/v2"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/scanner"
	"github.com/listenupapp/listenup-server/internal/service"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestServer creates a test server with all dependencies
func setupTestServer(t *testing.T) (*Server, func()) {
	t.Helper()

	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "listenup-api-test-*")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Create a no-op logger for tests (discards all logs)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create SSE manager for testing
	sseManager := sse.NewManager(logger)
	sseHandler := sse.NewHandler(sseManager, logger)

	// Create store with SSE manager
	s, err := store.New(dbPath, logger, sseManager)
	require.NoError(t, err)

	// Create scanner
	fileScanner := scanner.NewScanner(s, logger)

	// Create services
	instanceService := service.NewInstanceService(s, logger)
	bookService := service.NewBookService(s, fileScanner, logger)
	syncService := service.NewSyncService(s, logger)

	// Create server
	server := NewServer(instanceService, bookService, syncService, sseHandler, logger)

	// Return cleanup function
	cleanup := func() {
		s.Close()
		os.RemoveAll(tmpDir)
	}

	return server, cleanup
}

func TestHealthCheck(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result response.Envelope
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.True(t, result.Success)
	assert.NotNil(t, result.Data)
}

func TestGetInstance_Success(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Initialize instance first
	instanceService := server.instanceService
	ctx := httptest.NewRequest(http.MethodGet, "/", nil).Context()
	createdInstance, err := instanceService.InitializeInstance(ctx)
	require.NoError(t, err)

	// Make request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instance", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result response.Envelope
	err = json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.True(t, result.Success)
	assert.NotNil(t, result.Data)

	// Verify instance data
	data, ok := result.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, createdInstance.ID, data["id"])
	assert.Equal(t, false, data["has_root_user"])
}

func TestGetInstance_NotFound(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Don't initialize instance - should get 404

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instance", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var result response.Envelope
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.NotEmpty(t, result.Error)
	assert.Contains(t, result.Error, "not found")
}

func TestGetInstance_WithRootUser(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Initialize instance and mark as setup
	instanceService := server.instanceService
	ctx := httptest.NewRequest(http.MethodGet, "/", nil).Context()
	_, err := instanceService.InitializeInstance(ctx)
	require.NoError(t, err)

	err = instanceService.MarkInstanceAsSetup(ctx)
	require.NoError(t, err)

	// Make request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instance", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result response.Envelope
	err = json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.True(t, result.Success)

	// Verify instance data
	data, ok := result.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, data["has_root_user"])
}

func TestServer_Routes(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Initialize instance for successful tests
	instanceService := server.instanceService
	ctx := httptest.NewRequest(http.MethodGet, "/", nil).Context()
	_, err := instanceService.InitializeInstance(ctx)
	require.NoError(t, err)

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
	}{
		{
			name:           "health check",
			method:         http.MethodGet,
			path:           "/health",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "get instance",
			method:         http.MethodGet,
			path:           "/api/v1/instance",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "not found",
			method:         http.MethodGet,
			path:           "/api/v1/nonexistent",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "old endpoint",
			method:         http.MethodGet,
			path:           "/api/v1/server",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			server.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestServer_JSONResponse(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Initialize instance
	instanceService := server.instanceService
	ctx := httptest.NewRequest(http.MethodGet, "/", nil).Context()
	instance, err := instanceService.InitializeInstance(ctx)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instance", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	// Verify content type
	assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))

	// Verify JSON structure
	var result response.Envelope
	err = json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	// Verify envelope structure
	assert.True(t, result.Success)
	assert.NotNil(t, result.Data)
	assert.Empty(t, result.Error)

	// Verify instance fields are present
	data, ok := result.Data.(map[string]any)
	require.True(t, ok)
	assert.Contains(t, data, "id")
	assert.Contains(t, data, "has_root_user")
	assert.Contains(t, data, "created_at")
	assert.Contains(t, data, "updated_at")

	// Verify timestamp parsing
	createdAt, ok := data["created_at"].(string)
	require.True(t, ok)
	_, err = time.Parse(time.RFC3339Nano, createdAt)
	assert.NoError(t, err, "created_at should be valid RFC3339 timestamp")

	// Verify values match
	assert.Equal(t, instance.ID, data["id"])
	assert.Equal(t, instance.HasRootUser, data["has_root_user"])
}

func TestGetManifest_Success(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// No need to initialize instance for manifest endpoint
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sync/manifest", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result response.Envelope
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.True(t, result.Success)
	assert.NotNil(t, result.Data)

	// Verify manifest data structure
	data, ok := result.Data.(map[string]any)
	require.True(t, ok)
	assert.Contains(t, data, "library_version")
	assert.Contains(t, data, "checkpoint")
	assert.Contains(t, data, "counts")
	assert.Contains(t, data, "book_ids")

	// Verify counts structure
	counts, ok := data["counts"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, counts, "books")
	assert.Contains(t, counts, "authors")
	assert.Contains(t, counts, "series")

	// Verify empty library returns 0 books
	assert.Equal(t, float64(0), counts["books"])
	assert.Equal(t, float64(0), counts["authors"])
	assert.Equal(t, float64(0), counts["series"])

	// Verify timestamps are valid RFC3339
	libraryVersion, ok := data["library_version"].(string)
	require.True(t, ok)
	_, err = time.Parse(time.RFC3339, libraryVersion)
	assert.NoError(t, err)

	checkpoint, ok := data["checkpoint"].(string)
	require.True(t, ok)
	_, err = time.Parse(time.RFC3339, checkpoint)
	assert.NoError(t, err)
}

func TestGetSyncBooks_Success(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sync/books?limit=50", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result response.Envelope
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.True(t, result.Success)
	assert.NotNil(t, result.Data)

	// Verify books response structure
	data, ok := result.Data.(map[string]any)
	require.True(t, ok)
	assert.Contains(t, data, "books")
	assert.Contains(t, data, "has_more")

	// Verify empty library returns empty books array
	books, ok := data["books"].([]any)
	require.True(t, ok)
	assert.Empty(t, books)
	assert.Equal(t, false, data["has_more"])
}
