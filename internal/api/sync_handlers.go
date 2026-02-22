package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/listenupapp/listenup-server/internal/color"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/dto"
	"github.com/listenupapp/listenup-server/internal/store"
)

func (s *Server) registerSyncRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "getSyncManifest",
		Method:      http.MethodGet,
		Path:        "/api/v1/sync/manifest",
		Summary:     "Get sync manifest",
		Description: "Returns library manifest with book IDs and counts for initial sync",
		Tags:        []string{"Sync"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetSyncManifest)

	huma.Register(s.api, huma.Operation{
		OperationID: "getSyncBooks",
		Method:      http.MethodGet,
		Path:        "/api/v1/sync/books",
		Summary:     "Get books for sync",
		Description: "Returns paginated books with optional delta sync support",
		Tags:        []string{"Sync"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetSyncBooks)

	huma.Register(s.api, huma.Operation{
		OperationID: "getSyncContributors",
		Method:      http.MethodGet,
		Path:        "/api/v1/sync/contributors",
		Summary:     "Get contributors for sync",
		Description: "Returns paginated contributors with optional delta sync support",
		Tags:        []string{"Sync"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetSyncContributors)

	huma.Register(s.api, huma.Operation{
		OperationID: "getSyncSeries",
		Method:      http.MethodGet,
		Path:        "/api/v1/sync/series",
		Summary:     "Get series for sync",
		Description: "Returns paginated series with optional delta sync support",
		Tags:        []string{"Sync"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetSyncSeries)

	huma.Register(s.api, huma.Operation{
		OperationID: "getSyncActiveSessions",
		Method:      http.MethodGet,
		Path:        "/api/v1/sync/active-sessions",
		Summary:     "Get active reading sessions",
		Description: "Returns all active reading sessions for populating discovery page during sync",
		Tags:        []string{"Sync"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetSyncActiveSessions)

	// NOTE: SSE endpoint registered directly on chi (not Huma) because Huma doesn't support SSE.
	// Route: GET /api/v1/sync/events - Server-Sent Events for real-time sync notifications
	// SSE endpoint (handled via chi directly, not huma)
	s.router.Get("/api/v1/sync/events", s.sseHandler.ServeHTTP)
}

// === DTOs ===

// GetSyncManifestInput contains parameters for getting the sync manifest.
type GetSyncManifestInput struct {
	Authorization string `header:"Authorization"`
}

// SyncManifestCountsResponse contains entity counts for the sync manifest.
type SyncManifestCountsResponse struct {
	Books        int `json:"books" doc:"Total books"`
	Contributors int `json:"contributors" doc:"Total contributors"`
	Series       int `json:"series" doc:"Total series"`
}

// SyncManifestResponse contains the sync manifest data.
type SyncManifestResponse struct {
	LibraryVersion string                     `json:"library_version" doc:"Library version timestamp"`
	Checkpoint     string                     `json:"checkpoint" doc:"Checkpoint for delta sync"`
	BookIDs        []string                   `json:"book_ids" doc:"All book IDs"`
	Counts         SyncManifestCountsResponse `json:"counts" doc:"Entity counts"`
}

// SyncManifestOutput wraps the sync manifest response for Huma.
type SyncManifestOutput struct {
	Body SyncManifestResponse
}

// GetSyncBooksInput contains parameters for getting books for sync.
type GetSyncBooksInput struct {
	Authorization string `header:"Authorization"`
	Cursor        string `query:"cursor" doc:"Pagination cursor"`
	Limit         int    `query:"limit" default:"50" minimum:"1" maximum:"500" doc:"Items per page"`
	UpdatedAfter  string `query:"updated_after" doc:"For delta sync, only return items updated after this time (RFC3339)"`
}

// SyncBooksResponse contains books for sync.
type SyncBooksResponse struct {
	NextCursor     string      `json:"next_cursor,omitempty" doc:"Next page cursor"`
	Books          []*dto.Book `json:"books" doc:"Books"`
	DeletedBookIDs []string    `json:"deleted_book_ids,omitempty" doc:"Deleted book IDs (for delta sync)"`
	HasMore        bool        `json:"has_more" doc:"Whether more pages exist"`
}

// SyncBooksOutput wraps the sync books response for Huma.
type SyncBooksOutput struct {
	Body SyncBooksResponse
}

// GetSyncContributorsInput contains parameters for getting contributors for sync.
type GetSyncContributorsInput struct {
	Authorization string `header:"Authorization"`
	Cursor        string `query:"cursor" doc:"Pagination cursor"`
	Limit         int    `query:"limit" default:"50" minimum:"1" maximum:"500" doc:"Items per page"`
	UpdatedAfter  string `query:"updated_after" doc:"For delta sync, only return items updated after this time (RFC3339)"`
}

// SyncContributorResponse contains contributor data for sync.
type SyncContributorResponse struct {
	ID            string    `json:"id" doc:"Contributor ID"`
	Name          string    `json:"name" doc:"Name"`
	SortName      string    `json:"sort_name,omitempty" doc:"Sort name"`
	Biography     string    `json:"biography,omitempty" doc:"Biography"`
	ImageURL      string    `json:"image_url,omitempty" doc:"Image URL"`
	ImageBlurHash string    `json:"image_blur_hash,omitempty" doc:"Image blur hash for placeholders"`
	Website       string    `json:"website,omitempty" doc:"Website"`
	BirthDate     string    `json:"birth_date,omitempty" doc:"Birth date (ISO 8601)"`
	DeathDate     string    `json:"death_date,omitempty" doc:"Death date (ISO 8601)"`
	Aliases       []string  `json:"aliases,omitempty" doc:"Pen names and aliases"`
	ASIN          string    `json:"asin,omitempty" doc:"Audible ASIN"`
	CreatedAt     time.Time `json:"created_at" doc:"Created time"`
	UpdatedAt     time.Time `json:"updated_at" doc:"Updated time"`
}

// SyncContributorsResponse contains contributors for sync.
type SyncContributorsResponse struct {
	NextCursor            string                    `json:"next_cursor,omitempty" doc:"Next page cursor"`
	Contributors          []SyncContributorResponse `json:"contributors" doc:"Contributors"`
	DeletedContributorIDs []string                  `json:"deleted_contributor_ids,omitempty" doc:"Deleted contributor IDs (for delta sync)"`
	HasMore               bool                      `json:"has_more" doc:"Whether more pages exist"`
}

// SyncContributorsOutput wraps the sync contributors response for Huma.
type SyncContributorsOutput struct {
	Body SyncContributorsResponse
}

// GetSyncSeriesInput contains parameters for getting series for sync.
type GetSyncSeriesInput struct {
	Authorization string `header:"Authorization"`
	Cursor        string `query:"cursor" doc:"Pagination cursor"`
	Limit         int    `query:"limit" default:"50" minimum:"1" maximum:"500" doc:"Items per page"`
	UpdatedAfter  string `query:"updated_after" doc:"For delta sync, only return items updated after this time (RFC3339)"`
}

// SyncSeriesItemResponse contains series data for sync.
type SyncSeriesItemResponse struct {
	ID          string                `json:"id" doc:"Series ID"`
	Name        string                `json:"name" doc:"Name"`
	Description string                `json:"description,omitempty" doc:"Description"`
	CoverImage  *domain.ImageFileInfo `json:"cover_image,omitempty" doc:"Cover image metadata"`
	ASIN        string                `json:"asin,omitempty" doc:"Audible ASIN"`
	CreatedAt   time.Time             `json:"created_at" doc:"Created time"`
	UpdatedAt   time.Time             `json:"updated_at" doc:"Updated time"`
}

// SyncSeriesResponse contains series for sync.
type SyncSeriesResponse struct {
	NextCursor       string                   `json:"next_cursor,omitempty" doc:"Next page cursor"`
	Series           []SyncSeriesItemResponse `json:"series" doc:"Series"`
	DeletedSeriesIDs []string                 `json:"deleted_series_ids,omitempty" doc:"Deleted series IDs (for delta sync)"`
	HasMore          bool                     `json:"has_more" doc:"Whether more pages exist"`
}

// SyncSeriesOutput wraps the sync series response for Huma.
type SyncSeriesOutput struct {
	Body SyncSeriesResponse
}

// === Handlers ===

func (s *Server) handleGetSyncManifest(ctx context.Context, _ *GetSyncManifestInput) (*SyncManifestOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	manifest, err := s.services.Sync.GetManifest(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &SyncManifestOutput{
		Body: SyncManifestResponse{
			LibraryVersion: manifest.LibraryVersion,
			Checkpoint:     manifest.Checkpoint,
			BookIDs:        manifest.BookIDs,
			Counts: SyncManifestCountsResponse{
				Books:        manifest.Counts.Books,
				Contributors: manifest.Counts.Contributors,
				Series:       manifest.Counts.Series,
			},
		},
	}, nil
}

func (s *Server) handleGetSyncBooks(ctx context.Context, input *GetSyncBooksInput) (*SyncBooksOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}

	params := store.PaginationParams{
		Cursor: input.Cursor,
		Limit:  limit,
	}
	if input.UpdatedAfter != "" {
		t, err := time.Parse(time.RFC3339, input.UpdatedAfter)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid updated_after format, expected RFC3339")
		}
		params.UpdatedAfter = t
	}

	result, err := s.services.Sync.GetBooksForSync(ctx, userID, params)
	if err != nil {
		return nil, err
	}

	return &SyncBooksOutput{
		Body: SyncBooksResponse{
			NextCursor:     result.NextCursor,
			Books:          result.Books,
			DeletedBookIDs: result.DeletedBookIDs,
			HasMore:        result.HasMore,
		},
	}, nil
}

func (s *Server) handleGetSyncContributors(ctx context.Context, input *GetSyncContributorsInput) (*SyncContributorsOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}

	params := store.PaginationParams{
		Cursor: input.Cursor,
		Limit:  limit,
	}
	if input.UpdatedAfter != "" {
		t, err := time.Parse(time.RFC3339, input.UpdatedAfter)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid updated_after format, expected RFC3339")
		}
		params.UpdatedAfter = t
	}

	result, err := s.services.Sync.GetContributorsForSync(ctx, userID, params)
	if err != nil {
		return nil, err
	}

	resp := make([]SyncContributorResponse, len(result.Contributors))
	for i, c := range result.Contributors {
		resp[i] = SyncContributorResponse{
			ID:            c.ID,
			Name:          c.Name,
			SortName:      c.SortName,
			Biography:     c.Biography,
			ImageURL:      c.ImageURL,
			ImageBlurHash: c.ImageBlurHash,
			Website:       c.Website,
			BirthDate:     c.BirthDate,
			DeathDate:     c.DeathDate,
			Aliases:       c.Aliases,
			ASIN:          c.ASIN,
			CreatedAt:     c.CreatedAt,
			UpdatedAt:     c.UpdatedAt,
		}
	}

	return &SyncContributorsOutput{
		Body: SyncContributorsResponse{
			NextCursor:            result.NextCursor,
			Contributors:          resp,
			DeletedContributorIDs: result.DeletedContributorIDs,
			HasMore:               result.HasMore,
		},
	}, nil
}

func (s *Server) handleGetSyncSeries(ctx context.Context, input *GetSyncSeriesInput) (*SyncSeriesOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}

	params := store.PaginationParams{
		Cursor: input.Cursor,
		Limit:  limit,
	}
	if input.UpdatedAfter != "" {
		t, err := time.Parse(time.RFC3339, input.UpdatedAfter)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid updated_after format, expected RFC3339")
		}
		params.UpdatedAfter = t
	}

	result, err := s.services.Sync.GetSeriesForSync(ctx, userID, params)
	if err != nil {
		return nil, err
	}

	resp := make([]SyncSeriesItemResponse, len(result.Series))
	for i, series := range result.Series {
		resp[i] = SyncSeriesItemResponse{
			ID:          series.ID,
			Name:        series.Name,
			Description: series.Description,
			CoverImage:  series.CoverImage,
			ASIN:        series.ASIN,
			CreatedAt:   series.CreatedAt,
			UpdatedAt:   series.UpdatedAt,
		}
	}

	return &SyncSeriesOutput{
		Body: SyncSeriesResponse{
			NextCursor:       result.NextCursor,
			Series:           resp,
			DeletedSeriesIDs: result.DeletedSeriesIDs,
			HasMore:          result.HasMore,
		},
	}, nil
}

// === Active Sessions ===

// GetSyncActiveSessionsInput contains parameters for getting active sessions.
type GetSyncActiveSessionsInput struct {
	Authorization string `header:"Authorization"`
}

// SyncActiveSessionResponse contains an active session for sync.
// Includes user profile data for offline-first client display.
type SyncActiveSessionResponse struct {
	SessionID   string    `json:"session_id" doc:"Unique session ID"`
	UserID      string    `json:"user_id" doc:"User ID"`
	BookID      string    `json:"book_id" doc:"Book ID"`
	StartedAt   time.Time `json:"started_at" doc:"When session started"`
	DisplayName string    `json:"display_name" doc:"User's display name"`
	AvatarType  string    `json:"avatar_type" doc:"Avatar type (auto or image)"`
	AvatarValue string    `json:"avatar_value,omitempty" doc:"Avatar URL path for image avatars"`
	AvatarColor string    `json:"avatar_color" doc:"Avatar background color in hex"`
}

// SyncActiveSessionsResponse contains active sessions for sync.
type SyncActiveSessionsResponse struct {
	Sessions []SyncActiveSessionResponse `json:"sessions" doc:"Active reading sessions"`
}

// SyncActiveSessionsOutput wraps the active sessions response for Huma.
type SyncActiveSessionsOutput struct {
	Body SyncActiveSessionsResponse
}

func (s *Server) handleGetSyncActiveSessions(ctx context.Context, _ *GetSyncActiveSessionsInput) (*SyncActiveSessionsOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	allSessions, err := s.store.GetAllActiveSessions(ctx)
	if err != nil {
		return nil, err
	}

	// Filter sessions to only include books the user can access
	activeSessions := make([]*domain.BookReadingSession, 0, len(allSessions))
	for _, session := range allSessions {
		canAccess, err := s.store.CanUserAccessBook(ctx, userID, session.BookID)
		if err != nil {
			continue // Skip sessions we can't verify access for
		}
		if canAccess {
			activeSessions = append(activeSessions, session)
		}
	}

	// Collect unique user IDs for batch fetching
	userIDSet := make(map[string]bool, len(activeSessions))
	for _, session := range activeSessions {
		userIDSet[session.UserID] = true
	}
	userIDs := make([]string, 0, len(userIDSet))
	for id := range userIDSet {
		userIDs = append(userIDs, id)
	}

	// Batch fetch all users and profiles
	users, err := s.store.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	userMap := make(map[string]*domain.User, len(users))
	for _, user := range users {
		userMap[user.ID] = user
	}

	profiles, err := s.store.GetUserProfilesByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	resp := make([]SyncActiveSessionResponse, 0, len(activeSessions))
	for _, session := range activeSessions {
		user, ok := userMap[session.UserID]
		if !ok {
			// Skip sessions for users we can't find (deleted, etc.)
			continue
		}

		// Get user profile for avatar settings, use defaults if not found
		profile, ok := profiles[session.UserID]
		if !ok {
			profile = &domain.UserProfile{
				AvatarType: domain.AvatarTypeAuto,
			}
		}

		resp = append(resp, SyncActiveSessionResponse{
			SessionID:   session.ID,
			UserID:      session.UserID,
			BookID:      session.BookID,
			StartedAt:   session.StartedAt,
			DisplayName: user.DisplayName,
			AvatarType:  string(profile.AvatarType),
			AvatarValue: profile.AvatarValue,
			AvatarColor: color.ForUser(session.UserID),
		})
	}

	return &SyncActiveSessionsOutput{
		Body: SyncActiveSessionsResponse{
			Sessions: resp,
		},
	}, nil
}
