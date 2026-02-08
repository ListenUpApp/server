package api

import (
	"context"
	"encoding/hex"
	"encoding/json/v2"
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
	"github.com/listenupapp/listenup-server/internal/auth"
	"github.com/listenupapp/listenup-server/internal/config"
	"github.com/listenupapp/listenup-server/internal/dto"
	"github.com/listenupapp/listenup-server/internal/service"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
	"github.com/stretchr/testify/require"
)

// testServer wraps the API server for testing with humatest.
type testServer struct {
	*Server
	api     humatest.TestAPI
	cleanup func()
}

// setupTestServer creates a test server with a temporary database.
func setupTestServer(t *testing.T) *testServer {
	t.Helper()

	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "listenup-api-test-*")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Create store with no-op emitter
	st, err := store.New(dbPath, nil, store.NewNoopEmitter())
	require.NoError(t, err)

	// Create test config
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

	// Load or generate auth key
	authKey, err := auth.LoadOrGenerateKey(tmpDir)
	require.NoError(t, err)
	cfg.Auth.AccessTokenKey = authKey

	// Create token service
	tokenService, err := auth.NewTokenService(
		hex.EncodeToString(authKey),
		cfg.Auth.AccessTokenDuration,
		cfg.Auth.RefreshTokenDuration,
	)
	require.NoError(t, err)

	// Create logger (discard output in tests)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create SSE manager and enricher for services that emit events
	sseManager := sse.NewManager(logger)
	enricher := dto.NewEnricher(st)

	// Create services
	sessionService := service.NewSessionService(st, tokenService, logger)
	instanceService := service.NewInstanceService(st, logger, cfg)
	authService := service.NewAuthService(st, tokenService, sessionService, instanceService, logger)
	syncService := service.NewSyncService(st, logger)
	shelfService := service.NewShelfService(st, sseManager, logger)
	inboxService := service.NewInboxService(st, enricher, sseManager, logger)
	settingsService := service.NewSettingsService(st, inboxService, logger)

	services := &Services{
		Instance: instanceService,
		Auth:     authService,
		Sync:     syncService,
		Shelf:     shelfService,
		Settings: settingsService,
		Inbox:    inboxService,
	}

	// Create chi router
	router := chi.NewRouter()

	// Add auth middleware before routes
	router.Use(authMiddleware(services.Auth))

	// Configure huma API
	humaConfig := huma.DefaultConfig("ListenUp API Test", "1.0.0")
	humaConfig.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearer": {
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "PASETO",
		},
	}

	// Wrap all responses in {success, data, error} envelope for client compatibility
	humaConfig.Transformers = append(humaConfig.Transformers, EnvelopeTransformer)

	api := humachi.New(router, humaConfig)

	// Register custom error handler
	RegisterErrorHandler()

	// Create server
	s := &Server{
		store:           st,
		services:        services,
		router:          router,
		api:             api,
		logger:          logger,
		authRateLimiter: NewRateLimiter(100, time.Minute, 50), // Higher limits for testing
	}

	// Register routes
	s.registerHealthRoutes()
	s.registerInstanceRoutes()
	s.registerAuthRoutes()
	s.registerSyncRoutes()

	// Initialize instance (required before setup can work)
	_, err = services.Instance.InitializeInstance(context.Background())
	require.NoError(t, err)

	// Wrap with humatest
	testAPI := humatest.Wrap(t, api)

	cleanup := func() {
		_ = st.Close()
		_ = os.RemoveAll(tmpDir)
	}

	return &testServer{
		Server:  s,
		api:     testAPI,
		cleanup: cleanup,
	}
}

// testEnvelope wraps API responses for tests.
type testEnvelope[T any] struct {
	Version int    `json:"v"`
	Success bool   `json:"success"`
	Data    T      `json:"data"`
	Error   string `json:"error,omitempty"`
}

// createTestUserAndLogin creates a user through setup and returns the access token.
func (ts *testServer) createTestUserAndLogin(t *testing.T) string {
	t.Helper()

	// Setup creates the first admin user
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

	return envelope.Data.AccessToken
}
