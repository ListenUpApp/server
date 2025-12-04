package api

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/auth"
	"github.com/listenupapp/listenup-server/internal/config"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/listenupapp/listenup-server/internal/media/images"
	"github.com/listenupapp/listenup-server/internal/scanner"
	"github.com/listenupapp/listenup-server/internal/service"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestServer creates a test server with all dependencies.
func setupTestServer(t *testing.T) (server *Server, cleanup func()) {
	t.Helper()

	// Create temp directory for test database.
	tmpDir, err := os.MkdirTemp("", "listenup-api-test-*")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Create a no-op logger for tests (discards all logs).
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create SSE manager for testing.
	sseManager := sse.NewManager(logger)
	sseHandler := sse.NewHandler(sseManager, logger)

	// Create store with SSE manager.
	s, err := store.New(dbPath, logger, sseManager)
	require.NoError(t, err)

	// Create scanner with SSE manager.
	fileScanner := scanner.NewScanner(s, sseManager, nil, logger)

	// Create test config.
	cfg := &config.Config{
		Server: config.ServerConfig{
			Name:      "Test Server",
			LocalURL:  "http://localhost:8080",
			RemoteURL: "",
		},
		Auth: config.AuthConfig{
			AccessTokenDuration:  15 * time.Minute,
			RefreshTokenDuration: 7 * 24 * time.Hour,
			AccessTokenKey:       []byte("test-secret-key-for-testing-only-32b"),
		},
	}

	// Create services.
	instanceService := service.NewInstanceService(s, logger, cfg)
	bookService := service.NewBookService(s, fileScanner, logger)
	collectionService := service.NewCollectionService(s, logger)
	sharingService := service.NewSharingService(s, logger)
	syncService := service.NewSyncService(s, logger)
	listeningService := service.NewListeningService(s, store.NewNoopEmitter(), logger)
	genreService := service.NewGenreService(s, logger)
	tagService := service.NewTagService(s, logger)

	// Create auth services.
	// Use a test key (32 bytes as hex = 64 hex chars)
	testKeyHex := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	tokenService, err := auth.NewTokenService(testKeyHex, cfg.Auth.AccessTokenDuration, cfg.Auth.RefreshTokenDuration)
	require.NoError(t, err)
	sessionService := service.NewSessionService(s, tokenService, logger)
	authService := service.NewAuthService(s, tokenService, sessionService, instanceService, logger)

	// Create image storage.
	imageStorage, err := images.NewStorage(tmpDir)
	require.NoError(t, err)

	// Create server (nil searchService - not testing search in these tests).
	server = NewServer(s, instanceService, authService, bookService, collectionService, sharingService, syncService, listeningService, genreService, tagService, nil, sseHandler, imageStorage, logger)

	// Return cleanup function.
	cleanup = func() {
		_ = s.Close()            //nolint:errcheck // Cleanup function, error already logged
		_ = os.RemoveAll(tmpDir) //nolint:errcheck // Cleanup function, nothing we can do about errors here
	}

	return server, cleanup
}

// createTestUserWithToken creates a test user and returns an access token.
func createTestUserWithToken(t *testing.T, server *Server) string {
	t.Helper()

	ctx := context.Background()

	// Create a test user
	userID, err := id.Generate("user")
	require.NoError(t, err)

	now := time.Now()
	user := &domain.User{
		Syncable: domain.Syncable{
			ID:        userID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Email:       "test@example.com",
		DisplayName: "Test User",
		IsRoot:      false,
	}

	err = server.store.CreateUser(ctx, user)
	require.NoError(t, err)

	// Create token service with same key as setupTestServer
	testKeyHex := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	tokenService, err := auth.NewTokenService(testKeyHex, 15*time.Minute, 7*24*time.Hour)
	require.NoError(t, err)

	// Generate an access token
	token, err := tokenService.GenerateAccessToken(user)
	require.NoError(t, err)

	return token
}

func TestHealthCheck(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/health", http.NoBody)
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

	// Initialize instance first.
	instanceService := server.instanceService
	ctx := httptest.NewRequest(http.MethodGet, "/", http.NoBody).Context()
	createdInstance, err := instanceService.InitializeInstance(ctx)
	require.NoError(t, err)

	// Make request.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instance", http.NoBody)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result response.Envelope
	err = json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.True(t, result.Success)
	assert.NotNil(t, result.Data)

	// Verify instance data.
	data, ok := result.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, createdInstance.ID, data["id"])
	assert.Equal(t, true, data["setup_required"])
}

