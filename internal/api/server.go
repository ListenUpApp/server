// Package api provides the HTTP API server and handlers for the ListenUp application.
package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
)

// Server holds dependencies for HTTP handlers.
type Server struct {
	store                     *store.Store
	services                  *Services
	storage                   *StorageServices
	sseHandler                *sse.Handler
	sseManager                *sse.Manager
	registrationBroadcaster   *sse.RegistrationBroadcaster
	registrationStatusHandler *sse.RegistrationStatusHandler
	router                    chi.Router
	api                       huma.API
	logger                    *slog.Logger
	authRateLimiter           *RateLimiter
}

// NewServer creates a new HTTP server with all routes configured.
func NewServer(
	st *store.Store,
	services *Services,
	storage *StorageServices,
	sseHandler *sse.Handler,
	sseManager *sse.Manager,
	registrationBroadcaster *sse.RegistrationBroadcaster,
	logger *slog.Logger,
) *Server {
	router := chi.NewRouter()

	// Set up middleware BEFORE huma (which registers OpenAPI routes)
	setupMiddleware(router, logger)

	// Configure huma API
	config := huma.DefaultConfig("ListenUp API", "1.0.0")
	config.Info.Description = "Personal audiobook server API"
	config.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearer": {
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "PASETO",
			Description:  "PASETO access token from /auth/login",
		},
	}

	// Wrap all responses in {success, data, error} envelope for client compatibility
	config.Transformers = append(config.Transformers, EnvelopeTransformer)

	api := humachi.New(router, config)

	// Register custom error handler for domain errors
	RegisterErrorHandler()

	// Create registration status handler for pending user SSE
	registrationStatusHandler := sse.NewRegistrationStatusHandler(registrationBroadcaster, logger)

	s := &Server{
		store:                     st,
		services:                  services,
		storage:                   storage,
		sseHandler:                sseHandler,
		sseManager:                sseManager,
		registrationBroadcaster:   registrationBroadcaster,
		registrationStatusHandler: registrationStatusHandler,
		router:                    router,
		api:                       api,
		logger:                    logger,
		authRateLimiter:           NewRateLimiter(20, time.Minute, 10),
	}

	s.registerRoutes()

	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// setupMiddleware configures the middleware stack.
// Must be called before any routes are registered (including huma).
func setupMiddleware(router chi.Router, logger *slog.Logger) {
	// CORS - allow cross-origin requests (required for web clients)
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Requested-With"},
		ExposedHeaders:   []string{"Link", "X-Total-Count"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Security headers - protect against common web vulnerabilities
	router.Use(SecurityHeaders)

	// Request ID - generate unique ID for each request
	router.Use(middleware.RequestID)

	// Real IP - extract client IP from X-Forwarded-For / X-Real-IP headers
	router.Use(middleware.RealIP)

	// Structured logging - replaces chi's basic Logger with structured slog
	router.Use(StructuredLogger(logger))

	// Panic recovery - catch panics and return 500 instead of crashing
	router.Use(middleware.Recoverer)

	// Response compression - gzip responses at compression level 5
	router.Use(middleware.Compress(5))
}

// registerRoutes configures all HTTP routes.
func (s *Server) registerRoutes() {
	s.registerHealthRoutes()
	s.registerInstanceRoutes()
	s.registerAuthRoutes()
	s.registerInviteRoutes()
	s.registerUserRoutes()
	s.registerAdminRoutes()
	s.registerAdminCollectionRoutes()
	s.registerBookRoutes()
	s.registerMetadataRoutes()
	s.registerSeriesRoutes()
	s.registerContributorRoutes()
	s.registerCollectionRoutes()
	s.registerShareRoutes()
	s.registerLibraryRoutes()
	s.registerSyncRoutes()
	s.registerListeningRoutes()
	s.registerPlaybackRoutes()
	s.registerSettingsRoutes()
	s.registerGenreRoutes()
	s.registerTagRoutes()
	s.registerSearchRoutes()
	s.registerCoverRoutes()
	s.registerAudioRoutes()
	s.registerWebRoutes()
}
