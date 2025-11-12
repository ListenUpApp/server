package api

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/scanner"
	"github.com/listenupapp/listenup-server/internal/service"
	"github.com/listenupapp/listenup-server/internal/store"
)

// Server holds dependencies for HTTP handlers
type Server struct {
	instanceService *service.InstanceService
	bookService     *service.BookService
	router          *chi.Mux
	logger          *slog.Logger
}

// NewServer creates a new HTTP server with all routes configured
func NewServer(instanceService *service.InstanceService, bookService *service.BookService, logger *slog.Logger) *Server {
	s := &Server{
		instanceService: instanceService,
		bookService:     bookService,
		router:          chi.NewRouter(),
		logger:          logger,
	}

	s.setupMiddleware()
	s.setupRoutes()

	return s
}

// ServeHTTP implements http.Handler
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// setupMiddleware configures middleware stack
func (s *Server) setupMiddleware() {
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.Compress(5))
}

// setupRoutes configures all HTTP routes
func (s *Server) setupRoutes() {
	// Health check
	s.router.Get("/health", s.handleHealthCheck)

	// API v1
	s.router.Route("/api/v1", func(r chi.Router) {
		r.Get("/instance", s.handleGetInstance)

		// Books (temp)
		r.Get("/books", s.handleListBooks)
		r.Get("/books/{id}", s.handleGetBook)

		// Libraries (also temp)
		r.Post("/libraries/{id}/scan", s.handleTriggerScan)
	})
}

// handleHealthCheck returns server health status
func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	response.Success(w, map[string]string{
		"status": "healthy",
	}, s.logger)
}

// handleGetInstance returns the singleton server instance configuration
func (s *Server) handleGetInstance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	instance, err := s.instanceService.GetInstance(ctx)
	if err != nil {
		s.logger.Error("Failed to get instance", "error", err)
		response.NotFound(w, "Server instance configuration not found", s.logger)
		return
	}

	response.Success(w, instance, s.logger)
}

// Placeholder routes, since I haven't throught through our API layer yet.  Super basic logic
// For the time being.

// handleListBooks returns a paginated list of books
func (s *Server) handleListBooks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse pagination parameters from query string
	params := parsePaginationParams(r)

	books, err := s.bookService.ListBooks(ctx, params)
	if err != nil {
		s.logger.Error("Failed to list books", "error", err)
		response.InternalError(w, "Failed to retrieve books", s.logger)
		return
	}

	response.Success(w, books, s.logger)
}

// handleGetBook returns a single book by ID
func (s *Server) handleGetBook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if id == "" {
		response.BadRequest(w, "Book ID is required", s.logger)
		return
	}

	book, err := s.bookService.GetBook(ctx, id)
	if err != nil {
		if err == store.ErrBookNotFound {
			response.NotFound(w, "Book not found", s.logger)
			return
		}
		s.logger.Error("Failed to get book", "error", err, "id", id)
		response.InternalError(w, "Failed to retrieve book", s.logger)
		return
	}

	response.Success(w, book, s.logger)
}

// handleTriggerScan triggers a library scan
func (s *Server) handleTriggerScan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	libraryID := chi.URLParam(r, "id")

	if libraryID == "" {
		response.BadRequest(w, "Library ID is required", s.logger)
		return
	}

	// Parse force parameter
	force := r.URL.Query().Get("force") == "true"

	result, err := s.bookService.TriggerScan(ctx, libraryID, scanner.ScanOptions{
		Force: force,
	})
	if err != nil {
		s.logger.Error("Failed to trigger scan", "error", err, "library_id", libraryID)
		response.InternalError(w, "Failed to start library scan", s.logger)
		return
	}

	response.Success(w, result, s.logger)
}

// Helper functions

// parsePaginationParams parses pagination parameters from query string
func parsePaginationParams(r *http.Request) store.PaginationParams {
	params := store.DefaultPaginationParms()

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			params.Limit = limit
		}
	}

	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		params.Cursor = cursor
	}

	// Validate parameters
	params.Validate()

	return params
}
