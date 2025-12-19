package api

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/store"
)

// PreparePlaybackRequest is the request body for preparing playback.
type PreparePlaybackRequest struct {
	BookID       string   `json:"book_id"`
	AudioFileID  string   `json:"audio_file_id"`
	Capabilities []string `json:"capabilities"` // Codecs device can decode (e.g., ["aac", "ac4"])
	Spatial      bool     `json:"spatial"`      // Client wants spatial/surround audio
}

// PreparePlaybackResponse is the response from the prepare endpoint.
type PreparePlaybackResponse struct {
	// Ready indicates if the audio is ready to stream.
	// If false, check TranscodeJobID for progress.
	Ready bool `json:"ready"`

	// StreamURL is the URL to stream the audio.
	// Points to original or transcoded variant based on client capabilities.
	StreamURL string `json:"stream_url"`

	// Variant indicates which variant is being served.
	// "original" means the source file, "transcoded" means the converted AAC version.
	Variant string `json:"variant"`

	// Codec is the codec of the stream that will be served.
	Codec string `json:"codec"`

	// TranscodeJobID is set when transcoding is in progress (Ready=false).
	// Client can subscribe to SSE events for this job ID.
	TranscodeJobID string `json:"transcode_job_id,omitempty"`

	// Progress is the current transcoding progress (0-100) when Ready=false.
	Progress int `json:"progress,omitempty"`
}

// handlePreparePlayback negotiates the best audio format for playback.
// POST /api/v1/playback/prepare
//
// The client sends its supported codecs, and the server returns:
// - Original stream URL if the format is supported
// - Transcoded stream URL if transcode is ready
// - Transcode job info if transcoding is in progress
func (s *Server) handlePreparePlayback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	var req PreparePlaybackRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	if req.BookID == "" {
		response.BadRequest(w, "book_id is required", s.logger)
		return
	}

	if req.AudioFileID == "" {
		response.BadRequest(w, "audio_file_id is required", s.logger)
		return
	}

	// Get book (handles access control).
	book, err := s.services.Book.GetBook(ctx, userID, req.BookID)
	if err != nil {
		if errors.Is(err, store.ErrBookNotFound) {
			response.NotFound(w, "Book not found", s.logger)
			return
		}
		s.logger.Error("Failed to get book", "error", err, "book_id", req.BookID)
		response.InternalError(w, "Failed to retrieve book", s.logger)
		return
	}

	// Find the audio file.
	audioFile := book.GetAudioFileByID(req.AudioFileID)
	if audioFile == nil {
		response.NotFound(w, "Audio file not found", s.logger)
		return
	}

	// Determine playback strategy based on spatial preference and capabilities.
	clientSupportsSource := s.clientSupportsCodec(req.Capabilities, audioFile.Codec)
	sourceNeedsTranscode := domain.NeedsTranscode(audioFile.Codec)

	s.logger.Debug("PreparePlayback decision",
		"audio_file_id", audioFile.ID,
		"source_codec", audioFile.Codec,
		"client_capabilities", req.Capabilities,
		"spatial", req.Spatial,
		"client_supports_source", clientSupportsSource,
		"source_needs_transcode", sourceNeedsTranscode,
	)

	// Decision matrix:
	// 1. Source doesn't need transcode (e.g., AAC) - serve original regardless
	// 2. Spatial ON + client supports source (e.g., Samsung with AC-4) - serve original
	// 3. Spatial ON + client doesn't support source - transcode to 5.1
	// 4. Spatial OFF - transcode to stereo

	if !sourceNeedsTranscode {
		// Source is already universally playable (AAC, MP3, etc.)
		// Serve original
		resp := PreparePlaybackResponse{
			Ready:     true,
			StreamURL: s.buildStreamURL(req.BookID, req.AudioFileID, "original"),
			Variant:   "original",
			Codec:     audioFile.Codec,
		}
		response.Success(w, resp, s.logger)
		return
	}

	if req.Spatial && clientSupportsSource {
		// Client wants spatial AND can decode source natively (Samsung, Apple with Atmos)
		// Serve original
		resp := PreparePlaybackResponse{
			Ready:     true,
			StreamURL: s.buildStreamURL(req.BookID, req.AudioFileID, "original"),
			Variant:   "original",
			Codec:     audioFile.Codec,
		}
		response.Success(w, resp, s.logger)
		return
	}

	// Need to transcode - determine variant
	variant := domain.TranscodeVariantStereo
	if req.Spatial {
		variant = domain.TranscodeVariantSpatial
	}

	// Call prepareTranscodedPlayback with variant
	resp, err := s.prepareTranscodedPlayback(ctx, book, audioFile, variant)
	if err != nil {
		s.logger.Error("Failed to prepare transcoded playback",
			"error", err,
			"book_id", req.BookID,
			"audio_file_id", req.AudioFileID,
			"variant", variant,
		)
		response.InternalError(w, "Failed to prepare playback", s.logger)
		return
	}

	response.Success(w, resp, s.logger)
}

