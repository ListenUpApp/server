package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/service"
)

func (s *Server) registerPlaybackRoutes() {
	// Nothing special here - playback routes are covered by listening handlers
	// This is a placeholder for any future playback-specific routes
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

type GetUserSettingsInput struct {
	Authorization string `header:"Authorization"`
}

type UserSettingsResponse struct {
	DefaultPlaybackSpeed   float32   `json:"default_playback_speed" doc:"Default playback speed"`
	DefaultSkipForwardSec  int       `json:"default_skip_forward_sec" doc:"Default skip forward seconds"`
	DefaultSkipBackwardSec int       `json:"default_skip_backward_sec" doc:"Default skip backward seconds"`
	DefaultSleepTimerMin   *int      `json:"default_sleep_timer_min,omitempty" doc:"Default sleep timer minutes"`
	ShakeToResetSleepTimer bool      `json:"shake_to_reset_sleep_timer" doc:"Shake to reset sleep timer"`
	UpdatedAt              time.Time `json:"updated_at" doc:"Last update time"`
}

type UserSettingsOutput struct {
	Body UserSettingsResponse
}

type UpdateUserSettingsRequest struct {
	DefaultPlaybackSpeed   *float32 `json:"default_playback_speed" validate:"omitempty,gt=0,lte=4" doc:"Default playback speed"`
	DefaultSkipForwardSec  *int     `json:"default_skip_forward_sec" validate:"omitempty,gte=5,lte=300" doc:"Default skip forward seconds"`
	DefaultSkipBackwardSec *int     `json:"default_skip_backward_sec" validate:"omitempty,gte=5,lte=300" doc:"Default skip backward seconds"`
	DefaultSleepTimerMin   *int     `json:"default_sleep_timer_min" validate:"omitempty,gte=1,lte=480" doc:"Default sleep timer minutes"`
	ShakeToResetSleepTimer *bool    `json:"shake_to_reset_sleep_timer" doc:"Shake to reset sleep timer"`
}

type UpdateUserSettingsInput struct {
	Authorization string `header:"Authorization"`
	Body          UpdateUserSettingsRequest
}

type GetBookPreferencesInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
}

type BookPreferencesResponse struct {
	BookID                    string    `json:"book_id" doc:"Book ID"`
	PlaybackSpeed             *float32  `json:"playback_speed,omitempty" doc:"Playback speed override"`
	SkipForwardSec            *int      `json:"skip_forward_sec,omitempty" doc:"Skip forward override"`
	HideFromContinueListening bool      `json:"hide_from_continue_listening" doc:"Hide from continue listening"`
	UpdatedAt                 time.Time `json:"updated_at" doc:"Last update time"`
}

type BookPreferencesOutput struct {
	Body BookPreferencesResponse
}

type UpdateBookPreferencesRequest struct {
	PlaybackSpeed             *float32 `json:"playback_speed" validate:"omitempty,gt=0,lte=4" doc:"Playback speed override"`
	SkipForwardSec            *int     `json:"skip_forward_sec" validate:"omitempty,gte=5,lte=300" doc:"Skip forward override"`
	HideFromContinueListening *bool    `json:"hide_from_continue_listening" doc:"Hide from continue listening"`
}

type UpdateBookPreferencesInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
	Body          UpdateBookPreferencesRequest
}

// === Handlers ===

func (s *Server) handleGetUserSettings(ctx context.Context, input *GetUserSettingsInput) (*UserSettingsOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
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
	userID, err := s.authenticateRequest(ctx, input.Authorization)
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
	userID, err := s.authenticateRequest(ctx, input.Authorization)
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
	userID, err := s.authenticateRequest(ctx, input.Authorization)
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
