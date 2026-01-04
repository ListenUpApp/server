package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/service"
)

func (s *Server) registerPlaybackRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "preparePlayback",
		Method:      http.MethodPost,
		Path:        "/api/v1/playback/prepare",
		Summary:     "Prepare audio playback",
		Description: "Negotiates audio format based on client capabilities. Returns stream URL for playable formats, or triggers transcoding for incompatible formats.",
		Tags:        []string{"Playback"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handlePreparePlayback)
}

func (s *Server) registerSettingsRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "getUserSettings",
		Method:      http.MethodGet,
		Path:        "/api/v1/settings",
		Summary:     "Get user settings",
		Description: "Returns user playback settings",
		Tags:        []string{"Settings"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetUserSettings)

	huma.Register(s.api, huma.Operation{
		OperationID: "updateUserSettings",
		Method:      http.MethodPatch,
		Path:        "/api/v1/settings",
		Summary:     "Update user settings",
		Description: "Updates user playback settings",
		Tags:        []string{"Settings"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleUpdateUserSettings)

	huma.Register(s.api, huma.Operation{
		OperationID: "getBookPreferences",
		Method:      http.MethodGet,
		Path:        "/api/v1/books/{id}/preferences",
		Summary:     "Get book preferences",
		Description: "Returns per-book preferences",
		Tags:        []string{"Settings"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetBookPreferences)

	huma.Register(s.api, huma.Operation{
		OperationID: "updateBookPreferences",
		Method:      http.MethodPatch,
		Path:        "/api/v1/books/{id}/preferences",
		Summary:     "Update book preferences",
		Description: "Updates per-book preferences",
		Tags:        []string{"Settings"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleUpdateBookPreferences)
}

// === DTOs ===

// GetUserSettingsInput contains parameters for getting user settings.
type GetUserSettingsInput struct {
	Authorization string `header:"Authorization"`
}

// UserSettingsResponse contains user settings data in API responses.
type UserSettingsResponse struct {
	DefaultPlaybackSpeed   float32   `json:"default_playback_speed" doc:"Default playback speed"`
	DefaultSkipForwardSec  int       `json:"default_skip_forward_sec" doc:"Default skip forward seconds"`
	DefaultSkipBackwardSec int       `json:"default_skip_backward_sec" doc:"Default skip backward seconds"`
	DefaultSleepTimerMin   *int      `json:"default_sleep_timer_min,omitempty" doc:"Default sleep timer minutes"`
	ShakeToResetSleepTimer bool      `json:"shake_to_reset_sleep_timer" doc:"Shake to reset sleep timer"`
	UpdatedAt              time.Time `json:"updated_at" doc:"Last update time"`
}

// UserSettingsOutput wraps the user settings response for Huma.
type UserSettingsOutput struct {
	Body UserSettingsResponse
}

// UpdateUserSettingsRequest is the request body for updating user settings.
type UpdateUserSettingsRequest struct {
	DefaultPlaybackSpeed   *float32 `json:"default_playback_speed" validate:"omitempty,gt=0,lte=4" doc:"Default playback speed"`
	DefaultSkipForwardSec  *int     `json:"default_skip_forward_sec" validate:"omitempty,gte=5,lte=300" doc:"Default skip forward seconds"`
	DefaultSkipBackwardSec *int     `json:"default_skip_backward_sec" validate:"omitempty,gte=5,lte=300" doc:"Default skip backward seconds"`
	DefaultSleepTimerMin   *int     `json:"default_sleep_timer_min" validate:"omitempty,gte=1,lte=480" doc:"Default sleep timer minutes"`
	ShakeToResetSleepTimer *bool    `json:"shake_to_reset_sleep_timer" doc:"Shake to reset sleep timer"`
}

// UpdateUserSettingsInput wraps the update user settings request for Huma.
type UpdateUserSettingsInput struct {
	Authorization string `header:"Authorization"`
	Body          UpdateUserSettingsRequest
}

// GetBookPreferencesInput contains parameters for getting book preferences.
type GetBookPreferencesInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
}

// BookPreferencesResponse contains book preferences data in API responses.
type BookPreferencesResponse struct {
	BookID                    string    `json:"book_id" doc:"Book ID"`
	PlaybackSpeed             *float32  `json:"playback_speed,omitempty" doc:"Playback speed override"`
	SkipForwardSec            *int      `json:"skip_forward_sec,omitempty" doc:"Skip forward override"`
	HideFromContinueListening bool      `json:"hide_from_continue_listening" doc:"Hide from continue listening"`
	UpdatedAt                 time.Time `json:"updated_at" doc:"Last update time"`
}

// BookPreferencesOutput wraps the book preferences response for Huma.
type BookPreferencesOutput struct {
	Body BookPreferencesResponse
}

// UpdateBookPreferencesRequest is the request body for updating book preferences.
type UpdateBookPreferencesRequest struct {
	PlaybackSpeed             *float32 `json:"playback_speed" validate:"omitempty,gt=0,lte=4" doc:"Playback speed override"`
	SkipForwardSec            *int     `json:"skip_forward_sec" validate:"omitempty,gte=5,lte=300" doc:"Skip forward override"`
	HideFromContinueListening *bool    `json:"hide_from_continue_listening" doc:"Hide from continue listening"`
}

// UpdateBookPreferencesInput wraps the update book preferences request for Huma.
type UpdateBookPreferencesInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
	Body          UpdateBookPreferencesRequest
}

// === Handlers ===

func (s *Server) handleGetUserSettings(ctx context.Context, input *GetUserSettingsInput) (*UserSettingsOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	settings, err := s.services.Listening.GetUserSettings(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &UserSettingsOutput{
		Body: UserSettingsResponse{
			DefaultPlaybackSpeed:   settings.DefaultPlaybackSpeed,
			DefaultSkipForwardSec:  settings.DefaultSkipForwardSec,
			DefaultSkipBackwardSec: settings.DefaultSkipBackwardSec,
			DefaultSleepTimerMin:   settings.DefaultSleepTimerMin,
			ShakeToResetSleepTimer: settings.ShakeToResetSleepTimer,
			UpdatedAt:              settings.UpdatedAt,
		},
	}, nil
}

func (s *Server) handleUpdateUserSettings(ctx context.Context, input *UpdateUserSettingsInput) (*UserSettingsOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	settings, err := s.services.Listening.UpdateUserSettings(ctx, userID, service.UpdateUserSettingsRequest{
		DefaultPlaybackSpeed:   input.Body.DefaultPlaybackSpeed,
		DefaultSkipForwardSec:  input.Body.DefaultSkipForwardSec,
		DefaultSkipBackwardSec: input.Body.DefaultSkipBackwardSec,
		DefaultSleepTimerMin:   input.Body.DefaultSleepTimerMin,
		ShakeToResetSleepTimer: input.Body.ShakeToResetSleepTimer,
	})
	if err != nil {
		return nil, err
	}

	return &UserSettingsOutput{
		Body: UserSettingsResponse{
			DefaultPlaybackSpeed:   settings.DefaultPlaybackSpeed,
			DefaultSkipForwardSec:  settings.DefaultSkipForwardSec,
			DefaultSkipBackwardSec: settings.DefaultSkipBackwardSec,
			DefaultSleepTimerMin:   settings.DefaultSleepTimerMin,
			ShakeToResetSleepTimer: settings.ShakeToResetSleepTimer,
			UpdatedAt:              settings.UpdatedAt,
		},
	}, nil
}

func (s *Server) handleGetBookPreferences(ctx context.Context, input *GetBookPreferencesInput) (*BookPreferencesOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	prefs, err := s.services.Listening.GetBookPreferences(ctx, userID, input.ID)
	if err != nil {
		return nil, err
	}

	return &BookPreferencesOutput{
		Body: BookPreferencesResponse{
			BookID:                    prefs.BookID,
			PlaybackSpeed:             prefs.PlaybackSpeed,
			SkipForwardSec:            prefs.SkipForwardSec,
			HideFromContinueListening: prefs.HideFromContinueListening,
			UpdatedAt:                 prefs.UpdatedAt,
		},
	}, nil
}

func (s *Server) handleUpdateBookPreferences(ctx context.Context, input *UpdateBookPreferencesInput) (*BookPreferencesOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	prefs, err := s.services.Listening.UpdateBookPreferences(ctx, userID, input.ID, service.UpdateBookPreferencesRequest{
		PlaybackSpeed:             input.Body.PlaybackSpeed,
		SkipForwardSec:            input.Body.SkipForwardSec,
		HideFromContinueListening: input.Body.HideFromContinueListening,
	})
	if err != nil {
		return nil, err
	}

	return &BookPreferencesOutput{
		Body: BookPreferencesResponse{
			BookID:                    prefs.BookID,
			PlaybackSpeed:             prefs.PlaybackSpeed,
			SkipForwardSec:            prefs.SkipForwardSec,
			HideFromContinueListening: prefs.HideFromContinueListening,
			UpdatedAt:                 prefs.UpdatedAt,
		},
	}, nil
}

// === Playback Prepare ===

// PreparePlaybackRequest is the request body for preparing audio playback.
type PreparePlaybackRequest struct {
	BookID       string   `json:"book_id" doc:"Book ID"`
	AudioFileID  string   `json:"audio_file_id" doc:"Audio file ID"`
	Capabilities []string `json:"capabilities" doc:"Codecs the client can play (e.g., aac, mp3, opus)"`
	Spatial      bool     `json:"spatial" doc:"Whether client prefers spatial audio"`
}

// PreparePlaybackInput wraps the prepare playback request for Huma.
type PreparePlaybackInput struct {
	Authorization string `header:"Authorization"`
	Body          PreparePlaybackRequest
}

// PreparePlaybackResponse contains playback preparation data in API responses.
type PreparePlaybackResponse struct {
	Ready          bool   `json:"ready" doc:"True if audio is ready to stream"`
	StreamURL      string `json:"stream_url" doc:"URL to stream the audio"`
	Variant        string `json:"variant" doc:"Which variant: original or transcoded"`
	Codec          string `json:"codec" doc:"Codec of the stream"`
	TranscodeJobID string `json:"transcode_job_id,omitempty" doc:"Job ID if transcoding in progress"`
	Progress       int    `json:"progress" doc:"Transcode progress (0-100)"`
}

// PreparePlaybackOutput wraps the prepare playback response for Huma.
type PreparePlaybackOutput struct {
	Body PreparePlaybackResponse
}

func (s *Server) handlePreparePlayback(ctx context.Context, input *PreparePlaybackInput) (*PreparePlaybackOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Get the book
	book, err := s.store.GetBook(ctx, input.Body.BookID, userID)
	if err != nil {
		return nil, huma.Error404NotFound("book not found")
	}

	// Find the audio file
	audioFile := book.GetAudioFileByID(input.Body.AudioFileID)
	if audioFile == nil {
		return nil, huma.Error404NotFound("audio file not found")
	}

	// Check if client can play the source format
	sourceCodec := audioFile.Codec
	canPlay := s.canClientPlayCodec(sourceCodec, input.Body.Capabilities)

	// Build base URL for streaming from request context
	// The actual base URL will be provided by the client in the stream URL it receives
	baseURL := ""

	if canPlay {
		// Client can play original format - return direct stream URL
		streamURL := baseURL + "/api/v1/audio/" + input.Body.BookID + "/" + input.Body.AudioFileID
		return &PreparePlaybackOutput{
			Body: PreparePlaybackResponse{
				Ready:     true,
				StreamURL: streamURL,
				Variant:   "original",
				Codec:     sourceCodec,
				Progress:  100,
			},
		}, nil
	}

	// Client cannot play source format - need transcoding
	variant := s.selectTranscodeVariant(input.Body.Spatial, sourceCodec)

	// Check for existing transcode job or create one
	job, err := s.services.Transcode.CreateJob(
		ctx,
		input.Body.BookID,
		input.Body.AudioFileID,
		audioFile.Path,
		sourceCodec,
		10, // High priority for user-requested playback
		variant,
	)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to prepare transcoding: " + err.Error())
	}

	// Build response based on job status
	switch job.Status { //nolint:exhaustive // Only handling the relevant status values
	case "completed":
		// Transcode ready - return HLS stream URL
		streamURL := baseURL + "/api/v1/audio/" + input.Body.BookID + "/" + input.Body.AudioFileID + "/transcode/playlist.m3u8"
		return &PreparePlaybackOutput{
			Body: PreparePlaybackResponse{
				Ready:     true,
				StreamURL: streamURL,
				Variant:   "transcoded",
				Codec:     "aac",
				Progress:  100,
			},
		}, nil

	case "failed":
		// Transcode failed - return error details
		return nil, huma.Error500InternalServerError("transcoding failed: " + job.Error)

	default:
		// Pending or running - return progress
		return &PreparePlaybackOutput{
			Body: PreparePlaybackResponse{
				Ready:          false,
				StreamURL:      "",
				Variant:        "transcoded",
				Codec:          "aac",
				TranscodeJobID: job.ID,
				Progress:       job.Progress,
			},
		}, nil
	}
}

// canClientPlayCodec checks if the client's capabilities include the given codec.
func (s *Server) canClientPlayCodec(codec string, capabilities []string) bool {
	// Normalize codec name
	codec = normalizeCodec(codec)

	for _, cap := range capabilities {
		if normalizeCodec(cap) == codec {
			return true
		}
	}
	return false
}

// normalizeCodec normalizes codec names for comparison.
func normalizeCodec(codec string) string {
	codec = strings.ToLower(codec)
	// Handle common aliases
	switch codec {
	case "m4a", "m4b", "mp4a":
		return "aac"
	case "mp4":
		return "aac"
	default:
		return codec
	}
}

// selectTranscodeVariant chooses the appropriate transcode variant.
func (s *Server) selectTranscodeVariant(preferSpatial bool, sourceCodec string) domain.TranscodeVariant {
	// If client prefers spatial and source is a surround format, use spatial variant
	// AC-3, E-AC-3, AC-4, TrueHD, DTS typically contain surround audio
	if preferSpatial {
		codec := strings.ToLower(sourceCodec)
		if codec == "ac3" || codec == "eac3" || codec == "ac4" || codec == "ac-4" ||
			codec == "truehd" || codec == "dts" {
			return domain.TranscodeVariantSpatial
		}
	}
	return domain.TranscodeVariantStereo
}
