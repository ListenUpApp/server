// Package api provides the HTTP API server and handlers for the ListenUp application.
package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/media/images"
	"github.com/listenupapp/listenup-server/internal/scanner"
	"github.com/listenupapp/listenup-server/internal/service"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
)

// Server holds dependencies for HTTP handlers.
type Server struct {
	store                   *store.Store
	instanceService         *service.InstanceService
	authService             *service.AuthService
	bookService             *service.BookService
	collectionService       *service.CollectionService
	sharingService          *service.SharingService
	syncService             *service.SyncService
	listeningService        *service.ListeningService
	genreService            *service.GenreService
	tagService              *service.TagService
	searchService           *service.SearchService
	sseHandler              *sse.Handler
	coverStorage            *images.Storage // Book cover images
	contributorImageStorage *images.Storage // Contributor profile photos
	router                  *chi.Mux
	logger                  *slog.Logger
	authRateLimiter         *RateLimiter
}

// NewServer creates a new HTTP server with all routes configured.
func NewServer(store *store.Store, instanceService *service.InstanceService, authService *service.AuthService, bookService *service.BookService, collectionService *service.CollectionService, sharingService *service.SharingService, syncService *service.SyncService, listeningService *service.ListeningService, genreService *service.GenreService, tagService *service.TagService, searchService *service.SearchService, sseHandler *sse.Handler, coverStorage *images.Storage, contributorImageStorage *images.Storage, logger *slog.Logger) *Server {
	s := &Server{
		store:                   store,
		instanceService:         instanceService,
		authService:             authService,
		bookService:             bookService,
		collectionService:       collectionService,
		sharingService:          sharingService,
		syncService:             syncService,
		listeningService:        listeningService,
		genreService:            genreService,
		tagService:              tagService,
		searchService:           searchService,
		sseHandler:              sseHandler,
		coverStorage:            coverStorage,
		contributorImageStorage: contributorImageStorage,
		router:                  chi.NewRouter(),
		logger:                  logger,
		// Rate limiter for login endpoint: 20 attempts per minute with burst of 10.
		// Sensible default for self-hosted - protects against brute force
		// without impeding legitimate use.
		authRateLimiter: NewRateLimiter(20, time.Minute, 10),
	}

	s.setupMiddleware()
	s.setupRoutes()

	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// setupMiddleware configures middleware stack.
func (s *Server) setupMiddleware() {
	// CORS middleware - permissive defaults for self-hosted deployments.
	// Users can restrict origins by placing a reverse proxy in front.
	s.router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Requested-With"},
		ExposedHeaders:   []string{"Link", "X-Total-Count"},
		AllowCredentials: true,
		MaxAge:           300, // 5 minutes
	}))

	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.Compress(5))
}

