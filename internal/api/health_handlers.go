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

type HealthResponse struct {
	Status string `json:"status" doc:"Health status"`
}

type HealthOutput struct {
	Body HealthResponse
}

func (s *Server) handleHealthCheck(ctx context.Context, _ *struct{}) (*HealthOutput, error) {
	return &HealthOutput{
		Body: HealthResponse{Status: "healthy"},
	}, nil
}
