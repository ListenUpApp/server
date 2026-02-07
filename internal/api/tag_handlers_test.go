package api

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/listenupapp/listenup-server/internal/auth"
	"github.com/listenupapp/listenup-server/internal/config"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/service"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
)

// tagTestServer wraps the API server for tag testing.
type tagTestServer struct {
	*Server
	api          humatest.TestAPI
	cleanup      func()
	sseManager   *sse.Manager
	tokenService *auth.TokenService
}

// setupTagTestServer creates a test server with tag support.
func setupTagTestServer(t *testing.T) *tagTestServer {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "listenup-tag-test-*")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Create store with noop emitter.
	st, err := store.New(dbPath, nil, store.NewNoopEmitter())
	require.NoError(t, err)

	// Create test config.
	cfg := &config.Config{
		Server: config.ServerConfig{
			Name:      "Test Server",
			LocalURL:  "http://localhost:8080",
			RemoteURL: "",
		},
		Auth: config.AuthConfig{
			AccessTokenDuration:  15 * time.Minute,
			RefreshTokenDuration: 30 * 24 * time.Hour,
		},
	}

	// Load or generate auth key.
	authKey, err := auth.LoadOrGenerateKey(tmpDir)
	require.NoError(t, err)
	cfg.Auth.AccessTokenKey = authKey

	// Create token service.
	tokenService, err := auth.NewTokenService(
		hex.EncodeToString(authKey),
		cfg.Auth.AccessTokenDuration,
		cfg.Auth.RefreshTokenDuration,
	)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create SSE manager.
	sseManager := sse.NewManager(logger)

	// Create services.
	sessionService := service.NewSessionService(st, tokenService, logger)
	instanceService := service.NewInstanceService(st, logger, cfg)
	authService := service.NewAuthService(st, tokenService, sessionService, instanceService, logger)
	syncService := service.NewSyncService(st, logger)
	tagService := service.NewTagService(st, sseManager, nil, logger) // nil search for tests
	shelfService := service.NewShelfService(st, sseManager, logger)

	services := &Services{
		Instance: instanceService,
		Auth:     authService,
		Sync:     syncService,
		Tag:      tagService,
		Shelf:     shelfService,
	}

	router := chi.NewRouter()

	// Add auth middleware before routes
	router.Use(authMiddleware(services.Auth))

	humaConfig := huma.DefaultConfig("ListenUp API Test", "1.0.0")
	humaConfig.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearer": {
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "PASETO",
		},
	}
	humaConfig.Transformers = append(humaConfig.Transformers, EnvelopeTransformer)

	api := humachi.New(router, humaConfig)
	RegisterErrorHandler()

	s := &Server{
		store:           st,
		services:        services,
		router:          router,
		api:             api,
		logger:          logger,
		authRateLimiter: NewRateLimiter(100, time.Minute, 50),
	}

	// Register routes.
	s.registerHealthRoutes()
	s.registerInstanceRoutes()
	s.registerAuthRoutes()
	s.registerTagRoutes()

	// Initialize instance.
	_, err = services.Instance.InitializeInstance(context.Background())
	require.NoError(t, err)

	testAPI := humatest.Wrap(t, api)

	cleanup := func() {
		_ = st.Close()
		_ = os.RemoveAll(tmpDir)
	}

	return &tagTestServer{
		Server:       s,
		api:          testAPI,
		cleanup:      cleanup,
		sseManager:   sseManager,
		tokenService: tokenService,
	}
}

// createTagTestUser creates a user and returns the access token and user ID.
func (ts *tagTestServer) createTagTestUser(t *testing.T) (token string, userID string) {
	t.Helper()

	resp := ts.api.Post("/api/v1/auth/setup", map[string]any{
		"email":      "admin@test.com",
		"password":   "TestPassword123!",
		"first_name": "Test",
		"last_name":  "Admin",
	})

	require.Equal(t, http.StatusOK, resp.Code, "Setup failed: %s", resp.Body.String())

	var envelope testEnvelope[AuthResponse]
	err := json.Unmarshal(resp.Body.Bytes(), &envelope)
	require.NoError(t, err)

	// Extract user ID from token.
	claims, err := ts.tokenService.VerifyAccessToken(envelope.Data.AccessToken)
	require.NoError(t, err)

	return envelope.Data.AccessToken, claims.UserID
}

