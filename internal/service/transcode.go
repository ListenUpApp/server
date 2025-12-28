package service

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/listenupapp/listenup-server/internal/config"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
)

// TranscodeService manages audio transcoding operations.
type TranscodeService struct {
	store      *store.Store
	emitter    *sse.Manager
	logger     *slog.Logger
	config     config.TranscodeConfig
	ffmpegPath string

	// Worker management
	ctx       context.Context //nolint:containedctx // Context needed for worker lifecycle management
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	jobNotify chan struct{} // Signal that new jobs are available
}

// NewTranscodeService creates a new transcode service.
func NewTranscodeService(
	store *store.Store,
	emitter *sse.Manager,
	cfg config.TranscodeConfig,
	logger *slog.Logger,
) (*TranscodeService, error) {
	// Find ffmpeg
	ffmpegPath := cfg.FFmpegPath
	if ffmpegPath == "" {
		path, err := exec.LookPath("ffmpeg")
		if err != nil {
			if cfg.Enabled {
				return nil, fmt.Errorf("ffmpeg not found and transcoding is enabled: %w", err)
			}
			logger.Warn("ffmpeg not found, transcoding disabled")
		}
		ffmpegPath = path
	}
	logger.Info("using ffmpeg", slog.String("path", ffmpegPath))

	// Ensure cache directory exists
	if err := os.MkdirAll(cfg.CachePath, 0755); err != nil {
		return nil, fmt.Errorf("create cache directory: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &TranscodeService{
		store:      store,
		emitter:    emitter,
		logger:     logger,
		config:     cfg,
		ffmpegPath: ffmpegPath,
		ctx:        ctx,
		cancel:     cancel,
		jobNotify:  make(chan struct{}, 1),
	}, nil
}

// Start begins the transcode worker pool.
func (s *TranscodeService) Start() {
	if !s.config.Enabled || s.ffmpegPath == "" {
		s.logger.Info("transcoding disabled, not starting workers")
		return
	}

	s.logger.Info("starting transcode workers",
		slog.Int("workers", s.config.MaxConcurrent),
		slog.String("cache_path", s.config.CachePath),
	)

	// Start recovery for any stalled jobs
	s.recoverStalledJobs()

	// Start workers
	for i := range s.config.MaxConcurrent {
		s.wg.Add(1)
		go s.worker(i)
	}
}

// Stop gracefully shuts down the transcode service.
func (s *TranscodeService) Stop() {
	s.logger.Info("stopping transcode service")
	s.cancel()
	s.wg.Wait()
	s.logger.Info("transcode service stopped")
}

// NotifyNewJob signals workers that a new job is available.
func (s *TranscodeService) NotifyNewJob() {
	select {
	case s.jobNotify <- struct{}{}:
	default:
		// Already notified
	}
}

// CreateJob creates a new transcode job for an audio file.
func (s *TranscodeService) CreateJob(
	ctx context.Context,
	bookID, audioFileID, sourcePath, sourceCodec string,
	priority int,
	variant domain.TranscodeVariant,
) (*domain.TranscodeJob, error) {
	// Check if job already exists for this audio file and variant
	existing, err := s.store.GetTranscodeJobByAudioFileAndVariant(ctx, audioFileID, variant)
	if err == nil {
		// Check if source changed (applies to all statuses)
		hash, hashErr := s.hashFile(sourcePath)
		sourceChanged := hashErr == nil && hash != existing.SourceHash

		switch existing.Status {
		case domain.TranscodeStatusCompleted:
			if !sourceChanged {
				// Verify output files still exist
				hlsDir := filepath.Join(s.config.CachePath, existing.BookID, existing.AudioFileID, string(existing.Variant))
				playlistPath := filepath.Join(hlsDir, "playlist.m3u8")
				if _, statErr := os.Stat(playlistPath); statErr == nil {
					// Files exist, transcode is valid
					return existing, nil
				}
				// Files missing - fall through to re-create job
				s.logger.Warn("completed job missing output files, re-transcoding",
					slog.String("job_id", existing.ID),
					slog.String("audio_file_id", audioFileID),
					slog.String("variant", string(existing.Variant)),
				)
			}
			// Source changed or files missing, delete old job and create new
			if err := s.store.DeleteTranscodeJob(ctx, existing.ID); err != nil {
				return nil, fmt.Errorf("delete stale job: %w", err)
			}
			// Delete old output files
			if existing.OutputPath != "" {
				_ = os.RemoveAll(existing.OutputPath)
			}

		case domain.TranscodeStatusPending, domain.TranscodeStatusRunning:
			// Job in progress, maybe bump priority
			if priority > existing.Priority {
				existing.Priority = priority
				if err := s.store.UpdateTranscodeJob(ctx, existing); err != nil {
					return nil, fmt.Errorf("update job priority: %w", err)
				}
			}
			return existing, nil

		case domain.TranscodeStatusFailed:
			// Check if we can now decode the codec (e.g., librempeg was installed).
			// If so, retry instead of returning the stale failure.
			if s.canDecodeCodec(ctx, sourceCodec) {
				s.logger.Info("retrying previously failed job - codec now decodable",
					slog.String("audio_file_id", audioFileID),
					slog.String("codec", sourceCodec))
				if err := s.store.DeleteTranscodeJob(ctx, existing.ID); err != nil {
					return nil, fmt.Errorf("delete failed job for retry: %w", err)
				}
			} else if !sourceChanged {
				// Return failed job so error can be surfaced to client.
				// Don't retry - the failure is likely permanent (e.g., unsupported codec).
				return existing, nil
			} else {
				// Source changed, try again
				if err := s.store.DeleteTranscodeJob(ctx, existing.ID); err != nil {
					return nil, fmt.Errorf("delete failed job: %w", err)
				}
			}
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("check existing job: %w", err)
	}

	// Generate source hash
	sourceHash, err := s.hashFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("hash source file: %w", err)
	}

	// Generate job ID
	jobID, err := id.Generate("tj")
	if err != nil {
		return nil, fmt.Errorf("generate job id: %w", err)
	}

	job := &domain.TranscodeJob{
		ID:          jobID,
		BookID:      bookID,
		AudioFileID: audioFileID,
		SourcePath:  sourcePath,
		SourceCodec: sourceCodec,
		SourceHash:  sourceHash,
		OutputCodec: "aac",
		Variant:     variant,
		Status:      domain.TranscodeStatusPending,
		Priority:    priority,
		CreatedAt:   time.Now(),
	}

	if err := s.store.CreateTranscodeJob(ctx, job); err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}

	s.logger.Info("created transcode job",
		slog.String("job_id", job.ID),
		slog.String("book_id", bookID),
		slog.String("source_codec", sourceCodec),
		slog.Int("priority", priority),
	)

	// Notify workers
	s.NotifyNewJob()

	return job, nil
}

