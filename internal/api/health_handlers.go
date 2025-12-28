package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

func (s *Server) registerHealthRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "healthCheck",
		Method:      http.MethodGet,
		Path:        "/health",
		Summary:     "Health check",
		Description: "Returns server health status",
		Tags:        []string{"Health"},
	}, s.handleHealthCheck)
}

// HealthResponse contains health check data in API responses.
type HealthResponse struct {
	Status string `json:"status" doc:"Health status"`
}

// HealthOutput wraps the health response for Huma.
type HealthOutput struct {
	Body HealthResponse
}

func (s *Server) handleHealthCheck(_ context.Context, _ *struct{}) (*HealthOutput, error) {
	return &HealthOutput{
		Body: HealthResponse{Status: "healthy"},
	}, nil
}
