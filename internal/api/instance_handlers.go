package api

import (
	"context"
	"net/http"
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