// prepareTranscodedPlayback handles the case where transcoding is needed.
func (s *Server) prepareTranscodedPlayback(
	ctx context.Context,
	book *domain.Book,
	audioFile *domain.AudioFileInfo,
	variant domain.TranscodeVariant,
) (*PreparePlaybackResponse, error) {
	// Check if transcoding is disabled.
	if s.services.Transcode == nil || !s.services.Transcode.IsEnabled() {
		// Transcoding disabled - serve original and hope for the best.
		return &PreparePlaybackResponse{
			Ready:     true,
			StreamURL: s.buildStreamURL(book.ID, audioFile.ID, "original"),
			Variant:   "original",
			Codec:     audioFile.Codec,
		}, nil
	}

	// Check if HLS content is ready for this variant (first segment available for early playback).
	// This allows streaming to start before transcoding completes.
	if _, ok := s.services.Transcode.GetHLSPathIfReadyForVariant(ctx, audioFile.ID, variant); ok {
		return &PreparePlaybackResponse{
			Ready:     true,
			StreamURL: s.buildStreamURL(book.ID, audioFile.ID, "transcoded"),
			Variant:   "transcoded",
			Codec:     "aac",
		}, nil
	}

	// Need to transcode - create/get job with high priority (user requested) and specified variant.
	job, err := s.services.Transcode.CreateJob(
		ctx,
		book.ID,
		audioFile.ID,
		audioFile.Path,
		audioFile.Codec,
		10, // High priority for user-requested playback
		variant,
	)
	if err != nil {
		return nil, fmt.Errorf("create transcode job: %w", err)
	}

	// Handle job status.
	switch job.Status {
	case domain.TranscodeStatusCompleted:
		// Job completed - HLS files should be available.
		return &PreparePlaybackResponse{
			Ready:     true,
			StreamURL: s.buildStreamURL(book.ID, audioFile.ID, "transcoded"),
			Variant:   "transcoded",
			Codec:     "aac",
		}, nil

	case domain.TranscodeStatusFailed:
		// Job failed - codec may be unsupported.
		// Return error so client knows playback isn't possible.
		return nil, fmt.Errorf("transcode failed: %s", job.Error)

	case domain.TranscodeStatusRunning:
		// Check if HLS is ready for early playback (first segment available).
		if _, ok := s.services.Transcode.GetHLSPathIfReadyForVariant(ctx, audioFile.ID, variant); ok {
			return &PreparePlaybackResponse{
				Ready:     true,
				StreamURL: s.buildStreamURL(book.ID, audioFile.ID, "transcoded"),
				Variant:   "transcoded",
				Codec:     "aac",
			}, nil
		}
		// HLS not yet ready - return progress.
		fallthrough

	default:
		// Transcode pending or running (HLS not yet ready).
		return &PreparePlaybackResponse{
			Ready:          false,
			StreamURL:      s.buildStreamURL(book.ID, audioFile.ID, "transcoded"),
			Variant:        "transcoded",
			Codec:          "aac",
			TranscodeJobID: job.ID,
			Progress:       job.Progress,
		}, nil
	}
}

// clientSupportsCodec checks if the client's capability list includes the codec.
func (s *Server) clientSupportsCodec(capabilities []string, codec string) bool {
	// If no capabilities provided, assume client supports common codecs.
	if len(capabilities) == 0 {
		return !domain.NeedsTranscode(codec)
	}

	// Normalize codec name for comparison.
	normalizedCodec := normalizeCodecName(codec)

	for _, cap := range capabilities {
		if normalizeCodecName(cap) == normalizedCodec {
			return true
		}
	}

	return false
}

// normalizeCodecName normalizes codec names for comparison.
func normalizeCodecName(codec string) string {
	switch codec {
	case "aac", "mp4a", "mp4a-latm":
		return "aac"
	case "mp3", "mp3float", "libmp3lame":
		return "mp3"
	case "opus", "libopus":
		return "opus"
	case "vorbis", "libvorbis":
		return "vorbis"
	case "flac":
		return "flac"
	case "pcm_s16le", "pcm_s24le", "pcm_s32le", "pcm_f32le":
		return "pcm"
	case "ac3", "eac3", "ac-3", "e-ac-3":
		return "ac3"
	case "ac4", "ac-4":
		return "ac4"
	case "dts", "dca":
		return "dts"
	case "wma", "wmav1", "wmav2", "wmapro":
		return "wma"
	case "truehd", "mlp":
		return "truehd"
	default:
		return codec
	}
}

// buildStreamURL constructs the URL for streaming audio.
func (s *Server) buildStreamURL(bookID, audioFileID, variant string) string {
	if variant == "transcoded" {
		// Return HLS playlist URL directly.
		// We can't use redirects because HTTP redirects don't forward Authorization headers,
		// which breaks authentication when ExoPlayer follows the redirect.
		return fmt.Sprintf("/api/v1/transcode/%s/playlist.m3u8", audioFileID)
	}
	return fmt.Sprintf("/api/v1/books/%s/audio/%s", bookID, audioFileID)
}