// createTestBook creates a book (accessible to all since it's not in any collection).
func (ts *tagTestServer) createTestBook(t *testing.T, bookID string) {
	t.Helper()
	ctx := context.Background()

	now := time.Now()
	book := &domain.Book{
		Syncable: domain.Syncable{
			ID:        bookID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title:         "Test Book " + bookID,
		Path:          "/test/audiobooks/" + bookID,
		TotalDuration: 300000,
		TotalSize:     1024000,
		ScannedAt:     now,
	}
	err := ts.store.CreateBook(ctx, book)
	require.NoError(t, err)
}

// createBookInCollection creates a book in a collection owned by a specific user.
func (ts *tagTestServer) createBookInCollection(t *testing.T, bookID, collectionID, ownerID string) {
	t.Helper()
	ctx := context.Background()

	// Ensure library exists.
	lib := &domain.Library{
		ID:        "test-lib",
		Name:      "Test Library",
		ScanPaths: []string{"/test"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_ = ts.store.CreateLibrary(ctx, lib) // Ignore if exists.

	// Ensure collection exists.
	coll := &domain.Collection{
		ID:        collectionID,
		LibraryID: lib.ID,
		OwnerID:   ownerID,
		Name:      "Test Collection",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_ = ts.store.CreateCollection(ctx, coll) // Ignore if exists.

	// Create book.
	now := time.Now()
	book := &domain.Book{
		Syncable: domain.Syncable{
			ID:        bookID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title:         "Book in Collection",
		Path:          "/test/audiobooks/" + bookID,
		TotalDuration: 300000,
		TotalSize:     1024000,
		ScannedAt:     now,
	}
	err := ts.store.CreateBook(ctx, book)
	require.NoError(t, err)

	// Add book to collection.
	err = ts.store.AdminAddBookToCollection(ctx, bookID, collectionID)
	require.NoError(t, err)
}

// === Tests ===

func TestListTags_EmptyInitially(t *testing.T) {
	ts := setupTagTestServer(t)
	defer ts.cleanup()

	token, _ := ts.createTagTestUser(t)

	resp := ts.api.Get("/api/v1/tags", "Authorization: Bearer "+token)
	assert.Equal(t, http.StatusOK, resp.Code)

	var envelope testEnvelope[ListTagsResponse]
	err := json.Unmarshal(resp.Body.Bytes(), &envelope)
	require.NoError(t, err)

	assert.True(t, envelope.Success)
	assert.Empty(t, envelope.Data.Tags)
}

func TestListTags_SortedByBookCount(t *testing.T) {
	ts := setupTagTestServer(t)
	defer ts.cleanup()

	token, _ := ts.createTagTestUser(t)

	// Create two books.
	ts.createTestBook(t, "book-1")
	ts.createTestBook(t, "book-2")

	// Add tags: "popular" on both books, "rare" on one book.
	resp := ts.api.Post("/api/v1/books/book-1/tags",
		map[string]any{"tag": "popular"},
		"Authorization: Bearer "+token)
	require.Equal(t, http.StatusOK, resp.Code)

	resp = ts.api.Post("/api/v1/books/book-2/tags",
		map[string]any{"tag": "popular"},
		"Authorization: Bearer "+token)
	require.Equal(t, http.StatusOK, resp.Code)

	resp = ts.api.Post("/api/v1/books/book-1/tags",
		map[string]any{"tag": "rare"},
		"Authorization: Bearer "+token)
	require.Equal(t, http.StatusOK, resp.Code)

	// List tags - should be sorted by book count (popular=2, rare=1).
	resp = ts.api.Get("/api/v1/tags", "Authorization: Bearer "+token)
	assert.Equal(t, http.StatusOK, resp.Code)

	var envelope testEnvelope[ListTagsResponse]
	err := json.Unmarshal(resp.Body.Bytes(), &envelope)
	require.NoError(t, err)

	require.Len(t, envelope.Data.Tags, 2)
	assert.Equal(t, "popular", envelope.Data.Tags[0].Slug)
	assert.Equal(t, 2, envelope.Data.Tags[0].BookCount)
	assert.Equal(t, "rare", envelope.Data.Tags[1].Slug)
	assert.Equal(t, 1, envelope.Data.Tags[1].BookCount)
}

func TestAddTagToBook_NormalizesInput(t *testing.T) {
	ts := setupTagTestServer(t)
	defer ts.cleanup()

	token, _ := ts.createTagTestUser(t)
	ts.createTestBook(t, "book-1")

	tests := []struct {
		input        string
		expectedSlug string
	}{
		{"Slow Burn", "slow-burn"},
		{"FOUND FAMILY", "found-family"},
		{"   enemies to lovers   ", "enemies-to-lovers"},
		{"dark_romance", "dark-romance"},
		{"sci-fi", "sci-fi"},
		{"M/M Romance", "m-m-romance"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			resp := ts.api.Post("/api/v1/books/book-1/tags",
				map[string]any{"tag": tt.input},
				"Authorization: Bearer "+token)
			require.Equal(t, http.StatusOK, resp.Code)

			var envelope testEnvelope[TagDTO]
			err := json.Unmarshal(resp.Body.Bytes(), &envelope)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedSlug, envelope.Data.Slug)
		})
	}
}

func TestAddTagToBook_CreatesNewOrFindsExisting(t *testing.T) {
	ts := setupTagTestServer(t)
	defer ts.cleanup()

	token, _ := ts.createTagTestUser(t)
	ts.createTestBook(t, "book-1")

	// First add - creates new tag.
	resp := ts.api.Post("/api/v1/books/book-1/tags",
		map[string]any{"tag": "new-tag"},
		"Authorization: Bearer "+token)
	require.Equal(t, http.StatusOK, resp.Code)

	var firstEnvelope testEnvelope[TagDTO]
	err := json.Unmarshal(resp.Body.Bytes(), &firstEnvelope)
	require.NoError(t, err)

	firstTagID := firstEnvelope.Data.ID
	assert.NotEmpty(t, firstTagID)
	assert.Equal(t, "new-tag", firstEnvelope.Data.Slug)
	assert.Equal(t, 1, firstEnvelope.Data.BookCount)

	// Create second book.
	ts.createTestBook(t, "book-2")

	// Second add with same tag name (different case) - finds existing.
	resp = ts.api.Post("/api/v1/books/book-2/tags",
		map[string]any{"tag": "New Tag"}, // Different case, same slug
		"Authorization: Bearer "+token)
	require.Equal(t, http.StatusOK, resp.Code)

	var secondEnvelope testEnvelope[TagDTO]
	err = json.Unmarshal(resp.Body.Bytes(), &secondEnvelope)
	require.NoError(t, err)

	// Same tag ID, increased book count.
	assert.Equal(t, firstTagID, secondEnvelope.Data.ID)
	assert.Equal(t, "new-tag", secondEnvelope.Data.Slug)
	assert.Equal(t, 2, secondEnvelope.Data.BookCount)
}

func TestAddTagToBook_Idempotent(t *testing.T) {
	ts := setupTagTestServer(t)
	defer ts.cleanup()

	token, _ := ts.createTagTestUser(t)
	ts.createTestBook(t, "book-1")

	// Add same tag twice.
	resp := ts.api.Post("/api/v1/books/book-1/tags",
		map[string]any{"tag": "test-tag"},
		"Authorization: Bearer "+token)
	require.Equal(t, http.StatusOK, resp.Code)

	resp = ts.api.Post("/api/v1/books/book-1/tags",
		map[string]any{"tag": "test-tag"},
		"Authorization: Bearer "+token)
	require.Equal(t, http.StatusOK, resp.Code)

	var envelope testEnvelope[TagDTO]
	err := json.Unmarshal(resp.Body.Bytes(), &envelope)
	require.NoError(t, err)

	// Book count should still be 1 (not incremented twice).
	assert.Equal(t, 1, envelope.Data.BookCount)
}

func TestRemoveTagFromBook_Idempotent(t *testing.T) {
	ts := setupTagTestServer(t)
	defer ts.cleanup()

	token, _ := ts.createTagTestUser(t)
	ts.createTestBook(t, "book-1")

	// Add tag.
	resp := ts.api.Post("/api/v1/books/book-1/tags",
		map[string]any{"tag": "test-tag"},
		"Authorization: Bearer "+token)
	require.Equal(t, http.StatusOK, resp.Code)

	// Remove tag - first time.
	resp = ts.api.Delete("/api/v1/books/book-1/tags/test-tag",
		"Authorization: Bearer "+token)
	assert.Equal(t, http.StatusOK, resp.Code)

	// Remove tag - second time (already removed, but idempotent).
	resp = ts.api.Delete("/api/v1/books/book-1/tags/test-tag",
		"Authorization: Bearer "+token)
	assert.Equal(t, http.StatusOK, resp.Code)
}

func TestRemoveTagFromBook_TagNotFound(t *testing.T) {
	ts := setupTagTestServer(t)
	defer ts.cleanup()

	token, _ := ts.createTagTestUser(t)
	ts.createTestBook(t, "book-1")

	// Try to remove non-existent tag.
	resp := ts.api.Delete("/api/v1/books/book-1/tags/nonexistent",
		"Authorization: Bearer "+token)
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func TestGetTagBooks_FiltersByUserAccess(t *testing.T) {
	ts := setupTagTestServer(t)
	defer ts.cleanup()

	token, userID := ts.createTagTestUser(t)

	// Create book-1 (public, not in collection).
	ts.createTestBook(t, "book-1")

	// Add tag to book-1.
	resp := ts.api.Post("/api/v1/books/book-1/tags",
		map[string]any{"tag": "shared-tag"},
		"Authorization: Bearer "+token)
	require.Equal(t, http.StatusOK, resp.Code)

	// Create book-2 in a collection owned by a DIFFERENT user (user has no access).
	ts.createBookInCollection(t, "book-2", "private-coll", "other-user-id")

	// Add same tag to book-2 directly via store.
	ctx := context.Background()
	tag, _, err := ts.store.FindOrCreateTagBySlug(ctx, "shared-tag")
	require.NoError(t, err)
	err = ts.store.AddTagToBook(ctx, "book-2", tag.ID)
	require.NoError(t, err)

	// Get books for tag - should only return book-1 (user has access).
	resp = ts.api.Get("/api/v1/tags/shared-tag/books", "Authorization: Bearer "+token)
	assert.Equal(t, http.StatusOK, resp.Code)

	var envelope testEnvelope[GetTagBooksResponse]
	err = json.Unmarshal(resp.Body.Bytes(), &envelope)
	require.NoError(t, err)

	assert.Equal(t, "shared-tag", envelope.Data.Tag.Slug)
	assert.Equal(t, 1, envelope.Data.Total)
	require.Len(t, envelope.Data.Books, 1)
	assert.Equal(t, "book-1", envelope.Data.Books[0].ID)

	// Suppress unused variable warning.
	_ = userID
}

func TestAddTagToBook_AccessDenied(t *testing.T) {
	ts := setupTagTestServer(t)
	defer ts.cleanup()

	token, _ := ts.createTagTestUser(t)

	// Create a book in a collection owned by someone else.
	ts.createBookInCollection(t, "private-book", "private-coll", "other-user-id")

	// Try to add tag - should be forbidden.
	resp := ts.api.Post("/api/v1/books/private-book/tags",
		map[string]any{"tag": "test"},
		"Authorization: Bearer "+token)
	assert.Equal(t, http.StatusForbidden, resp.Code)
}

func TestGetBookTags_Success(t *testing.T) {
	ts := setupTagTestServer(t)
	defer ts.cleanup()

	token, _ := ts.createTagTestUser(t)
	ts.createTestBook(t, "book-1")

	// Add multiple tags.
	for _, tag := range []string{"tag-a", "tag-b", "tag-c"} {
		resp := ts.api.Post("/api/v1/books/book-1/tags",
			map[string]any{"tag": tag},
			"Authorization: Bearer "+token)
		require.Equal(t, http.StatusOK, resp.Code)
	}

	// Get book tags.
	resp := ts.api.Get("/api/v1/books/book-1/tags", "Authorization: Bearer "+token)
	assert.Equal(t, http.StatusOK, resp.Code)

	var envelope testEnvelope[GetBookTagsResponse]
	err := json.Unmarshal(resp.Body.Bytes(), &envelope)
	require.NoError(t, err)

	assert.Len(t, envelope.Data.Tags, 3)
}

func TestGetTagBySlug_Success(t *testing.T) {
	ts := setupTagTestServer(t)
	defer ts.cleanup()

	token, _ := ts.createTagTestUser(t)
	ts.createTestBook(t, "book-1")

	// Create tag.
	resp := ts.api.Post("/api/v1/books/book-1/tags",
		map[string]any{"tag": "specific-tag"},
		"Authorization: Bearer "+token)
	require.Equal(t, http.StatusOK, resp.Code)

	// Get tag by slug.
	resp = ts.api.Get("/api/v1/tags/specific-tag", "Authorization: Bearer "+token)
	assert.Equal(t, http.StatusOK, resp.Code)

	var envelope testEnvelope[TagDTO]
	err := json.Unmarshal(resp.Body.Bytes(), &envelope)
	require.NoError(t, err)

	assert.Equal(t, "specific-tag", envelope.Data.Slug)
	assert.Equal(t, 1, envelope.Data.BookCount)
}

func TestGetTagBySlug_NotFound(t *testing.T) {
	ts := setupTagTestServer(t)
	defer ts.cleanup()

	token, _ := ts.createTagTestUser(t)

	resp := ts.api.Get("/api/v1/tags/nonexistent", "Authorization: Bearer "+token)
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func TestAddTagToBook_EmptyAfterNormalization(t *testing.T) {
	ts := setupTagTestServer(t)
	defer ts.cleanup()

	token, _ := ts.createTagTestUser(t)
	ts.createTestBook(t, "book-1")

	// Try to add tag that becomes empty after normalization.
	resp := ts.api.Post("/api/v1/books/book-1/tags",
		map[string]any{"tag": "!!!@@@###"},
		"Authorization: Bearer "+token)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)
}

func TestAddTagToBook_BookNotFound(t *testing.T) {
	ts := setupTagTestServer(t)
	defer ts.cleanup()

	token, _ := ts.createTagTestUser(t)

	resp := ts.api.Post("/api/v1/books/nonexistent/tags",
		map[string]any{"tag": "test"},
		"Authorization: Bearer "+token)
	assert.Equal(t, http.StatusNotFound, resp.Code)
}