// BumpPriority increases a job's priority for user-requested playback.
func (s *TranscodeService) BumpPriority(ctx context.Context, jobID string) error {
	job, err := s.store.GetTranscodeJob(ctx, jobID)
	if err != nil {
		return err
	}

	job.BumpPriority()

	if err := s.store.UpdateTranscodeJob(ctx, job); err != nil {
		return err
	}

	s.NotifyNewJob()
	return nil
}

// GetTranscodePath returns the path to the HLS directory if transcoding is complete.
func (s *TranscodeService) GetTranscodePath(ctx context.Context, audioFileID string) (string, bool) {
	job, err := s.store.GetTranscodeJobByAudioFile(ctx, audioFileID)
	if err != nil {
		return "", false
	}

	if job.Status != domain.TranscodeStatusCompleted {
		return "", false
	}

	// Verify HLS playlist exists
	playlistPath := filepath.Join(job.OutputPath, "playlist.m3u8")
	if _, err := os.Stat(playlistPath); err != nil {
		return "", false
	}

	return job.OutputPath, true
}

// GetHLSPathIfReady returns the HLS directory path as soon as first segment is available.
// This allows progressive playback while transcoding continues.
// Playlists are generated dynamically from available segments, not read from disk.
func (s *TranscodeService) GetHLSPathIfReady(ctx context.Context, audioFileID string) (string, bool) {
	job, err := s.store.GetTranscodeJobByAudioFile(ctx, audioFileID)
	if err != nil {
		return "", false
	}

	// Job must be running or completed
	if job.Status != domain.TranscodeStatusRunning && job.Status != domain.TranscodeStatusCompleted {
		return "", false
	}

	// Construct the HLS directory path from job metadata.
	// OutputPath is only set on completion, so we build it from BookID/AudioFileID/Variant.
	hlsDir := filepath.Join(s.config.CachePath, job.BookID, job.AudioFileID, string(job.Variant))

	// Check if at least one segment exists
	segmentPath := filepath.Join(hlsDir, "seg_0000.ts")
	if _, err := os.Stat(segmentPath); err != nil {
		return "", false
	}

	return hlsDir, true
}

