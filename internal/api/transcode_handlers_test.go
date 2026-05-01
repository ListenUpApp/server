package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"encoding/hex"
	"encoding/json/v2"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/go-chi/chi/v5"
	"github.com/listenupapp/listenup-server/internal/auth"
	"github.com/listenupapp/listenup-server/internal/config"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/dto"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/listenupapp/listenup-server/internal/service"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// transcodeTestServer wraps the API server for transcode endpoint testing.
type transcodeTestServer struct {
	*Server
	api          humatest.TestAPI
	tokenService *auth.TokenService
}

// setupTranscodeAPITestServer creates a test server with transcode support.
func setupTranscodeAPITestServer(t *testing.T) *transcodeTestServer {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "listenup-transcode-api-test-*")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")
	cachePath := filepath.Join(tmpDir, "cache")

	st, err := sqlite.Open(dbPath, nil)
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
	shelfService := service.NewShelfService(st, sseManager, logger)
	inboxService := service.NewInboxService(st, enricher, sseManager, logger)
	settingsService := service.NewSettingsService(st, inboxService, logger)
	absImportService := service.NewABSImportService(st, logger)

	// Transcode service with workers disabled (no FFmpeg needed for unit tests).
	transcodeService, err := service.NewTranscodeService(st, sseManager, config.TranscodeConfig{
		Enabled:       false,
		CachePath:     cachePath,
		MaxConcurrent: 1,
	}, logger)
	require.NoError(t, err)

	services := &Services{
		Instance:  instanceService,
		Auth:      authService,
		Sync:      syncService,
		Shelf:     shelfService,
		Settings:  settingsService,
		Inbox:     inboxService,
		ABSImport: absImportService,
		Transcode: transcodeService,
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
		authRateLimiter: NewRateLimiter(100, time.Minute, 50),
	}

	s.registerHealthRoutes()
	s.registerInstanceRoutes()
	s.registerAuthRoutes()
	s.registerTranscodeRoutes()

	_, err = services.Instance.InitializeInstance(context.Background())
	require.NoError(t, err)

	testAPI := humatest.Wrap(t, api)

	t.Cleanup(func() {
		_ = st.Close()
		_ = os.RemoveAll(tmpDir)
	})

	return &transcodeTestServer{
		Server:       s,
		api:          testAPI,
		tokenService: tokenService,
	}
}

// createTranscodeTestUser creates a user and returns the access token.
func (ts *transcodeTestServer) createTranscodeTestUser(t *testing.T) string {
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

	return envelope.Data.AccessToken
}

// seedTranscodeJob inserts a transcode job directly into the store for testing.
func (ts *transcodeTestServer) seedTranscodeJob(t *testing.T, status domain.TranscodeStatus) string {
	t.Helper()
	ctx := context.Background()

	bookID, err := id.Generate("bk")
	require.NoError(t, err)
	audioFileID, err := id.Generate("af")
	require.NoError(t, err)
	jobID, err := id.Generate("tj")
	require.NoError(t, err)

	// Books table has FK constraints; insert a minimal book first.
	now := time.Now()
	book := &domain.Book{
		Syncable: domain.Syncable{
			ID:        bookID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title:         "Test Book",
		Path:          "/test/" + bookID,
		TotalDuration: 300000,
		TotalSize:     1024000,
		ScannedAt:     now,
		AudioFiles: []domain.AudioFileInfo{
			{
				ID:       audioFileID,
				Path:     "/test/source.m4a",
				Filename: "source.m4a",
				Duration: 300000,
				Size:     1024000,
			},
		},
	}
	require.NoError(t, ts.store.CreateBook(ctx, book))

	job := &domain.TranscodeJob{
		ID:          jobID,
		BookID:      bookID,
		AudioFileID: audioFileID,
		SourcePath:  "/test/source.m4a",
		SourceCodec: "eac3",
		SourceHash:  "test-hash",
		OutputCodec: "aac",
		Variant:     domain.TranscodeVariantStereo,
		Status:      status,
		Priority:    1,
		CreatedAt:   now,
	}

	if status == domain.TranscodeStatusRunning {
		t := now
		job.StartedAt = &t
	}
	if status == domain.TranscodeStatusCompleted {
		t1 := now
		t2 := now
		job.StartedAt = &t1
		job.CompletedAt = &t2
		job.OutputPath = "/test/output"
		job.OutputSize = 1024
		job.Progress = 100
	}
	if status == domain.TranscodeStatusCancelled {
		t1 := now
		t2 := now
		job.StartedAt = &t1
		job.CompletedAt = &t2
	}

	require.NoError(t, ts.store.CreateTranscodeJob(ctx, job))
	return jobID
}

// === Tests ===

func TestCancelTranscode_ActivePendingJob(t *testing.T) {
	t.Parallel()
	ts := setupTranscodeAPITestServer(t)
	token := ts.createTranscodeTestUser(t)

	jobID := ts.seedTranscodeJob(t, domain.TranscodeStatusPending)

	resp := ts.api.Post("/api/v1/transcode/cancel/"+jobID, struct{}{},
		"Authorization: Bearer "+token)

	assert.Equal(t, http.StatusNoContent, resp.Code, resp.Body.String())
}

func TestCancelTranscode_ActiveRunningJob(t *testing.T) {
	t.Parallel()
	ts := setupTranscodeAPITestServer(t)
	token := ts.createTranscodeTestUser(t)

	jobID := ts.seedTranscodeJob(t, domain.TranscodeStatusRunning)

	resp := ts.api.Post("/api/v1/transcode/cancel/"+jobID, struct{}{},
		"Authorization: Bearer "+token)

	assert.Equal(t, http.StatusNoContent, resp.Code, resp.Body.String())
}

func TestCancelTranscode_NonExistentJob(t *testing.T) {
	t.Parallel()
	ts := setupTranscodeAPITestServer(t)
	token := ts.createTranscodeTestUser(t)

	resp := ts.api.Post("/api/v1/transcode/cancel/tj_doesnotexist", struct{}{},
		"Authorization: Bearer "+token)

	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func TestCancelTranscode_AlreadyCancelled(t *testing.T) {
	t.Parallel()
	ts := setupTranscodeAPITestServer(t)
	token := ts.createTranscodeTestUser(t)

	jobID := ts.seedTranscodeJob(t, domain.TranscodeStatusCancelled)

	// Idempotent: second cancel returns 404.
	resp := ts.api.Post("/api/v1/transcode/cancel/"+jobID, struct{}{},
		"Authorization: Bearer "+token)

	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func TestCancelTranscode_AlreadyCompleted(t *testing.T) {
	t.Parallel()
	ts := setupTranscodeAPITestServer(t)
	token := ts.createTranscodeTestUser(t)

	jobID := ts.seedTranscodeJob(t, domain.TranscodeStatusCompleted)

	resp := ts.api.Post("/api/v1/transcode/cancel/"+jobID, struct{}{},
		"Authorization: Bearer "+token)

	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func TestCancelTranscode_Unauthenticated(t *testing.T) {
	t.Parallel()
	ts := setupTranscodeAPITestServer(t)

	// No Authorization header — expect 401.
	resp := ts.api.Post("/api/v1/transcode/cancel/tj_any", struct{}{})

	assert.Equal(t, http.StatusUnauthorized, resp.Code)
}