// setupRoutes configures all HTTP routes.
func (s *Server) setupRoutes() {
	// Health check.
	s.router.Get("/health", s.handleHealthCheck)

	// API v1.
	s.router.Route("/api/v1", func(r chi.Router) {
		r.Get("/instance", s.handleGetInstance)

		// Auth endpoints (public).
		r.Route("/auth", func(r chi.Router) {
			// Rate limit login/setup (brute-force attack vectors).
			// Refresh/logout don't need rate limiting:
			// - Refresh tokens are random and single-use
			// - Logout is harmless
			r.With(RateLimitMiddleware(s.authRateLimiter, s.logger)).Post("/setup", s.handleSetup)
			r.With(RateLimitMiddleware(s.authRateLimiter, s.logger)).Post("/login", s.handleLogin)
			r.Post("/refresh", s.handleRefresh)
			r.Post("/logout", s.handleLogout)
		})

		// Protected user endpoints.
		r.Route("/users", func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Get("/me", s.handleGetCurrentUser)
		})

		// Books (require auth for ACL).
		r.Route("/books", func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Get("/", s.handleListBooks)
			r.Get("/{id}", s.handleGetBook)
			r.Patch("/{id}", s.handleUpdateBook)
			r.Put("/{id}/contributors", s.handleSetBookContributors)
			r.Put("/{id}/series", s.handleSetBookSeries)
			r.Put("/{id}/cover", s.handleUploadCover)
			r.Get("/{id}/audio/{audioFileId}", s.handleStreamAudio)
			// Book genres and tags
			r.Get("/{id}/genres", s.handleGetBookGenres)
			r.Post("/{id}/genres", s.handleSetBookGenres)
			r.Get("/{id}/tags", s.handleGetBookTags)
			r.Post("/{id}/tags", s.handleAddBookTag)
			r.Delete("/{id}/tags/{tagID}", s.handleRemoveBookTag)
		})

		// Series (require auth for ACL).
		r.Route("/series", func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Get("/", s.handleListSeries)
			r.Get("/{id}", s.handleGetSeries)
			r.Get("/{id}/books", s.handleGetSeriesBooks)
		})

		// Contributors (require auth for ACL).
		r.Route("/contributors", func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Get("/", s.handleListContributors)
			r.Get("/search", s.handleSearchContributors)
			r.Get("/{id}", s.handleGetContributor)
			r.Put("/{id}", s.handleUpdateContributor)
			r.Get("/{id}/books", s.handleGetContributorBooks)
			r.Post("/{id}/merge", s.handleMergeContributors)
			r.Post("/{id}/unmerge", s.handleUnmergeContributor)
			r.Put("/{id}/image", s.handleUploadContributorImage)
			r.Get("/{id}/image", s.handleGetContributorImage)
		})

		// Collections (require auth).
		r.Route("/collections", func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Post("/", s.handleCreateCollection)
			r.Get("/", s.handleListCollections)
			r.Get("/{id}", s.handleGetCollection)
			r.Patch("/{id}", s.handleUpdateCollection)
			r.Delete("/{id}", s.handleDeleteCollection)
			r.Post("/{id}/books", s.handleAddBookToCollection)
			r.Delete("/{id}/books/{bookID}", s.handleRemoveBookFromCollection)
			r.Get("/{id}/books", s.handleGetCollectionBooks)
		})

		// Shares (require auth).
		r.Route("/shares", func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Post("/", s.handleCreateShare)
			r.Get("/", s.handleListShares)
			r.Get("/{id}", s.handleGetShare)
			r.Patch("/{id}", s.handleUpdateShare)
			r.Delete("/{id}", s.handleDeleteShare)
		})

		// Libraries (require auth).
		r.Route("/libraries", func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Post("/{id}/scan", s.handleTriggerScan)
		})

		// Sync endpoints (require auth for ACL filtering).
		r.Route("/sync", func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Get("/manifest", s.handleGetManifest)
			r.Get("/books", s.handleSyncBooks)
			r.Get("/series", s.handleSyncSeries)
			r.Get("/contributors", s.handleSyncContributors)
			r.Get("/stream", s.sseHandler.ServeHTTP)
		})

		// Listening events and progress (require auth).
		r.Route("/listening", func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Post("/events", s.handleRecordEvent)
			r.Get("/continue", s.handleGetContinueListening)
			r.Get("/progress/{bookID}", s.handleGetProgress)
			r.Delete("/progress/{bookID}", s.handleResetProgress)
			r.Get("/stats", s.handleGetUserStats)
			r.Get("/stats/{bookID}", s.handleGetBookStats)
		})

		// User settings (require auth).
		r.Route("/settings", func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Get("/", s.handleGetUserSettings)
			r.Patch("/", s.handleUpdateUserSettings)
		})

		// Book preferences (require auth).
		r.Route("/preferences", func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Get("/{bookID}", s.handleGetBookPreferences)
			r.Patch("/{bookID}", s.handleUpdateBookPreferences)
		})

		// Genres (public for list, auth for mutations).
		r.Route("/genres", func(r chi.Router) {
			r.Get("/", s.handleListGenres)              // Public: list all genres
			r.Get("/{id}", s.handleGetGenre)            // Public: get single genre
			r.Get("/{id}/books", s.handleGetGenreBooks) // Public: get books in genre

			r.Group(func(r chi.Router) {
				r.Use(s.requireAuth)
				r.Post("/", s.handleCreateGenre)                  // Create genre
				r.Put("/{id}", s.handleUpdateGenre)               // Update genre
				r.Delete("/{id}", s.handleDeleteGenre)            // Delete genre
				r.Post("/{id}/move", s.handleMoveGenre)           // Move genre in tree
				r.Post("/merge", s.handleMergeGenres)             // Merge two genres
				r.Get("/unmapped", s.handleListUnmappedGenres)    // List unmapped
				r.Post("/unmapped/map", s.handleMapUnmappedGenre) // Map unmapped
			})
		})

		// Tags (all require auth - user-specific).
		r.Route("/tags", func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Get("/", s.handleListTags)
			r.Post("/", s.handleCreateTag)
			r.Get("/{id}", s.handleGetTag)
			r.Patch("/{id}", s.handleUpdateTag)
			r.Delete("/{id}", s.handleDeleteTag)
			r.Get("/{id}/books", s.handleGetTagBooks)
		})

		// Search (require auth for ACL filtering).
		r.Route("/search", func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Get("/", s.handleSearch)
			r.Get("/stats", s.handleSearchStats)
			r.Post("/reindex", s.handleReindex)
		})

		// Cover images (public for sharing, cached aggressively).
		r.Get("/covers/{id}", s.handleGetCover)
	})
}

