package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/store"
)

// Health status constants.
const (
	statusHealthy   = "healthy"
	statusDegraded  = "degraded"
	statusUnhealthy = "unhealthy"
)

// lastTicker is the minimal interface health checks need from background workers.
type lastTicker interface {
	LastTick() time.Time
}

const (
	workerStaleThreshold = 5 * time.Minute
	indexerHighDepth     = 50 // queue depth threshold for "high"
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
	overall := statusHealthy

	// Check database.
	dbHealth := s.checkDatabase(ctx)
	components["database"] = dbHealth
	if dbHealth.Status != statusHealthy {
		overall = statusUnhealthy
	}

	// Check search index
	searchHealth := s.checkSearchIndex()
	components["search"] = searchHealth
	if searchHealth.Status == statusUnhealthy {
		overall = statusUnhealthy
	} else if searchHealth.Status == statusDegraded && overall == statusHealthy {
		overall = statusDegraded
	}

	// Check SSE manager
	sseHealth := s.checkSSEManager()
	components["sse"] = sseHealth
	if sseHealth.Status == statusUnhealthy {
		overall = statusUnhealthy
	} else if sseHealth.Status == statusDegraded && overall == statusHealthy {
		overall = statusDegraded
	}

	// Async indexer.
	indexerHealth := s.checkAsyncIndexer()
	components["async_indexer"] = indexerHealth
	if indexerHealth.Status == statusUnhealthy {
		overall = statusUnhealthy
	} else if indexerHealth.Status == statusDegraded && overall == statusHealthy {
		overall = statusDegraded
	}

	// importJobs is a concrete typed pointer; convert to interface only when non-nil
	// to avoid the "typed nil wrapped in non-nil interface" trap.
	var importJobsTicker lastTicker
	if s.importJobs != nil {
		importJobsTicker = s.importJobs
	}

	// Background workers.
	workerChecks := []struct {
		name string
		lt   lastTicker
	}{
		{"file_watcher", s.fileWatcher},
		{"session_cleanup", s.sessionJob},
		{"event_log_cleanup", s.eventLogJob},
		{"import_jobs", importJobsTicker},
	}
	for _, w := range workerChecks {
		h := s.checkWorker(w.name, w.lt)
		components[w.name] = h
		if h.Status == statusUnhealthy {
			overall = statusUnhealthy
		} else if h.Status == statusDegraded && overall == statusHealthy {
			overall = statusDegraded
		}
	}

	return &HealthOutput{
		Body: HealthResponse{
			Status:     overall,
			Components: components,
		},
	}, nil
}

// checkDatabase verifies the database is accessible.
func (s *Server) checkDatabase(ctx context.Context) ComponentHealth {
	// Handle nil store (e.g., in tests)
	if s.store == nil {
		return ComponentHealth{
			Status:  statusDegraded,
			Message: "database not configured",
		}
	}

	start := time.Now()

	// Health check pings the DB directly: a HealthService wrapper would add no value here.
	// ErrServerNotFound is fine - DB is accessible, just not setup yet.
	_, err := s.store.GetInstance(ctx)
	latency := time.Since(start)

	if err != nil && !errors.Is(err, store.ErrServerNotFound) {
		return ComponentHealth{
			Status:  statusUnhealthy,
			Latency: latency.String(),
			Message: "database read failed",
		}
	}

	return ComponentHealth{
		Status:  statusHealthy,
		Latency: latency.String(),
	}
}

// checkSearchIndex verifies the Bleve index is accessible.
func (s *Server) checkSearchIndex() ComponentHealth {
	// Handle nil search service (e.g., in tests)
	if s.services == nil || s.services.Search == nil {
		return ComponentHealth{
			Status:  statusDegraded,
			Message: "search service not configured",
		}
	}

	start := time.Now()

	docCount, err := s.services.Search.DocumentCount()
	latency := time.Since(start)

	if err != nil {
		return ComponentHealth{
			Status:  statusUnhealthy,
			Latency: latency.String(),
			Message: "search index unreachable",
		}
	}

	// Index is accessible but might be empty (degraded during reindex)
	if docCount == 0 {
		return ComponentHealth{
			Status:  statusDegraded,
			Latency: latency.String(),
			Message: "search index empty",
		}
	}

	return ComponentHealth{
		Status:  statusHealthy,
		Latency: latency.String(),
	}
}

// checkSSEManager verifies the SSE event system is running.
func (s *Server) checkSSEManager() ComponentHealth {
	// Handle nil SSE manager (e.g., in tests)
	if s.sseManager == nil {
		return ComponentHealth{
			Status:  statusDegraded,
			Message: "SSE manager not configured",
		}
	}

	clientCount := s.sseManager.ClientCount()

	// SSE manager is always "healthy" if it exists and is accepting connections.
	// We could track if Start() has been called but that's complex.
	// Instead, just report current state.
	return ComponentHealth{
		Status:  statusHealthy,
		Message: formatSSEStatus(clientCount),
	}
}

// checkAsyncIndexer reports the indexer's drop count and queue depth as health.
func (s *Server) checkAsyncIndexer() ComponentHealth {
	if s.indexer == nil {
		return ComponentHealth{
			Status:  statusDegraded,
			Message: "async indexer not configured",
		}
	}
	drops := s.indexer.Drops()
	depth := s.indexer.QueueDepth()
	if drops > 0 {
		return ComponentHealth{
			Status:  statusDegraded,
			Message: fmt.Sprintf("%d index ops dropped (cumulative); queue at %d", drops, depth),
		}
	}
	if depth > indexerHighDepth {
		return ComponentHealth{
			Status:  statusDegraded,
			Message: fmt.Sprintf("queue depth %d (high)", depth),
		}
	}
	return ComponentHealth{
		Status:  statusHealthy,
		Message: fmt.Sprintf("queue depth %d, no drops", depth),
	}
}

// checkWorker reports a worker as degraded if it hasn't ticked recently.
func (s *Server) checkWorker(name string, lt lastTicker) ComponentHealth {
	if lt == nil {
		return ComponentHealth{
			Status:  statusDegraded,
			Message: name + " not configured",
		}
	}
	age := time.Since(lt.LastTick())
	if age > workerStaleThreshold {
		return ComponentHealth{
			Status:  statusDegraded,
			Message: fmt.Sprintf("last tick %s ago", age.Round(time.Second)),
		}
	}
	return ComponentHealth{
		Status:  statusHealthy,
		Message: fmt.Sprintf("last tick %s ago", age.Round(time.Second)),
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
