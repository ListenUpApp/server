package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/service"
)

func (s *Server) registerAdminSettingsRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "getServerSettings",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/settings",
		Summary:     "Get server settings",
		Description: "Gets server-wide settings (admin only)",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetServerSettings)

	huma.Register(s.api, huma.Operation{
		OperationID: "updateServerSettings",
		Method:      http.MethodPatch,
		Path:        "/api/v1/admin/settings",
		Summary:     "Update server settings",
		Description: "Updates server-wide settings (admin only)",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleUpdateServerSettings)
}

// === DTOs ===

// ServerSettingsResponse is the API response for server settings.
type ServerSettingsResponse struct {
	InboxEnabled bool `json:"inbox_enabled" doc:"Whether inbox workflow is enabled"`
	InboxCount   int  `json:"inbox_count" doc:"Number of books currently in inbox"`
}

// GetServerSettingsInput is the Huma input for getting server settings.
type GetServerSettingsInput struct {
	Authorization string `header:"Authorization"`
}

// GetServerSettingsOutput is the Huma output wrapper for getting server settings.
type GetServerSettingsOutput struct {
	Body ServerSettingsResponse
}

// UpdateServerSettingsRequest is the request body for updating settings.
type UpdateServerSettingsRequest struct {
	InboxEnabled *bool `json:"inbox_enabled,omitempty" doc:"Enable or disable inbox workflow"`
}

// UpdateServerSettingsInput is the Huma input for updating server settings.
type UpdateServerSettingsInput struct {
	Authorization string `header:"Authorization"`
	Body          UpdateServerSettingsRequest
}

// UpdateServerSettingsOutput is the Huma output wrapper for updating server settings.
type UpdateServerSettingsOutput struct {
	Body ServerSettingsResponse
}

// === Handlers ===

func (s *Server) handleGetServerSettings(ctx context.Context, _ *GetServerSettingsInput) (*GetServerSettingsOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	settings, err := s.services.Settings.GetServerSettings(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get settings", err)
	}

	// Get inbox count
	inboxCount, err := s.services.Inbox.GetInboxCount(ctx)
	if err != nil {
		inboxCount = 0 // Non-fatal, just use 0
	}

	return &GetServerSettingsOutput{
		Body: ServerSettingsResponse{
			InboxEnabled: settings.InboxEnabled,
			InboxCount:   inboxCount,
		},
	}, nil
}

func (s *Server) handleUpdateServerSettings(ctx context.Context, input *UpdateServerSettingsInput) (*UpdateServerSettingsOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	settings, err := s.services.Settings.UpdateServerSettings(ctx, &service.SettingsUpdate{
		InboxEnabled: input.Body.InboxEnabled,
	})
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to update settings", err)
	}

	// Get inbox count
	inboxCount, err := s.services.Inbox.GetInboxCount(ctx)
	if err != nil {
		inboxCount = 0 // Non-fatal, just use 0
	}

	return &UpdateServerSettingsOutput{
		Body: ServerSettingsResponse{
			InboxEnabled: settings.InboxEnabled,
			InboxCount:   inboxCount,
		},
	}, nil
}