// handleHealthCheck returns server health status.
func (s *Server) handleHealthCheck(w http.ResponseWriter, _ *http.Request) {
	response.Success(w, map[string]string{
		"status": "healthy",
	}, s.logger)
}

// handleGetInstance returns the singleton server instance configuration.
func (s *Server) handleGetInstance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	instance, err := s.instanceService.GetInstance(ctx)
	if err != nil {
		s.logger.Error("Failed to get instance", "error", err)
		response.NotFound(w, "Server instance configuration not found", s.logger)
		return
	}

	instanceResponse := map[string]interface{}{
		"id":             instance.ID,
		"name":           instance.Name,
		"version":        instance.Version,
		"local_url":      instance.LocalURL,
		"remote_url":     instance.RemoteURL,
		"created_at":     instance.CreatedAt,
		"updated_at":     instance.UpdatedAt,
		"setup_required": instance.IsSetupRequired(),
	}

	response.Success(w, instanceResponse, s.logger)
}

// Placeholder routes, since I haven't thought through our API layer yet. Super basic logic.
// for the time being.

// handleListBooks returns a paginated list of books accessible to the authenticated user.
func (s *Server) handleListBooks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	// Parse pagination parameters from query string.
	params := parsePaginationParams(r)

	books, err := s.bookService.ListBooks(ctx, userID, params)
	if err != nil {
		s.logger.Error("Failed to list books", "error", err, "user_id", userID)
		response.InternalError(w, "Failed to retrieve books", s.logger)
		return
	}

	response.Success(w, books, s.logger)
}

// handleGetBook returns a single book by ID if the user has access.
func (s *Server) handleGetBook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	id := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if id == "" {
		response.BadRequest(w, "Book ID is required", s.logger)
		return
	}

	book, err := s.bookService.GetBook(ctx, userID, id)
	if err != nil {
		if errors.Is(err, store.ErrBookNotFound) {
			response.NotFound(w, "Book not found", s.logger)
			return
		}
		s.logger.Error("Failed to get book", "error", err, "id", id, "user_id", userID)
		response.InternalError(w, "Failed to retrieve book", s.logger)
		return
	}

	response.Success(w, book, s.logger)
}

