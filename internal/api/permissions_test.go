package api

import (
	"context"
	"encoding/hex"
	"encoding/json/v2"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
	"github.com/listenupapp/listenup-server/internal/dto"
	"github.com/listenupapp/listenup-server/internal/service"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
)

// permTestServer wraps the API server for permission testing.
type permTestServer struct {
	*Server
	api          humatest.TestAPI
	cleanup      func()
	tokenService *auth.TokenService
}

// setupPermTestServer creates a test server with routes needed for permission testing.
func setupPermTestServer(t *testing.T) *permTestServer {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "listenup-perm-test-*")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")

	st, err := store.New(dbPath, nil, store.NewNoopEmitter())
	require.NoError(t, err)

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

	authKey, err := auth.LoadOrGenerateKey(tmpDir)
	require.NoError(t, err)
	cfg.Auth.AccessTokenKey = authKey

	tokenService, err := auth.NewTokenService(
		hex.EncodeToString(authKey),
		cfg.Auth.AccessTokenDuration,
		cfg.Auth.RefreshTokenDuration,
	)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	sseManager := sse.NewManager(logger)
	enricher := dto.NewEnricher(st)

	sessionService := service.NewSessionService(st, tokenService, logger)
	instanceService := service.NewInstanceService(st, logger, cfg)
	authService := service.NewAuthService(st, tokenService, sessionService, instanceService, logger)
	syncService := service.NewSyncService(st, logger)
	sharingService := service.NewSharingService(st, logger)
	shelfService := service.NewShelfService(st, sseManager, logger)
	inboxService := service.NewInboxService(st, enricher, sseManager, logger)

	services := &Services{
		Instance: instanceService,
		Auth:     authService,
		Sync:     syncService,
		Sharing:  sharingService,
		Shelf:    shelfService,
		Inbox:    inboxService,
	}

	router := chi.NewRouter()
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
		sseManager:      sseManager,
		authRateLimiter: NewRateLimiter(100, time.Minute, 50),
	}

	s.registerHealthRoutes()
	s.registerInstanceRoutes()
	s.registerAuthRoutes()
	s.registerBookRoutes()
	s.registerShareRoutes()
	s.registerContributorRoutes()
	s.registerSeriesRoutes()
	s.registerAudioRoutes()

	_, err = services.Instance.InitializeInstance(context.Background())
	require.NoError(t, err)

	testAPI := humatest.Wrap(t, api)

	cleanup := func() {
		_ = st.Close()
		_ = os.RemoveAll(tmpDir)
	}

	return &permTestServer{
		Server:       s,
		api:          testAPI,
		cleanup:      cleanup,
		tokenService: tokenService,
	}
}

// createPermUser creates the admin user via setup and returns token + userID.
func (ts *permTestServer) createPermUser(t *testing.T) (token string, userID string) {
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

	claims, err := ts.tokenService.VerifyAccessToken(envelope.Data.AccessToken)
	require.NoError(t, err)

	return envelope.Data.AccessToken, claims.UserID
}

// setPermissions updates a user's permissions directly in the store.
func (ts *permTestServer) setPermissions(t *testing.T, userID string, perms domain.UserPermissions) {
	t.Helper()
	ctx := context.Background()

	user, err := ts.store.GetUser(ctx, userID)
	require.NoError(t, err)

	user.Permissions = perms
	err = ts.store.UpdateUser(ctx, user)
	require.NoError(t, err)
}

