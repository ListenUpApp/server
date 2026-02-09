package api

import (
	"context"
	"net/http"

	"github.com/listenupapp/listenup-server/internal/service"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

func (s *Server) registerInstanceRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "getInstance",
		Method:      http.MethodGet,
		Path:        "/api/v1/instance",
		Summary:     "Get server instance",
		Description: "Returns server instance configuration and setup status",
		Tags:        []string{"Instance"},
	}, s.handleGetInstance)

	huma.Register(s.api, huma.Operation{
		OperationID: "updateInstance",
		Method:      http.MethodPatch,
		Path:        "/api/v1/admin/instance",
		Summary:     "Update instance settings",
		Description: "Updates instance configuration such as remote URL (admin only)",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleUpdateInstance)
}

// InstanceResponse contains server instance data in API responses.
type InstanceResponse struct {
	ID               string    `json:"id" doc:"Instance ID"`
	Name             string    `json:"name" doc:"Server name"`
	Version          string    `json:"version" doc:"Server version"`
	LocalURL         string    `json:"local_url" doc:"Local network URL"`
	RemoteURL        string    `json:"remote_url,omitempty" doc:"Remote access URL"`
	OpenRegistration bool      `json:"open_registration" doc:"Whether public registration is enabled"`
	CreatedAt        time.Time `json:"created_at" doc:"Creation timestamp"`
	UpdatedAt        time.Time `json:"updated_at" doc:"Last update timestamp"`
	SetupRequired    bool      `json:"setup_required" doc:"Whether initial setup is needed"`
}

// InstanceOutput wraps the instance response for Huma.
type InstanceOutput struct {
	Body InstanceResponse
}

func (s *Server) handleGetInstance(ctx context.Context, _ *struct{}) (*InstanceOutput, error) {
	instance, err := s.services.Instance.GetInstance(ctx)
	if err != nil {
		s.logger.Error("Failed to get instance", "error", err)
		return nil, huma.Error404NotFound("Server instance configuration not found")
	}

	return &InstanceOutput{
		Body: InstanceResponse{
			ID:               instance.ID,
			Name:             instance.Name,
			Version:          instance.Version,
			LocalURL:         instance.LocalURL,
			RemoteURL:        instance.RemoteURL,
			OpenRegistration: instance.OpenRegistration,
			CreatedAt:        instance.CreatedAt,
			UpdatedAt:        instance.UpdatedAt,
			SetupRequired:    instance.IsSetupRequired(),
		},
	}, nil
}

// UpdateInstanceRequest contains fields for updating instance settings.
type UpdateInstanceRequest struct {
	Name      *string `json:"name,omitempty" doc:"Server display name"`
	RemoteURL *string `json:"remote_url,omitempty" doc:"Remote access URL"`
}

// UpdateInstanceInput is the Huma input for updating instance settings.
type UpdateInstanceInput struct {
	Authorization string `header:"Authorization"`
	Body          UpdateInstanceRequest
}

func (s *Server) handleUpdateInstance(ctx context.Context, input *UpdateInstanceInput) (*InstanceOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	instance, err := s.services.Instance.UpdateInstanceSettings(ctx, &service.InstanceUpdate{
		Name:      input.Body.Name,
		RemoteURL: input.Body.RemoteURL,
	})
	if err != nil {
		s.logger.Error("Failed to update instance", "error", err)
		return nil, huma.Error500InternalServerError("failed to update instance settings", err)
	}

	// Notify mDNS to refresh if callback is set
	if s.onInstanceUpdated != nil {
		s.onInstanceUpdated(instance)
	}

	return &InstanceOutput{
		Body: InstanceResponse{
			ID:               instance.ID,
			Name:             instance.Name,
			Version:          instance.Version,
			LocalURL:         instance.LocalURL,
			RemoteURL:        instance.RemoteURL,
			OpenRegistration: instance.OpenRegistration,
			CreatedAt:        instance.CreatedAt,
			UpdatedAt:        instance.UpdatedAt,
			SetupRequired:    instance.IsSetupRequired(),
		},
	}, nil
}