// GetHLSPathIfReadyForVariant returns the HLS directory path for a specific variant
// as soon as first segment is available. This allows progressive playback while
// transcoding continues.
func (s *TranscodeService) GetHLSPathIfReadyForVariant(ctx context.Context, audioFileID string, variant domain.TranscodeVariant) (string, bool) {
	job, err := s.store.GetTranscodeJobByAudioFileAndVariant(ctx, audioFileID, variant)
	if err != nil {
		return "", false
	}

	// Job must be running or completed
	if job.Status != domain.TranscodeStatusRunning && job.Status != domain.TranscodeStatusCompleted {
		return "", false
	}

	// Construct the HLS directory path from job metadata.
	hlsDir := filepath.Join(s.config.CachePath, job.BookID, job.AudioFileID, string(job.Variant))

	// Check if at least one segment exists
	segmentPath := filepath.Join(hlsDir, "seg_0000.ts")
	if _, err := os.Stat(segmentPath); err != nil {
		return "", false
	}

	return hlsDir, true
}

// findAvailableSegments returns sorted list of completed segment filenames in a directory.
func findAvailableSegments(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var segments []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "seg_") && strings.HasSuffix(name, ".ts") {
			segments = append(segments, name)
		}
	}

	sort.Strings(segments)
	return segments, nil
}

// GenerateDynamicPlaylist builds an HLS playlist from available segments.
// This allows playback to start before transcoding completes.
func (s *TranscodeService) GenerateDynamicPlaylist(ctx context.Context, audioFileID string) (string, error) {
	job, err := s.store.GetTranscodeJobByAudioFile(ctx, audioFileID)
	if err != nil {
		return "", fmt.Errorf("get transcode job: %w", err)
	}

	// Build path to HLS directory
	hlsDir := filepath.Join(s.config.CachePath, job.BookID, job.AudioFileID, string(job.Variant))

	segments, err := findAvailableSegments(hlsDir)
	if err != nil {
		return "", fmt.Errorf("find segments: %w", err)
	}

	if len(segments) == 0 {
		return "", fmt.Errorf("no segments available yet")
	}

	// Build playlist
	var playlist strings.Builder
	playlist.WriteString("#EXTM3U\n")
	playlist.WriteString("#EXT-X-VERSION:3\n")
	playlist.WriteString("#EXT-X-TARGETDURATION:10\n")
	playlist.WriteString("#EXT-X-MEDIA-SEQUENCE:0\n")

	for _, seg := range segments {
		playlist.WriteString("#EXTINF:10.0,\n")
		playlist.WriteString(seg)
		playlist.WriteString("\n")
	}

	// Only add ENDLIST when transcode is complete
	if job.Status == domain.TranscodeStatusCompleted {
		playlist.WriteString("#EXT-X-ENDLIST\n")
	}

	return playlist.String(), nil
}

