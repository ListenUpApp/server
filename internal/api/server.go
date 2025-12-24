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
	store           *store.Store
	services        *Services
	storage         *StorageServices
	sseHandler      *sse.Handler
	router          chi.Router
	api             huma.API
	logger          *slog.Logger
	authRateLimiter *RateLimiter
}

// NewServer creates a new HTTP server with all routes configured.
func NewServer(
	st *store.Store,
	services *Services,
	storage *StorageServices,
	sseHandler *sse.Handler,
	logger *slog.Logger,
) *Server {
	router := chi.NewRouter()

	// Set up middleware BEFORE huma (which registers OpenAPI routes)
	setupMiddleware(router)

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

	api := humachi.New(router, config)

	// Register custom error handler for domain errors
	RegisterErrorHandler()

	s := &Server{
		store:           st,
		services:        services,
		storage:         storage,
		sseHandler:      sseHandler,
		router:          router,
		api:             api,
		logger:          logger,
		authRateLimiter: NewRateLimiter(20, time.Minute, 10),
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
func setupMiddleware(router chi.Router) {
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Requested-With"},
		ExposedHeaders:   []string{"Link", "X-Total-Count"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
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