// createBook creates a book directly in the store.
func (ts *permTestServer) createBook(t *testing.T, bookID, ownerID string) {
	t.Helper()
	ctx := context.Background()

	now := time.Now()
	book := &domain.Book{
		Syncable: domain.Syncable{
			ID:        bookID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title:         "Test Book",
		Path:          "/test/path/" + bookID,
		TotalDuration: 3600000,
		AudioFiles: []domain.AudioFileInfo{
			{
				ID:       "af-1",
				Path:     "/test/path/" + bookID + "/book.m4b",
				Filename: "book.m4b",
				Size:     1024000,
				Duration: 3600000,
				Format:   "m4b",
				Inode:    1001,
				ModTime:  now.Unix(),
			},
		},
	}

	err := ts.store.CreateBook(ctx, book)
	require.NoError(t, err)
}

func TestCanEdit_UpdateBook_Forbidden(t *testing.T) {
	ts := setupPermTestServer(t)
	defer ts.cleanup()

	token, userID := ts.createPermUser(t)
	ts.createBook(t, "book-1", userID)

	// Disable edit permission
	ts.setPermissions(t, userID, domain.UserPermissions{
		CanDownload: true,
		CanShare:    true,
		CanEdit:     false,
	})

	resp := ts.api.Patch("/api/v1/books/book-1",
		"Authorization: Bearer "+token,
		map[string]any{"title": "New Title"},
	)

	assert.Equal(t, http.StatusForbidden, resp.Code)
}

func TestCanEdit_UpdateBook_Allowed(t *testing.T) {
	ts := setupPermTestServer(t)
	defer ts.cleanup()

	token, userID := ts.createPermUser(t)
	ts.createBook(t, "book-1", userID)

	// Edit permission enabled (default)
	resp := ts.api.Patch("/api/v1/books/book-1",
		"Authorization: Bearer "+token,
		map[string]any{"title": "New Title"},
	)

	assert.Equal(t, http.StatusOK, resp.Code)
}

func TestCanEdit_SetBookContributors_Forbidden(t *testing.T) {
	ts := setupPermTestServer(t)
	defer ts.cleanup()

	token, userID := ts.createPermUser(t)
	ts.createBook(t, "book-1", userID)

	ts.setPermissions(t, userID, domain.UserPermissions{
		CanDownload: true,
		CanShare:    true,
		CanEdit:     false,
	})

	resp := ts.api.Put("/api/v1/books/book-1/contributors",
		"Authorization: Bearer "+token,
		map[string]any{"contributors": []map[string]any{
			{"name": "Author", "roles": []string{"author"}},
		}},
	)

	assert.Equal(t, http.StatusForbidden, resp.Code)
}

func TestCanEdit_SetBookSeries_Forbidden(t *testing.T) {
	ts := setupPermTestServer(t)
	defer ts.cleanup()

	token, userID := ts.createPermUser(t)
	ts.createBook(t, "book-1", userID)

	ts.setPermissions(t, userID, domain.UserPermissions{
		CanDownload: true,
		CanShare:    true,
		CanEdit:     false,
	})

	resp := ts.api.Put("/api/v1/books/book-1/series",
		"Authorization: Bearer "+token,
		map[string]any{"series": []map[string]any{
			{"name": "Series", "sequence": "1"},
		}},
	)

	assert.Equal(t, http.StatusForbidden, resp.Code)
}

func TestCanShare_ShareCollection_Forbidden(t *testing.T) {
	ts := setupPermTestServer(t)
	defer ts.cleanup()

	token, userID := ts.createPermUser(t)

	// Create a collection to share
	ctx := context.Background()
	coll := &domain.Collection{
		ID:        "coll-1",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Name:    "Test Collection",
		OwnerID: userID,
	}
	err := ts.store.CreateCollection(ctx, coll)
	require.NoError(t, err)

	// Disable share permission
	ts.setPermissions(t, userID, domain.UserPermissions{
		CanDownload: true,
		CanShare:    false,
		CanEdit:     true,
	})

	resp := ts.api.Post("/api/v1/collections/coll-1/shares",
		"Authorization: Bearer "+token,
		map[string]any{
			"user_id":    "some-user-id",
			"permission": "read",
		},
	)

	assert.Equal(t, http.StatusForbidden, resp.Code)
}

func TestCanDownload_StreamAudio_Forbidden(t *testing.T) {
	ts := setupPermTestServer(t)
	defer ts.cleanup()

	token, userID := ts.createPermUser(t)
	ts.createBook(t, "book-1", userID)

	// Disable download permission
	ts.setPermissions(t, userID, domain.UserPermissions{
		CanDownload: false,
		CanShare:    true,
		CanEdit:     true,
	})

	// Test audio streaming endpoint directly (chi route, not huma)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audio/book-1/af-1?token="+token, nil)
	w := httptest.NewRecorder()
	ts.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestCanDownload_StreamAudio_Allowed(t *testing.T) {
	ts := setupPermTestServer(t)
	defer ts.cleanup()

	token, userID := ts.createPermUser(t)
	ts.createBook(t, "book-1", userID)

	// Download permission enabled (default)
	// This will return 500 because the file doesn't exist on disk,
	// but it should NOT return 403
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audio/book-1/af-1?token="+token, nil)
	w := httptest.NewRecorder()
	ts.router.ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusForbidden, w.Code)
}

func TestCanDownload_TranscodedAudio_Forbidden(t *testing.T) {
	ts := setupPermTestServer(t)
	defer ts.cleanup()

	token, userID := ts.createPermUser(t)
	ts.createBook(t, "book-1", userID)

	ts.setPermissions(t, userID, domain.UserPermissions{
		CanDownload: false,
		CanShare:    true,
		CanEdit:     true,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audio/book-1/af-1/transcode/index.m3u8?token="+token, nil)
	w := httptest.NewRecorder()
	ts.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestDefaultPermissions_AllEnabled(t *testing.T) {
	perms := domain.DefaultPermissions()
	assert.True(t, perms.CanDownload)
	assert.True(t, perms.CanShare)
	assert.True(t, perms.CanEdit)
}

func TestUser_CanEdit_Method(t *testing.T) {
	user := &domain.User{
		Permissions: domain.UserPermissions{CanEdit: true},
	}
	assert.True(t, user.CanEdit())

	user.Permissions.CanEdit = false
	assert.False(t, user.CanEdit())
}