// worker processes transcode jobs.
func (s *TranscodeService) worker(id int) {
	defer s.wg.Done()

	s.logger.Debug("transcode worker started", slog.Int("worker_id", id))

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Debug("transcode worker stopping", slog.Int("worker_id", id))
			return
		case <-s.jobNotify:
			s.processNextJob(id)
		case <-time.After(5 * time.Second):
			// Periodic check for jobs (in case notification was missed)
			s.processNextJob(id)
		}
	}
}

// processNextJob finds and processes the next pending job.
func (s *TranscodeService) processNextJob(workerID int) {
	ctx := s.ctx

	// Get pending jobs sorted by priority
	jobs, err := s.store.ListPendingTranscodeJobs(ctx)
	if err != nil {
		s.logger.Error("failed to list pending jobs", slog.Any("error", err))
		return
	}

	if len(jobs) == 0 {
		return
	}

	// Try to claim the highest priority job
	job := jobs[0]
	job.MarkRunning()

	if err := s.store.UpdateTranscodeJob(ctx, job); err != nil {
		// Another worker got it first
		return
	}

	s.logger.Info("starting transcode",
		slog.Int("worker_id", workerID),
		slog.String("job_id", job.ID),
		slog.String("source", job.SourcePath),
	)

	// Execute transcode
	outputPath, err := s.executeTranscode(ctx, job)
	if err != nil {
		s.handleTranscodeError(ctx, job, err)
		return
	}

	// Get output file size
	info, err := os.Stat(outputPath)
	if err != nil {
		s.handleTranscodeError(ctx, job, fmt.Errorf("stat output: %w", err))
		return
	}

	// Mark completed
	job.MarkCompleted(outputPath, info.Size())
	if err := s.store.UpdateTranscodeJob(ctx, job); err != nil {
		s.logger.Error("failed to update completed job", slog.Any("error", err))
		return
	}

	s.logger.Info("transcode completed",
		slog.String("job_id", job.ID),
		slog.String("output", outputPath),
		slog.Int64("size", info.Size()),
	)

	// Emit completion event
	s.emitter.Emit(sse.NewTranscodeCompleteEvent(job.ID, job.BookID, job.AudioFileID))
}

