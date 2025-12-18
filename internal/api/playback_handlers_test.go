package api

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/config"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/listenupapp/listenup-server/internal/service"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupPlaybackTestServer creates a minimal test server for playback testing.
func setupPlaybackTestServer(t *testing.T) (*Server, *store.Store, func()) {
	t.Helper()

	// Create temp directory for test database.
	tmpDir, err := os.MkdirTemp("", "listenup-playback-test-*")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Create a no-op logger for tests.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create SSE manager.
	sseManager := sse.NewManager(logger)

	// Create store.
	s, err := store.New(dbPath, logger, sseManager)
	require.NoError(t, err)

	// Create transcode cache directory
	transCacheDir := filepath.Join(tmpDir, "transcode_cache")
	err = os.MkdirAll(transCacheDir, 0755)
	require.NoError(t, err)

	// Create transcode service
	transcodeService, err := service.NewTranscodeService(
		s,
		sseManager,
		config.TranscodeConfig{
			Enabled:       true,
			CachePath:     transCacheDir,
			MaxConcurrent: 1,
			FFmpegPath:    "", // Will auto-detect
		},
		logger,
	)
	require.NoError(t, err)

	// Create minimal services needed for playback.
	bookService := service.NewBookService(s, nil, logger)

	// Create server.
	sseHandler := sse.NewHandler(sseManager, logger)
	server := NewServer(s, &Services{
		Book:      bookService,
		Transcode: transcodeService,
	}, &StorageServices{}, sseHandler, logger)

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return server, s, cleanup
}