func TestGetInstance_NotFound(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Don't initialize instance - should get 404.

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instance", http.NoBody)
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

	// Initialize instance and set root user.
	instanceService := server.instanceService
	ctx := httptest.NewRequest(http.MethodGet, "/", http.NoBody).Context()
	_, err := instanceService.InitializeInstance(ctx)
	require.NoError(t, err)

	err = instanceService.SetRootUser(ctx, "user_test_root")
	require.NoError(t, err)

	// Make request.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instance", http.NoBody)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result response.Envelope
	err = json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.True(t, result.Success)

	// Verify instance data.
	data, ok := result.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, data["setup_required"])
}

func TestServer_Routes(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Initialize instance for successful tests.
	instanceService := server.instanceService
	ctx := httptest.NewRequest(http.MethodGet, "/", http.NoBody).Context()
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
			req := httptest.NewRequest(tt.method, tt.path, http.NoBody)
			w := httptest.NewRecorder()

			server.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestServer_JSONResponse(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Initialize instance.
	instanceService := server.instanceService
	ctx := httptest.NewRequest(http.MethodGet, "/", http.NoBody).Context()
	instance, err := instanceService.InitializeInstance(ctx)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instance", http.NoBody)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	// Verify content type.
	assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))

	// Verify JSON structure.
	var result response.Envelope
	err = json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	// Verify envelope structure.
	assert.True(t, result.Success)
	assert.NotNil(t, result.Data)
	assert.Empty(t, result.Error)

	// Verify instance fields are present.
	data, ok := result.Data.(map[string]any)
	require.True(t, ok)
	assert.Contains(t, data, "id")
	assert.Contains(t, data, "setup_required")
	assert.Contains(t, data, "created_at")
	assert.Contains(t, data, "updated_at")

	// Verify timestamp parsing.
	createdAt, ok := data["created_at"].(string)
	require.True(t, ok)
	_, err = time.Parse(time.RFC3339Nano, createdAt)
	assert.NoError(t, err, "created_at should be valid RFC3339 timestamp")

	// Verify values match.
	assert.Equal(t, instance.ID, data["id"])
	assert.Equal(t, instance.IsSetupRequired(), data["setup_required"])
}

func TestGetManifest_Success(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Create a test user and get access token.
	token := createTestUserWithToken(t, server)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sync/manifest", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result response.Envelope
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.True(t, result.Success)
	assert.NotNil(t, result.Data)

	// Verify manifest data structure.
	data, ok := result.Data.(map[string]any)
	require.True(t, ok)
	assert.Contains(t, data, "library_version")
	assert.Contains(t, data, "checkpoint")
	assert.Contains(t, data, "counts")
	assert.Contains(t, data, "book_ids")

	// Verify counts structure.
	counts, ok := data["counts"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, counts, "books")
	assert.Contains(t, counts, "contributors")
	assert.Contains(t, counts, "series")

	// Verify empty library returns 0 books.
	assert.Equal(t, float64(0), counts["books"])
	assert.Equal(t, float64(0), counts["contributors"])
	assert.Equal(t, float64(0), counts["series"])

	// Verify timestamps are valid RFC3339.
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

	// Create a test user and get access token.
	token := createTestUserWithToken(t, server)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sync/books?limit=50", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result response.Envelope
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.True(t, result.Success)
	assert.NotNil(t, result.Data)

	// Verify books response structure.
	data, ok := result.Data.(map[string]any)
	require.True(t, ok)
	assert.Contains(t, data, "books")
	assert.Contains(t, data, "has_more")

	// Verify empty library returns empty books array.
	books, ok := data["books"].([]any)
	require.True(t, ok)
	assert.Empty(t, books)
	assert.Equal(t, false, data["has_more"])
}

// Test cover endpoint functionality.