// executeTranscode runs ffmpeg and returns the output path.
func (s *TranscodeService) executeTranscode(ctx context.Context, job *domain.TranscodeJob) (string, error) {
	// Check if FFmpeg can decode this codec before attempting transcode.
	// Some codecs (e.g., AC-4/Dolby Atmos) are proprietary and FFmpeg lacks decoders.
	if !s.canDecodeCodec(ctx, job.SourceCodec) {
		return "", fmt.Errorf("FFmpeg cannot decode %s codec - this format requires proprietary decoders", job.SourceCodec)
	}

	// Build output path: {cache}/{bookID}/{audioFileID}/{variant}/ (directory for HLS files)
	outputDir := filepath.Join(s.config.CachePath, job.BookID, job.AudioFileID, string(job.Variant))
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	// HLS outputs playlist.m3u8 and seg_XXXX.ts files in this directory
	outputPath := outputDir

	// Get source duration for progress calculation
	duration, err := s.getSourceDuration(ctx, job.SourcePath)
	if err != nil {
		s.logger.Warn("could not get source duration", slog.Any("error", err))
		duration = 0 // Will report progress in seconds instead of percentage
	}

	// Get source file info for bitrate matching
	bitrate, channels, err := s.getSourceInfo(ctx, job.SourcePath)
	if err != nil {
		s.logger.Warn("could not get source info, using defaults", slog.Any("error", err))
		bitrate = 128000
		channels = 2
	}

	// AC-4 (Dolby Atmos) special handling: ffprobe reports 2 channels (stereo bed),
	// but AC-4 contains object-based audio that decodes to 5.1 surround.
	// Force 6 channels to extract the full surround mix for Spatial Audio.
	if strings.EqualFold(job.SourceCodec, "ac4") || strings.EqualFold(job.SourceCodec, "ac-4") {
		channels = 6 // 5.1 surround
		// 5.1 needs higher bitrate than stereo for quality
		// ~64kbps per channel = 384kbps for 6 channels
		if bitrate < 384000 {
			bitrate = 384000
		}
		s.logger.Info("AC-4 detected, forcing 5.1 output for Spatial Audio",
			slog.String("job_id", job.ID),
			slog.Int("bitrate", bitrate),
		)
	}

	// Build ffmpeg command
	args := s.buildFFmpegArgs(job.SourcePath, outputPath, bitrate, channels, job.Variant)

	s.logger.Debug("executing ffmpeg",
		slog.String("job_id", job.ID),
		slog.Any("args", args),
	)

	cmd := exec.CommandContext(ctx, s.ffmpegPath, args...) //nolint:gosec // ffmpegPath is validated at service init

	// Capture stderr for progress parsing
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start ffmpeg: %w", err)
	}

	// Parse progress in goroutine
	go s.parseProgress(job, stderr, duration)

	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("ffmpeg failed: %w", err)
	}

	// Verify HLS playlist exists
	playlistPath := filepath.Join(outputPath, "playlist.m3u8")
	if _, err := os.Stat(playlistPath); err != nil {
		return "", fmt.Errorf("HLS playlist not created: %w", err)
	}

	// Return path to the HLS directory (contains playlist.m3u8 and seg_*.ts)
	return outputPath, nil
}

// buildFFmpegArgs constructs the ffmpeg command arguments for HLS output.
// HLS allows progressive playback - client can start as soon as first segment is ready.
func (s *TranscodeService) buildFFmpegArgs(input, outputDir string, bitrate, channels int, variant domain.TranscodeVariant) []string {
	// Override channels and bitrate based on variant
	if variant == domain.TranscodeVariantStereo {
		channels = 2
		if bitrate > 128000 {
			bitrate = 128000
		}
	} else if variant == domain.TranscodeVariantSpatial {
		channels = 6
		if bitrate < 384000 {
			bitrate = 384000
		}
	}

	playlistPath := filepath.Join(outputDir, "playlist.m3u8")
	segmentPattern := filepath.Join(outputDir, "seg_%04d.ts")

	args := []string{
		"-y",        // Overwrite output
		"-i", input, // Input file
		"-vn",         // No video
		"-c:a", "aac", // AAC codec
		"-b:a", fmt.Sprintf("%d", bitrate), // Bitrate based on variant
		"-ac", fmt.Sprintf("%d", channels), // Channels based on variant
		"-ar", "48000", // Standard sample rate
		"-f", "hls", // HLS format
		"-hls_time", "10", // 10 second segments
		"-hls_list_size", "0", // Keep all segments in playlist
		"-hls_playlist_type", "vod", // VOD playlist - adds #EXT-X-ENDLIST when complete
		"-hls_segment_type", "mpegts", // MPEG-TS segments
		"-hls_segment_filename", segmentPattern,
		playlistPath,
	}

	return args
}