// createTestBookForPlayback creates a test book in the store with the given audio file.
// Creates a temporary file on disk for the audio file so transcode service can hash it.
func createTestBookForPlayback(t *testing.T, s *store.Store, bookID, audioFileID, codec string) *domain.Book {
	t.Helper()

	// Create a temporary file for the audio
	tmpFile, err := os.CreateTemp("", "test-audio-*.m4b")
	require.NoError(t, err)
	tmpFile.WriteString("dummy audio content for testing")
	tmpFile.Close()
	t.Cleanup(func() {
		os.Remove(tmpFile.Name())
	})

	now := time.Now()
	book := &domain.Book{
		Syncable: domain.Syncable{
			ID:        bookID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title: "Test Book",
		Path:  "/test/path",
		AudioFiles: []domain.AudioFileInfo{
			{
				ID:       audioFileID,
				Path:     tmpFile.Name(),
				Filename: filepath.Base(tmpFile.Name()),
				Format:   "m4b",
				Codec:    codec,
				Size:     1024,
				Duration: 3600000, // 1 hour in ms
				Bitrate:  128000,
			},
		},
	}

	ctx := context.Background()
	err = s.CreateBook(ctx, book)
	require.NoError(t, err)

	return book
}

// extractResponse extracts the PreparePlaybackResponse from the http response envelope.
func extractResponse(t *testing.T, w *httptest.ResponseRecorder) PreparePlaybackResponse {
	t.Helper()

	var envelope response.Envelope
	err := json.Unmarshal(w.Body.Bytes(), &envelope)
	require.NoError(t, err, "Failed to unmarshal envelope")

	if !envelope.Success {
		t.Logf("Response error: %s, Message: %s, Body: %s", envelope.Error, envelope.Message, w.Body.String())
	}
	assert.True(t, envelope.Success, "Response should be successful")

	// Extract PreparePlaybackResponse from envelope.Data
	respData, err := json.Marshal(envelope.Data)
	require.NoError(t, err, "Failed to marshal envelope data")

	var resp PreparePlaybackResponse
	err = json.Unmarshal(respData, &resp)
	require.NoError(t, err, "Failed to unmarshal response data")

	return resp
}

// TestPreparePlayback_SourceDoesNotNeedTranscode tests that original file is served
// when the source codec doesn't need transcoding (e.g., AAC).
func TestPreparePlayback_SourceDoesNotNeedTranscode(t *testing.T) {
	server, s, cleanup := setupPlaybackTestServer(t)
	defer cleanup()

	bookID := "book-1"
	audioFileID := "af-123"

	// Create book with AAC codec (doesn't need transcode)
	createTestBookForPlayback(t, s, bookID, audioFileID, "aac")

	// Test with spatial=true - should still serve original
	reqBody := PreparePlaybackRequest{
		BookID:       bookID,
		AudioFileID:  audioFileID,
		Capabilities: []string{"aac"},
		Spatial:      true,
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/playback/prepare", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "test-user"))
	w := httptest.NewRecorder()

	server.handlePreparePlayback(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := extractResponse(t, w)

	assert.True(t, resp.Ready)
	assert.Equal(t, "original", resp.Variant)
	assert.Equal(t, "aac", resp.Codec)
	assert.Contains(t, resp.StreamURL, "/api/v1/books/"+bookID+"/audio/"+audioFileID)
}

// TestPreparePlayback_SpatialOn_ClientSupportsAC4 tests that original AC-4 is served
// when client wants spatial audio AND supports AC-4 natively.
func TestPreparePlayback_SpatialOn_ClientSupportsAC4(t *testing.T) {
	server, s, cleanup := setupPlaybackTestServer(t)
	defer cleanup()

	bookID := "book-1"
	audioFileID := "af-123"

	// Create book with AC-4 codec (Dolby Atmos)
	createTestBookForPlayback(t, s, bookID, audioFileID, "ac4")

	// Test with spatial=true and AC-4 capability (Samsung/Apple device)
	reqBody := PreparePlaybackRequest{
		BookID:       bookID,
		AudioFileID:  audioFileID,
		Capabilities: []string{"aac", "ac4"},
		Spatial:      true,
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/playback/prepare", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "test-user"))
	w := httptest.NewRecorder()

	server.handlePreparePlayback(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := extractResponse(t, w)

	assert.True(t, resp.Ready)
	assert.Equal(t, "original", resp.Variant)
	assert.Equal(t, "ac4", resp.Codec)
	assert.Contains(t, resp.StreamURL, "/api/v1/books/"+bookID+"/audio/"+audioFileID)
}

// TestPreparePlayback_SpatialOn_ClientDoesNotSupportAC4 tests that 5.1 AAC transcode
// is created when client wants spatial audio but doesn't support AC-4.
func TestPreparePlayback_SpatialOn_ClientDoesNotSupportAC4(t *testing.T) {
	server, s, cleanup := setupPlaybackTestServer(t)
	defer cleanup()

	bookID := "book-1"
	audioFileID := "af-123"

	// Create book with AC-4 codec
	createTestBookForPlayback(t, s, bookID, audioFileID, "ac4")

	// Test with spatial=true but no AC-4 capability (Pixel device)
	reqBody := PreparePlaybackRequest{
		BookID:       bookID,
		AudioFileID:  audioFileID,
		Capabilities: []string{"aac"},
		Spatial:      true,
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/playback/prepare", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "test-user"))
	w := httptest.NewRecorder()

	server.handlePreparePlayback(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := extractResponse(t, w)

	// Should create transcode job for spatial variant
	assert.False(t, resp.Ready, "Should not be ready yet (transcode job created)")
	assert.Equal(t, "transcoded", resp.Variant)
	assert.Equal(t, "aac", resp.Codec)
	assert.NotEmpty(t, resp.TranscodeJobID, "Should have transcode job ID")

	// Verify the job was created with the correct variant
	ctx := context.Background()
	job, err := s.GetTranscodeJob(ctx, resp.TranscodeJobID)
	require.NoError(t, err)
	assert.Equal(t, domain.TranscodeVariantSpatial, job.Variant, "Job should be for spatial variant")
}

// TestPreparePlayback_SpatialOff tests that stereo transcode is created
// when client doesn't want spatial audio.
func TestPreparePlayback_SpatialOff(t *testing.T) {
	server, s, cleanup := setupPlaybackTestServer(t)
	defer cleanup()

	bookID := "book-1"
	audioFileID := "af-123"

	// Create book with AC-4 codec
	createTestBookForPlayback(t, s, bookID, audioFileID, "ac4")

	// Test with spatial=false
	reqBody := PreparePlaybackRequest{
		BookID:       bookID,
		AudioFileID:  audioFileID,
		Capabilities: []string{"aac", "ac4"},
		Spatial:      false,
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/playback/prepare", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "test-user"))
	w := httptest.NewRecorder()

	server.handlePreparePlayback(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := extractResponse(t, w)

	// Should create transcode job for stereo variant
	assert.False(t, resp.Ready, "Should not be ready yet (transcode job created)")
	assert.Equal(t, "transcoded", resp.Variant)
	assert.Equal(t, "aac", resp.Codec)
	assert.NotEmpty(t, resp.TranscodeJobID, "Should have transcode job ID")

	// Verify the job was created with the correct variant
	ctx := context.Background()
	job, err := s.GetTranscodeJob(ctx, resp.TranscodeJobID)
	require.NoError(t, err)
	assert.Equal(t, domain.TranscodeVariantStereo, job.Variant, "Job should be for stereo variant")
}

// TestPreparePlayback_VariantIsolation tests that different variants are treated independently.
func TestPreparePlayback_VariantIsolation(t *testing.T) {
	server, s, cleanup := setupPlaybackTestServer(t)
	defer cleanup()

	bookID := "book-1"
	audioFileID := "af-123"

	// Create book with AC-4 codec
	createTestBookForPlayback(t, s, bookID, audioFileID, "ac4")

	// Test 1: Request spatial=true - should create spatial job
	reqBody1 := PreparePlaybackRequest{
		BookID:       bookID,
		AudioFileID:  audioFileID,
		Capabilities: []string{"aac"},
		Spatial:      true,
	}

	body1, err := json.Marshal(reqBody1)
	require.NoError(t, err)

	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/playback/prepare", bytes.NewReader(body1))
	req1 = req1.WithContext(context.WithValue(req1.Context(), contextKeyUserID, "test-user"))
	w1 := httptest.NewRecorder()

	server.handlePreparePlayback(w1, req1)

	assert.Equal(t, http.StatusOK, w1.Code)

	resp1 := extractResponse(t, w1)

	assert.False(t, resp1.Ready)
	assert.NotEmpty(t, resp1.TranscodeJobID)

	// Verify spatial job was created
	ctx := context.Background()
	job1, err := s.GetTranscodeJob(ctx, resp1.TranscodeJobID)
	require.NoError(t, err)
	assert.Equal(t, domain.TranscodeVariantSpatial, job1.Variant)

	// Test 2: Request spatial=false - should create separate stereo job
	reqBody2 := PreparePlaybackRequest{
		BookID:       bookID,
		AudioFileID:  audioFileID,
		Capabilities: []string{"aac"},
		Spatial:      false,
	}

	body2, err := json.Marshal(reqBody2)
	require.NoError(t, err)

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/playback/prepare", bytes.NewReader(body2))
	req2 = req2.WithContext(context.WithValue(req2.Context(), contextKeyUserID, "test-user"))
	w2 := httptest.NewRecorder()

	server.handlePreparePlayback(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)

	resp2 := extractResponse(t, w2)

	assert.False(t, resp2.Ready)
	assert.NotEmpty(t, resp2.TranscodeJobID)

	// Verify stereo job was created and it's different from spatial job
	job2, err := s.GetTranscodeJob(ctx, resp2.TranscodeJobID)
	require.NoError(t, err)
	assert.Equal(t, domain.TranscodeVariantStereo, job2.Variant)
	assert.NotEqual(t, resp1.TranscodeJobID, resp2.TranscodeJobID, "Should create separate jobs for different variants")
}

// TestPreparePlayback_TranscodeDecisionMatrix tests the complete decision matrix.
func TestPreparePlayback_TranscodeDecisionMatrix(t *testing.T) {
	tests := []struct {
		name             string
		sourceCodec      string
		spatial          bool
		capabilities     []string
		expectedVariant  string
		expectedCodec    string
		shouldTranscode  bool
		transcodeVariant domain.TranscodeVariant
	}{
		{
			name:            "AAC source, spatial on",
			sourceCodec:     "aac",
			spatial:         true,
			capabilities:    []string{"aac"},
			expectedVariant: "original",
			expectedCodec:   "aac",
			shouldTranscode: false,
		},
		{
			name:            "AAC source, spatial off",
			sourceCodec:     "aac",
			spatial:         false,
			capabilities:    []string{"aac"},
			expectedVariant: "original",
			expectedCodec:   "aac",
			shouldTranscode: false,
		},
		{
			name:             "AC4 source, spatial on, client supports AC4",
			sourceCodec:      "ac4",
			spatial:          true,
			capabilities:     []string{"aac", "ac4"},
			expectedVariant:  "original",
			expectedCodec:    "ac4",
			shouldTranscode:  false,
		},
		{
			name:             "AC4 source, spatial on, client doesn't support AC4",
			sourceCodec:      "ac4",
			spatial:          true,
			capabilities:     []string{"aac"},
			expectedVariant:  "transcoded",
			expectedCodec:    "aac",
			shouldTranscode:  true,
			transcodeVariant: domain.TranscodeVariantSpatial,
		},
		{
			name:             "AC4 source, spatial off, client supports AC4",
			sourceCodec:      "ac4",
			spatial:          false,
			capabilities:     []string{"aac", "ac4"},
			expectedVariant:  "transcoded",
			expectedCodec:    "aac",
			shouldTranscode:  true,
			transcodeVariant: domain.TranscodeVariantStereo,
		},
		{
			name:             "AC4 source, spatial off, client doesn't support AC4",
			sourceCodec:      "ac4",
			spatial:          false,
			capabilities:     []string{"aac"},
			expectedVariant:  "transcoded",
			expectedCodec:    "aac",
			shouldTranscode:  true,
			transcodeVariant: domain.TranscodeVariantStereo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, s, cleanup := setupPlaybackTestServer(t)
			defer cleanup()

			bookID, _ := id.Generate("book")
			audioFileID, _ := id.Generate("af")

			// Create book with the specified codec
			createTestBookForPlayback(t, s, bookID, audioFileID, tt.sourceCodec)

			// Make the request
			reqBody := PreparePlaybackRequest{
				BookID:       bookID,
				AudioFileID:  audioFileID,
				Capabilities: tt.capabilities,
				Spatial:      tt.spatial,
			}

			body, err := json.Marshal(reqBody)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/playback/prepare", bytes.NewReader(body))
			req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "test-user"))
			w := httptest.NewRecorder()

			server.handlePreparePlayback(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			resp := extractResponse(t, w)

			assert.Equal(t, tt.expectedVariant, resp.Variant, "Variant mismatch")
			assert.Equal(t, tt.expectedCodec, resp.Codec, "Codec mismatch")

			if tt.shouldTranscode {
				assert.False(t, resp.Ready, "Should not be ready (transcode job created)")
				assert.NotEmpty(t, resp.TranscodeJobID, "Should have transcode job ID")

				// Verify the transcode variant
				ctx := context.Background()
				job, err := s.GetTranscodeJob(ctx, resp.TranscodeJobID)
				require.NoError(t, err)
				assert.Equal(t, tt.transcodeVariant, job.Variant, "Transcode variant mismatch")
			} else {
				assert.True(t, resp.Ready, "Should be ready (serving original)")
			}
		})
	}
}