func TestGetCover_Success(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Create a test cover image.
	bookID := "book-test-123"
	testCover := createTestJPEG(t, 1200, 1200)
	err := server.imageStorage.Save(bookID, testCover)
	require.NoError(t, err)

	// Request cover.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/covers/"+bookID, http.NoBody)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	// Verify response.
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "image/jpeg", w.Header().Get("Content-Type"))
	assert.NotEmpty(t, w.Header().Get("ETag"))
	assert.NotEmpty(t, w.Header().Get("Last-Modified"))
	assert.Equal(t, "public, max-age=604800", w.Header().Get("Cache-Control"))

	// Verify content matches.
	assert.Equal(t, testCover, w.Body.Bytes())

	// Verify it's a valid JPEG.
	_, err = jpeg.Decode(bytes.NewReader(w.Body.Bytes()))
	assert.NoError(t, err)
}

func TestGetCover_NotFound(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Request non-existent cover.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/covers/non-existent-book", http.NoBody)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	// Verify 404 response.
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Verify JSON error response.
	var result response.Envelope
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "Cover not found")
}

func TestGetCover_NotModified(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Create a test cover.
	bookID := "book-cache-test"
	testCover := createTestJPEG(t, 800, 800)
	err := server.imageStorage.Save(bookID, testCover)
	require.NoError(t, err)

	// First request - get ETag.
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/covers/"+bookID, http.NoBody)
	w1 := httptest.NewRecorder()

	server.ServeHTTP(w1, req1)
	require.Equal(t, http.StatusOK, w1.Code)

	etag := w1.Header().Get("ETag")
	require.NotEmpty(t, etag)

	// Second request with If-None-Match header.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/covers/"+bookID, http.NoBody)
	req2.Header.Set("If-None-Match", etag)
	w2 := httptest.NewRecorder()

	server.ServeHTTP(w2, req2)

	// Verify 304 response.
	assert.Equal(t, http.StatusNotModified, w2.Code)
	assert.Empty(t, w2.Body.Bytes(), "304 response should have no body")
}

func TestGetCover_ETagConsistency(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Create a test cover.
	bookID := "book-etag-test"
	testCover := createTestJPEG(t, 600, 600)
	err := server.imageStorage.Save(bookID, testCover)
	require.NoError(t, err)

	// Make multiple requests.
	etags := make([]string, 3)
	for i := range 3 {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/covers/"+bookID, http.NoBody)
		w := httptest.NewRecorder()

		server.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		etags[i] = w.Header().Get("ETag")
		require.NotEmpty(t, etags[i])
	}

	// All ETags should be identical.
	assert.Equal(t, etags[0], etags[1])
	assert.Equal(t, etags[1], etags[2])
}

func TestGetCover_CacheHeaders(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Create a test cover.
	bookID := "book-cache-headers-test"
	testCover := createTestJPEG(t, 400, 400)
	err := server.imageStorage.Save(bookID, testCover)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/covers/"+bookID, http.NoBody)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify all cache headers are set.
	assert.Equal(t, "public, max-age=604800", w.Header().Get("Cache-Control"))
	assert.NotEmpty(t, w.Header().Get("ETag"))
	assert.NotEmpty(t, w.Header().Get("Last-Modified"))

	// Verify Last-Modified is a valid HTTP date.
	lastModified := w.Header().Get("Last-Modified")
	_, err = http.ParseTime(lastModified)
	assert.NoError(t, err, "Last-Modified should be valid HTTP date")

	// Verify ETag is quoted.
	etag := w.Header().Get("ETag")
	assert.True(t, etag[0] == '"' && etag[len(etag)-1] == '"', "ETag should be quoted")
}

func TestGetCover_EmptyBookID(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Request with empty book ID.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/covers/", http.NoBody)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	// Should get 404 (route doesn't match).
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetCover_ContentLength(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Create a test cover.
	bookID := "book-content-length-test"
	testCover := createTestJPEG(t, 1200, 1200)
	err := server.imageStorage.Save(bookID, testCover)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/covers/"+bookID, http.NoBody)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify Content-Length header.
	contentLength := w.Header().Get("Content-Length")
	assert.NotEmpty(t, contentLength)
	assert.Equal(t, len(testCover), w.Body.Len())
}

// Helper function to create a test JPEG image.
func createTestJPEG(t *testing.T, width, height int) []byte {
	t.Helper()

	// Create test image with gradient.
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			r := uint8((x * 255) / width)
			g := uint8((y * 255) / height)
			b := uint8(((x + y) * 255) / (width + height))
			img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}

	// Encode as JPEG.
	var buf bytes.Buffer
	err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	require.NoError(t, err)

	return buf.Bytes()
}
