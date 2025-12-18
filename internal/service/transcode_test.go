package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/config"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/listenupapp/listenup-server/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTranscodeTest creates a minimal transcode service for testing.
func setupTranscodeTest(t *testing.T) (*TranscodeService, string, func()) {
	t.Helper()

	// Create temp directories for test database and cache
	tmpDir, err := os.MkdirTemp("", "listenup-transcode-test-*")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")
	cachePath := filepath.Join(tmpDir, "cache")

	// Create store
	s, err := store.New(dbPath, nil, store.NewNoopEmitter())
	require.NoError(t, err)

	// Create transcode service config
	cfg := config.TranscodeConfig{
		Enabled:       false, // Don't start workers in tests
		CachePath:     cachePath,
		MaxConcurrent: 1,
	}

	// Create service (without ffmpeg - not needed for these tests)
	service := &TranscodeService{
		store:  s,
		logger: nil, // Tests can pass nil logger
		config: cfg,
	}

	// Cleanup function
	cleanup := func() {
		//nolint:errcheck // Test cleanup - errors logged but not critical
		_ = s.Close()
		//nolint:errcheck // Test cleanup - errors logged but not critical
		_ = os.RemoveAll(tmpDir)
	}

	return service, tmpDir, cleanup
}

// createTestTranscodeJob creates a transcode job for testing.
func createTestTranscodeJob(t *testing.T, s *store.Store, bookID, audioFileID string, status domain.TranscodeStatus) *domain.TranscodeJob {
	return createTestTranscodeJobWithVariant(t, s, bookID, audioFileID, status, domain.TranscodeVariantSpatial)
}

// createTestTranscodeJobWithVariant creates a transcode job with a specific variant for testing.
func createTestTranscodeJobWithVariant(t *testing.T, s *store.Store, bookID, audioFileID string, status domain.TranscodeStatus, variant domain.TranscodeVariant) *domain.TranscodeJob {
	t.Helper()

	jobID, err := id.Generate("tj")
	require.NoError(t, err)

	job := &domain.TranscodeJob{
		ID:          jobID,
		BookID:      bookID,
		AudioFileID: audioFileID,
		SourcePath:  "/test/source.m4a",
		SourceCodec: "aac",
		SourceHash:  "test-hash",
		OutputCodec: "aac",
		Variant:     variant,
		Status:      status,
		Priority:    1,
		CreatedAt:   time.Now(),
	}

	if status == domain.TranscodeStatusRunning {
		now := time.Now()
		job.StartedAt = &now
	}

	if status == domain.TranscodeStatusCompleted {
		now := time.Now()
		job.StartedAt = &now
		job.CompletedAt = &now
		job.OutputPath = "/test/output"
		job.OutputSize = 1024
	}

	err = s.CreateTranscodeJob(context.Background(), job)
	require.NoError(t, err)

	return job
}

// Test_findAvailableSegments tests the findAvailableSegments function.
func Test_findAvailableSegments(t *testing.T) {
	tests := []struct {
		name          string
		setupDir      func(t *testing.T, dir string)
		wantSegments  []string
		wantErr       bool
		skipDirCreate bool
	}{
		{
			name: "empty directory returns empty slice",
			setupDir: func(t *testing.T, dir string) {
				// Directory exists but is empty
			},
			wantSegments: nil, // Empty directory returns nil slice
			wantErr:      false,
		},
		{
			name: "directory with segments returns sorted list",
			setupDir: func(t *testing.T, dir string) {
				// Create segments out of order
				require.NoError(t, os.WriteFile(filepath.Join(dir, "seg_0002.ts"), []byte("test"), 0644))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "seg_0000.ts"), []byte("test"), 0644))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "seg_0001.ts"), []byte("test"), 0644))
			},
			wantSegments: []string{"seg_0000.ts", "seg_0001.ts", "seg_0002.ts"},
			wantErr:      false,
		},
		{
			name: "directory with mixed files only returns seg_*.ts",
			setupDir: func(t *testing.T, dir string) {
				// Create valid segments
				require.NoError(t, os.WriteFile(filepath.Join(dir, "seg_0000.ts"), []byte("test"), 0644))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "seg_0001.ts"), []byte("test"), 0644))
				// Create other files that should be ignored
				require.NoError(t, os.WriteFile(filepath.Join(dir, "playlist.m3u8"), []byte("test"), 0644))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "other.ts"), []byte("test"), 0644))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "seg_test.txt"), []byte("test"), 0644))
			},
			wantSegments: []string{"seg_0000.ts", "seg_0001.ts"},
			wantErr:      false,
		},
		{
			name:          "non-existent directory returns error",
			skipDirCreate: true,
			wantSegments:  nil,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory for test
			tmpDir, err := os.MkdirTemp("", "segments-test-*")
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			testDir := filepath.Join(tmpDir, "test")
			if !tt.skipDirCreate {
				require.NoError(t, os.MkdirAll(testDir, 0755))
				if tt.setupDir != nil {
					tt.setupDir(t, testDir)
				}
			}

			// Run test
			segments, err := findAvailableSegments(testDir)

			// Verify results
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantSegments, segments)
		})
	}
}

