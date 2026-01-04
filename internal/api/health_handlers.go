package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/store"
)

func (s *Server) registerHealthRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "healthCheck",
		Method:      http.MethodGet,
		Path:        "/health",
		Summary:     "Health check",
		Description: "Returns server health status with component checks",
		Tags:        []string{"Health"},
	}, s.handleHealthCheck)
}

// ComponentHealth describes the health of a single component.
type ComponentHealth struct {
	Status  string `json:"status" doc:"Component status: healthy, degraded, or unhealthy"`
	Latency string `json:"latency,omitempty" doc:"Response time for this component"`
	Message string `json:"message,omitempty" doc:"Additional status information"`
}

// HealthResponse contains health check data in API responses.
type HealthResponse struct {
	Status     string                     `json:"status" doc:"Overall status: healthy, degraded, or unhealthy"`
	Components map[string]ComponentHealth `json:"components" doc:"Individual component statuses"`
}

// HealthOutput wraps the health response for Huma.
type HealthOutput struct {
	Body HealthResponse
}

func (s *Server) handleHealthCheck(ctx context.Context, _ *struct{}) (*HealthOutput, error) {
	components := make(map[string]ComponentHealth)
	overall := "healthy"

	// Check database (BadgerDB)
	dbHealth := s.checkDatabase(ctx)
	components["database"] = dbHealth
	if dbHealth.Status != "healthy" {
		overall = "unhealthy"
	}

	// Check search index
	searchHealth := s.checkSearchIndex()
	components["search"] = searchHealth
	if searchHealth.Status == "unhealthy" {
		overall = "unhealthy"
	} else if searchHealth.Status == "degraded" && overall == "healthy" {
		overall = "degraded"
	}

	// Check SSE manager
	sseHealth := s.checkSSEManager()
	components["sse"] = sseHealth
	if sseHealth.Status == "unhealthy" {
		overall = "unhealthy"
	} else if sseHealth.Status == "degraded" && overall == "healthy" {
		overall = "degraded"
	}

	return &HealthOutput{
		Body: HealthResponse{
			Status:     overall,
			Components: components,
		},
	}, nil
}

// checkDatabase verifies BadgerDB is accessible.
func (s *Server) checkDatabase(ctx context.Context) ComponentHealth {
	// Handle nil store (e.g., in tests)
	if s.store == nil {
		return ComponentHealth{
			Status:  "degraded",
			Message: "database not configured",
		}
	}

	start := time.Now()

	// Quick read operation to verify DB is accessible.
	// ErrServerNotFound is fine - DB is accessible, just not setup yet.
	_, err := s.store.GetInstance(ctx)
	latency := time.Since(start)

	if err != nil && !errors.Is(err, store.ErrServerNotFound) {
		return ComponentHealth{
			Status:  "unhealthy",
			Latency: latency.String(),
			Message: "database read failed",
		}
	}

	return ComponentHealth{
		Status:  "healthy",
		Latency: latency.String(),
	}
}

// checkSearchIndex verifies the Bleve index is accessible.
func (s *Server) checkSearchIndex() ComponentHealth {
	// Handle nil search service (e.g., in tests)
	if s.services == nil || s.services.Search == nil {
		return ComponentHealth{
			Status:  "degraded",
			Message: "search service not configured",
		}
	}

	start := time.Now()

	docCount, err := s.services.Search.DocumentCount()
	latency := time.Since(start)

	if err != nil {
		return ComponentHealth{
			Status:  "unhealthy",
			Latency: latency.String(),
			Message: "search index unreachable",
		}
	}

	// Index is accessible but might be empty (degraded during reindex)
	if docCount == 0 {
		return ComponentHealth{
			Status:  "degraded",
			Latency: latency.String(),
			Message: "search index empty",
		}
	}

	return ComponentHealth{
		Status:  "healthy",
		Latency: latency.String(),
	}
}

// checkSSEManager verifies the SSE event system is running.
func (s *Server) checkSSEManager() ComponentHealth {
	// Handle nil SSE manager (e.g., in tests)
	if s.sseManager == nil {
		return ComponentHealth{
			Status:  "degraded",
			Message: "SSE manager not configured",
		}
	}

	clientCount := s.sseManager.ClientCount()

	// SSE manager is always "healthy" if it exists and is accepting connections.
	// We could track if Start() has been called but that's complex.
	// Instead, just report current state.
	return ComponentHealth{
		Status:  "healthy",
		Message: formatSSEStatus(clientCount),
	}
}

func formatSSEStatus(count int) string {
	switch count {
	case 0:
		return "no connected clients"
	case 1:
		return "1 connected client"
	default:
		return formatInt(count) + " connected clients"
	}
}

func formatInt(n int) string {
	// Simple int to string without importing strconv
	if n == 0 {
		return "0"
	}
	digits := make([]byte, 0, 10)
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