// parseProgress reads ffmpeg stderr and emits progress events.
func (s *TranscodeService) parseProgress(job *domain.TranscodeJob, stderr io.Reader, durationMs int64) {
	// FFmpeg outputs: time=00:01:23.45
	timeRegex := regexp.MustCompile(`time=(\d+):(\d+):(\d+)\.(\d+)`)

	scanner := bufio.NewScanner(stderr)
	lastProgress := 0

	for scanner.Scan() {
		line := scanner.Text()

		matches := timeRegex.FindStringSubmatch(line)
		if len(matches) >= 4 {
			hours, _ := strconv.Atoi(matches[1])
			mins, _ := strconv.Atoi(matches[2])
			secs, _ := strconv.Atoi(matches[3])

			currentMs := int64((hours*3600 + mins*60 + secs) * 1000)

			var progress int
			if durationMs > 0 {
				progress = int(currentMs * 100 / durationMs)
			} else {
				// Fallback: report seconds as progress
				progress = int(currentMs / 1000)
			}

			// Only emit if progress changed significantly (every 5%)
			if progress-lastProgress >= 5 || progress == 100 {
				lastProgress = progress

				// Update job progress in store
				job.SetProgress(progress)
				if err := s.store.UpdateTranscodeJob(s.ctx, job); err != nil {
					s.logger.Warn("failed to update job progress", slog.Any("error", err))
				}

				// Emit progress event
				s.emitter.Emit(sse.NewTranscodeProgressEvent(
					job.ID, job.BookID, job.AudioFileID, progress,
				))
			}
		}
	}
}

// getSourceDuration uses ffprobe to get audio duration in milliseconds.
func (s *TranscodeService) getSourceDuration(ctx context.Context, path string) (int64, error) {
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		return 0, err
	}

	cmd := exec.CommandContext(ctx, ffprobe, //nolint:gosec // ffprobe path is from exec.LookPath
		"-v", "quiet",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)

	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	durationSec, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		return 0, err
	}

	return int64(durationSec * 1000), nil
}

// getSourceInfo uses ffprobe to get bitrate and channel count.
func (s *TranscodeService) getSourceInfo(ctx context.Context, path string) (bitrate, channels int, err error) {
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		return 128000, 2, err
	}

	// Get bitrate
	cmdBitrate := exec.CommandContext(ctx, ffprobe, //nolint:gosec // ffprobe path is from exec.LookPath
		"-v", "quiet",
		"-select_streams", "a:0",
		"-show_entries", "stream=bit_rate",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)

	output, err := cmdBitrate.Output()
	if err == nil {
		if b, err := strconv.Atoi(strings.TrimSpace(string(output))); err == nil && b > 0 {
			bitrate = b
		}
	}

	// Default bitrate if not found
	if bitrate == 0 {
		bitrate = 128000
	}

	// Cap at reasonable maximum
	if bitrate > 640000 {
		bitrate = 640000
	}

	// Get channels
	cmdChannels := exec.CommandContext(ctx, ffprobe, //nolint:gosec // ffprobe path is from exec.LookPath
		"-v", "quiet",
		"-select_streams", "a:0",
		"-show_entries", "stream=channels",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)

	output, err = cmdChannels.Output()
	if err == nil {
		if c, err := strconv.Atoi(strings.TrimSpace(string(output))); err == nil && c > 0 {
			channels = c
		}
	}

	// Default channels if not found
	if channels == 0 {
		channels = 2
	}

	return bitrate, channels, nil
}

// handleTranscodeError marks a job as failed and emits an event.
func (s *TranscodeService) handleTranscodeError(ctx context.Context, job *domain.TranscodeJob, err error) {
	s.logger.Error("transcode failed",
		slog.String("job_id", job.ID),
		slog.Any("error", err),
	)

	job.MarkFailed(err.Error())
	if updateErr := s.store.UpdateTranscodeJob(ctx, job); updateErr != nil {
		s.logger.Error("failed to update failed job", slog.Any("error", updateErr))
	}

	s.emitter.Emit(sse.NewTranscodeFailedEvent(job.ID, job.BookID, job.AudioFileID, err.Error()))
}