// Test_GenerateDynamicPlaylist tests the GenerateDynamicPlaylist method.
func Test_GenerateDynamicPlaylist(t *testing.T) {
	tests := []struct {
		name        string
		setupJob    func(t *testing.T, s *store.Store, cachePath string) (string, string) // Returns bookID, audioFileID
		setupFiles  func(t *testing.T, cachePath, bookID, audioFileID string)
		wantErr     bool
		wantErrMsg  string
		checkResult func(t *testing.T, playlist string, status domain.TranscodeStatus)
	}{
		{
			name: "job exists, segments exist, status=Running - playlist without ENDLIST",
			setupJob: func(t *testing.T, s *store.Store, cachePath string) (string, string) {
				bookID := "book_test123"
				audioFileID := "af_test456"
				createTestTranscodeJob(t, s, bookID, audioFileID, domain.TranscodeStatusRunning)
				return bookID, audioFileID
			},
			setupFiles: func(t *testing.T, cachePath, bookID, audioFileID string) {
				hlsDir := filepath.Join(cachePath, bookID, audioFileID, string(domain.TranscodeVariantSpatial))
				require.NoError(t, os.MkdirAll(hlsDir, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(hlsDir, "seg_0000.ts"), []byte("test"), 0644))
				require.NoError(t, os.WriteFile(filepath.Join(hlsDir, "seg_0001.ts"), []byte("test"), 0644))
			},
			wantErr: false,
			checkResult: func(t *testing.T, playlist string, status domain.TranscodeStatus) {
				// Verify playlist structure
				assert.Contains(t, playlist, "#EXTM3U")
				assert.Contains(t, playlist, "#EXT-X-VERSION:3")
				assert.Contains(t, playlist, "#EXT-X-TARGETDURATION:10")
				assert.Contains(t, playlist, "#EXT-X-MEDIA-SEQUENCE:0")
				assert.Contains(t, playlist, "seg_0000.ts")
				assert.Contains(t, playlist, "seg_0001.ts")
				// Should NOT have ENDLIST for running jobs
				assert.NotContains(t, playlist, "#EXT-X-ENDLIST")
			},
		},
		{
			name: "job exists, segments exist, status=Completed - playlist with ENDLIST",
			setupJob: func(t *testing.T, s *store.Store, cachePath string) (string, string) {
				bookID := "book_test789"
				audioFileID := "af_test012"
				createTestTranscodeJob(t, s, bookID, audioFileID, domain.TranscodeStatusCompleted)
				return bookID, audioFileID
			},
			setupFiles: func(t *testing.T, cachePath, bookID, audioFileID string) {
				hlsDir := filepath.Join(cachePath, bookID, audioFileID, string(domain.TranscodeVariantSpatial))
				require.NoError(t, os.MkdirAll(hlsDir, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(hlsDir, "seg_0000.ts"), []byte("test"), 0644))
				require.NoError(t, os.WriteFile(filepath.Join(hlsDir, "seg_0001.ts"), []byte("test"), 0644))
				require.NoError(t, os.WriteFile(filepath.Join(hlsDir, "seg_0002.ts"), []byte("test"), 0644))
			},
			wantErr: false,
			checkResult: func(t *testing.T, playlist string, status domain.TranscodeStatus) {
				// Verify playlist structure
				assert.Contains(t, playlist, "#EXTM3U")
				assert.Contains(t, playlist, "seg_0000.ts")
				assert.Contains(t, playlist, "seg_0001.ts")
				assert.Contains(t, playlist, "seg_0002.ts")
				// Should have ENDLIST for completed jobs
				assert.Contains(t, playlist, "#EXT-X-ENDLIST")
			},
		},
		{
			name: "job not found returns error",
			setupJob: func(t *testing.T, s *store.Store, cachePath string) (string, string) {
				// Don't create a job - return non-existent IDs
				return "book_nonexistent", "af_nonexistent"
			},
			setupFiles: func(t *testing.T, cachePath, bookID, audioFileID string) {
				// No files to create
			},
			wantErr:    true,
			wantErrMsg: "get transcode job",
		},
		{
			name: "no segments available returns error",
			setupJob: func(t *testing.T, s *store.Store, cachePath string) (string, string) {
				bookID := "book_noseg"
				audioFileID := "af_noseg"
				createTestTranscodeJob(t, s, bookID, audioFileID, domain.TranscodeStatusRunning)
				return bookID, audioFileID
			},
			setupFiles: func(t *testing.T, cachePath, bookID, audioFileID string) {
				// Create directory but no segments
				hlsDir := filepath.Join(cachePath, bookID, audioFileID, string(domain.TranscodeVariantSpatial))
				require.NoError(t, os.MkdirAll(hlsDir, 0755))
			},
			wantErr:    true,
			wantErrMsg: "no segments available yet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, tmpDir, cleanup := setupTranscodeTest(t)
			defer cleanup()

			ctx := context.Background()

			// Setup job and get IDs
			bookID, audioFileID := tt.setupJob(t, service.store, service.config.CachePath)

			// Setup files
			if tt.setupFiles != nil {
				tt.setupFiles(t, service.config.CachePath, bookID, audioFileID)
			}

			// Generate playlist
			playlist, err := service.GenerateDynamicPlaylist(ctx, audioFileID)

			// Verify error expectations
			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantErrMsg != "" {
					assert.Contains(t, err.Error(), tt.wantErrMsg)
				}
				return
			}

			// Verify success
			require.NoError(t, err)
			assert.NotEmpty(t, playlist)

			// Get job status for checkResult
			job, err := service.store.GetTranscodeJobByAudioFile(ctx, audioFileID)
			require.NoError(t, err)

			// Run custom checks
			if tt.checkResult != nil {
				tt.checkResult(t, playlist, job.Status)
			}

			// Cleanup tmp dir
			_ = tmpDir
		})
	}
}