// handleTriggerScan triggers a library scan.
func (s *Server) handleTriggerScan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	libraryID := chi.URLParam(r, "id")

	if libraryID == "" {
		response.BadRequest(w, "Library ID is required", s.logger)
		return
	}

	// Parse force parameter.
	force := r.URL.Query().Get("force") == "true"

	result, err := s.bookService.TriggerScan(ctx, libraryID, scanner.ScanOptions{
		Force: force,
	})
	if err != nil {
		s.logger.Error("Failed to trigger scan", "error", err, "library_id", libraryID)
		response.InternalError(w, "Failed to start library scan", s.logger)
		return
	}

	// Trigger search reindex after scan completes to ensure index is in sync.
	// This handles cases where async per-book indexing may have failed.
	if s.searchService != nil && (result.Added > 0 || result.Updated > 0) {
		go func() {
			// Use background context since HTTP request context will be cancelled
			reindexCtx := context.Background()
			s.logger.Info("Triggering search reindex after scan",
				"added", result.Added,
				"updated", result.Updated,
			)
			if err := s.searchService.ReindexAll(reindexCtx); err != nil {
				s.logger.Error("Failed to reindex after scan", "error", err)
			} else {
				count, _ := s.searchService.DocumentCount()
				s.logger.Info("Search reindex after scan completed", "documents", count)
			}
		}()
	}

	response.Success(w, result, s.logger)
}

// handleGetManifest returns the sync manifest (initial sync phase 1).
func (s *Server) handleGetManifest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	manifest, err := s.syncService.GetManifest(ctx)
	if err != nil {
		s.logger.Error("Failed to get manifest", "error", err)
		response.InternalError(w, "Failed to retrieve sync manifest", s.logger)
		return
	}

	response.Success(w, manifest, s.logger)
}

// handleSyncBooks returns paginated books for synching.
// Allows unauthenticated access (returns only uncollected books for anonymous users).
func (s *Server) handleSyncBooks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx) // May be empty for unauthenticated requests

	// Parse pagination parameters.
	params := parsePaginationParams(r)

	books, err := s.syncService.GetBooksForSync(ctx, userID, params)
	if err != nil {
		s.logger.Error("Failed to get books for sync", "error", err)
		response.InternalError(w, "Failed to retrieve books", s.logger)
		return
	}

	response.Success(w, books, s.logger)
}

// handleGetCover serves cover images for audiobooks.
func (s *Server) handleGetCover(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	if bookID == "" {
		response.BadRequest(w, "Book ID is required", s.logger)
		return
	}

	// Check if cover exists.
	if !s.coverStorage.Exists(bookID) {
		response.NotFound(w, "Cover not found for this book", s.logger)
		return
	}

	// Get cover file info for Last-Modified header.
	coverPath := s.coverStorage.Path(bookID)
	fileInfo, err := os.Stat(coverPath)
	if err != nil {
		s.logger.Error("Failed to stat cover file", "book_id", bookID, "error", err)
		response.InternalError(w, "Failed to retrieve cover", s.logger)
		return
	}

	// Compute ETag from hash.
	hash, err := s.coverStorage.Hash(bookID)
	if err != nil {
		s.logger.Error("Failed to compute cover hash", "book_id", bookID, "error", err)
		response.InternalError(w, "Failed to retrieve cover", s.logger)
		return
	}
	etag := `"` + hash + `"`

	// Check If-None-Match for cache validation.
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// Get cover data.
	data, err := s.coverStorage.Get(bookID)
	if err != nil {
		s.logger.Error("Failed to read cover file", "book_id", bookID, "error", err)
		response.InternalError(w, "Failed to retrieve cover", s.logger)
		return
	}

	// Set caching headers.
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "public, max-age=604800") // 1 week
	w.Header().Set("ETag", etag)
	w.Header().Set("Last-Modified", fileInfo.ModTime().UTC().Format(http.TimeFormat))

	// Write image data.
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		s.logger.Error("Failed to write cover response", "book_id", bookID, "error", err)
	}
}

// Helper functions.

// parsePaginationParams parses pagination parameters from query string.
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

	if updatedAfterStr := r.URL.Query().Get("updated_after"); updatedAfterStr != "" {
		if t, err := time.Parse(time.RFC3339, updatedAfterStr); err == nil {
			params.UpdatedAfter = t
		}
	}

	// Validate parameters.
	params.Validate()

	return params
}