// recoverStalledJobs resets any jobs that were running when the server stopped.
func (s *TranscodeService) recoverStalledJobs() {
	ctx := context.Background()

	runningJobs, err := s.store.ListTranscodeJobsByStatus(ctx, domain.TranscodeStatusRunning)
	if err != nil {
		s.logger.Error("failed to list running jobs for recovery", slog.Any("error", err))
		return
	}

	for _, job := range runningJobs {
		s.logger.Info("recovering stalled transcode job",
			slog.String("job_id", job.ID),
		)

		job.Status = domain.TranscodeStatusPending
		job.Progress = 0
		job.StartedAt = nil

		if err := s.store.UpdateTranscodeJob(ctx, job); err != nil {
			s.logger.Error("failed to reset stalled job", slog.Any("error", err))
		}
	}

	if len(runningJobs) > 0 {
		s.logger.Info("recovered stalled jobs", slog.Int("count", len(runningJobs)))
		s.NotifyNewJob()
	}
}

// canDecodeCodec checks if FFmpeg has a decoder for the given codec.
// Some codecs (e.g., AC-4) are proprietary and FFmpeg doesn't include decoders.
func (s *TranscodeService) canDecodeCodec(ctx context.Context, codec string) bool {
	// Check FFmpeg decoders list
	cmd := exec.CommandContext(ctx, s.ffmpegPath, "-decoders") //nolint:gosec // ffmpegPath is validated at service init
	output, err := cmd.Output()
	if err != nil {
		s.logger.Warn("could not check FFmpeg decoders", slog.Any("error", err))
		// Optimistically assume it can decode - will fail later if not
		return true
	}

	// Normalize codec name for decoder lookup.
	// ffprobe reports "ac-4" but decoder is named "ac4".
	normalizedCodec := strings.ReplaceAll(strings.ToLower(codec), "-", "")

	// FFmpeg decoder output format: " A....D codec_name    Description"
	// where A = Audio, V = Video, S = Subtitle
	decoderLine := fmt.Sprintf(" %s ", normalizedCodec)
	canDecode := strings.Contains(string(output), decoderLine)
	s.logger.Debug("codec decoder check",
		slog.String("codec", codec),
		slog.String("normalized", normalizedCodec),
		slog.String("search_pattern", decoderLine),
		slog.Bool("can_decode", canDecode),
		slog.String("ffmpeg_path", s.ffmpegPath))
	return canDecode
}

// hashFile computes SHA256 hash of a file's contents.
func (s *TranscodeService) hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// DeleteTranscodesForBook removes all transcoded files and jobs for a book.
func (s *TranscodeService) DeleteTranscodesForBook(ctx context.Context, bookID string) error {
	// Delete jobs from store
	count, err := s.store.DeleteTranscodeJobsByBook(ctx, bookID)
	if err != nil {
		return err
	}

	// Delete cache directory for book
	bookCacheDir := filepath.Join(s.config.CachePath, bookID)
	if err := os.RemoveAll(bookCacheDir); err != nil && !os.IsNotExist(err) {
		s.logger.Warn("failed to delete book cache directory",
			slog.String("path", bookCacheDir),
			slog.Any("error", err),
		)
	}

	if count > 0 {
		s.logger.Info("deleted transcodes for book",
			slog.String("book_id", bookID),
			slog.Int("jobs_deleted", count),
		)
	}

	return nil
}

// IsEnabled returns whether transcoding is enabled.
func (s *TranscodeService) IsEnabled() bool {
	return s.config.Enabled && s.ffmpegPath != ""
}

// QueueTranscode implements scanner.TranscodeQueuer.
// It queues a transcode job for an audio file if transcoding is enabled and the codec needs it.
func (s *TranscodeService) QueueTranscode(ctx context.Context, bookID, audioFileID, sourcePath, sourceCodec string) error {
	if !s.IsEnabled() {
		return nil
	}

	if !domain.NeedsTranscode(sourceCodec) {
		return nil
	}

	// Create job with background priority (1) and spatial variant
	_, err := s.CreateJob(ctx, bookID, audioFileID, sourcePath, sourceCodec, 1, domain.TranscodeVariantSpatial)
	return err
}