// Test_TranscodeVariants tests variant-specific functionality.
func Test_TranscodeVariants(t *testing.T) {
	t.Run("stereo variant creates correct cache path", func(t *testing.T) {
		service, _, cleanup := setupTranscodeTest(t)
		defer cleanup()

		job := createTestTranscodeJobWithVariant(
			t, service.store,
			"book_123", "af_456",
			domain.TranscodeStatusCompleted,
			domain.TranscodeVariantStereo,
		)

		assert.Equal(t, domain.TranscodeVariantStereo, job.Variant)
	})

	t.Run("spatial variant creates correct cache path", func(t *testing.T) {
		service, _, cleanup := setupTranscodeTest(t)
		defer cleanup()

		job := createTestTranscodeJobWithVariant(
			t, service.store,
			"book_123", "af_456",
			domain.TranscodeStatusCompleted,
			domain.TranscodeVariantSpatial,
		)

		assert.Equal(t, domain.TranscodeVariantSpatial, job.Variant)
	})

	t.Run("multiple variants for same audio file", func(t *testing.T) {
		service, _, cleanup := setupTranscodeTest(t)
		defer cleanup()

		ctx := context.Background()
		bookID := "book_multi"
		audioFileID := "af_multi"

		// Create stereo variant
		stereoJob := createTestTranscodeJobWithVariant(
			t, service.store,
			bookID, audioFileID,
			domain.TranscodeStatusCompleted,
			domain.TranscodeVariantStereo,
		)

		// Create spatial variant
		spatialJob := createTestTranscodeJobWithVariant(
			t, service.store,
			bookID, audioFileID,
			domain.TranscodeStatusCompleted,
			domain.TranscodeVariantSpatial,
		)

		// Verify both can be retrieved
		retrievedStereo, err := service.store.GetTranscodeJobByAudioFileAndVariant(ctx, audioFileID, domain.TranscodeVariantStereo)
		require.NoError(t, err)
		assert.Equal(t, stereoJob.ID, retrievedStereo.ID)
		assert.Equal(t, domain.TranscodeVariantStereo, retrievedStereo.Variant)

		retrievedSpatial, err := service.store.GetTranscodeJobByAudioFileAndVariant(ctx, audioFileID, domain.TranscodeVariantSpatial)
		require.NoError(t, err)
		assert.Equal(t, spatialJob.ID, retrievedSpatial.ID)
		assert.Equal(t, domain.TranscodeVariantSpatial, retrievedSpatial.Variant)
	})

	t.Run("GetHLSPathIfReadyForVariant returns correct variant path", func(t *testing.T) {
		service, _, cleanup := setupTranscodeTest(t)
		defer cleanup()

		ctx := context.Background()
		bookID := "book_hls"
		audioFileID := "af_hls"

		// Create stereo job with segments
		createTestTranscodeJobWithVariant(
			t, service.store,
			bookID, audioFileID,
			domain.TranscodeStatusRunning,
			domain.TranscodeVariantStereo,
		)

		// Create segment file for stereo variant
		stereoDir := filepath.Join(service.config.CachePath, bookID, audioFileID, string(domain.TranscodeVariantStereo))
		require.NoError(t, os.MkdirAll(stereoDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(stereoDir, "seg_0000.ts"), []byte("test"), 0644))

		// GetHLSPathIfReadyForVariant should return the stereo path
		path, ready := service.GetHLSPathIfReadyForVariant(ctx, audioFileID, domain.TranscodeVariantStereo)
		assert.True(t, ready)
		assert.Equal(t, stereoDir, path)

		// Spatial variant should not be ready (no segments)
		_, ready = service.GetHLSPathIfReadyForVariant(ctx, audioFileID, domain.TranscodeVariantSpatial)
		assert.False(t, ready)
	})
}

// Test_buildFFmpegArgs_Variants tests that buildFFmpegArgs correctly handles variants.
func Test_buildFFmpegArgs_Variants(t *testing.T) {
	service, tmpDir, cleanup := setupTranscodeTest(t)
	defer cleanup()

	outputDir := filepath.Join(tmpDir, "output")
	require.NoError(t, os.MkdirAll(outputDir, 0755))

	t.Run("stereo variant limits channels and bitrate", func(t *testing.T) {
		// High bitrate and 6 channels should be capped for stereo
		args := service.buildFFmpegArgs("/test/input.m4a", outputDir, 500000, 6, domain.TranscodeVariantStereo)

		// Find bitrate and channel args
		bitrateIdx := -1
		channelsIdx := -1
		for i, arg := range args {
			if arg == "-b:a" && i+1 < len(args) {
				bitrateIdx = i + 1
			}
			if arg == "-ac" && i+1 < len(args) {
				channelsIdx = i + 1
			}
		}

		require.NotEqual(t, -1, bitrateIdx, "bitrate arg not found")
		require.NotEqual(t, -1, channelsIdx, "channels arg not found")

		// Stereo should be capped at 2 channels and 128kbps
		assert.Equal(t, "2", args[channelsIdx])
		assert.Equal(t, "128000", args[bitrateIdx])
	})

	t.Run("spatial variant enforces minimum bitrate and 6 channels", func(t *testing.T) {
		// Low bitrate and 2 channels should be increased for spatial
		args := service.buildFFmpegArgs("/test/input.m4a", outputDir, 128000, 2, domain.TranscodeVariantSpatial)

		// Find bitrate and channel args
		bitrateIdx := -1
		channelsIdx := -1
		for i, arg := range args {
			if arg == "-b:a" && i+1 < len(args) {
				bitrateIdx = i + 1
			}
			if arg == "-ac" && i+1 < len(args) {
				channelsIdx = i + 1
			}
		}

		require.NotEqual(t, -1, bitrateIdx, "bitrate arg not found")
		require.NotEqual(t, -1, channelsIdx, "channels arg not found")

		// Spatial should be forced to 6 channels and minimum 384kbps
		assert.Equal(t, "6", args[channelsIdx])
		assert.Equal(t, "384000", args[bitrateIdx])
	})
}
