package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	"github.com/listenupapp/listenup-server/internal/color"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/service"
)

func (s *Server) registerProfileRoutes() {
	// Own profile
	huma.Register(s.api, huma.Operation{
		OperationID: "getMyProfile",
		Method:      http.MethodGet,
		Path:        "/api/v1/profile",
		Summary:     "Get my profile",
		Description: "Returns the authenticated user's profile",
		Tags:        []string{"Profile"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetMyProfile)

	huma.Register(s.api, huma.Operation{
		OperationID: "updateMyProfile",
		Method:      http.MethodPatch,
		Path:        "/api/v1/profile",
		Summary:     "Update my profile",
		Description: "Updates the authenticated user's profile settings",
		Tags:        []string{"Profile"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleUpdateMyProfile)

	huma.Register(s.api, huma.Operation{
		OperationID: "uploadAvatar",
		Method:      http.MethodPost,
		Path:        "/api/v1/profile/avatar",
		Summary:     "Upload avatar image",
		Description: "Uploads a new avatar image for the authenticated user",
		Tags:        []string{"Profile"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleUploadAvatar)

	// View any user's profile
	huma.Register(s.api, huma.Operation{
		OperationID: "getUserProfile",
		Method:      http.MethodGet,
		Path:        "/api/v1/users/{id}/profile",
		Summary:     "Get user profile",
		Description: "Returns a user's full profile including stats and recent activity",
		Tags:        []string{"Profile"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetUserProfile)

	// Avatar image serving (chi direct, not huma)
	s.router.Get("/avatars/{id}", s.handleServeAvatar)
}

// === Request/Response Types ===

// ProfileResponse contains basic profile data.
type ProfileResponse struct {
	UserID      string `json:"user_id" doc:"User ID"`
	FirstName   string `json:"first_name" doc:"User's first name"`
	LastName    string `json:"last_name" doc:"User's last name"`
	AvatarType  string `json:"avatar_type" doc:"Avatar type (auto or image)"`
	AvatarValue string `json:"avatar_value,omitempty" doc:"Avatar image path for image type"`
	AvatarColor string `json:"avatar_color" doc:"Avatar color for auto type"`
	Tagline     string `json:"tagline,omitempty" doc:"User's tagline (max 60 chars)"`
}

// ProfileOutput wraps the profile response for Huma.
type ProfileOutput struct {
	Body ProfileResponse
}

// UpdateProfileInput contains the update request.
type UpdateProfileInput struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	Body          struct {
		AvatarType  *string `json:"avatar_type,omitempty" enum:"auto,image" doc:"Avatar type"`
		Tagline     *string `json:"tagline,omitempty" maxLength:"60" doc:"User tagline"`
		FirstName   *string `json:"first_name,omitempty" maxLength:"100" doc:"User's first name"`
		LastName    *string `json:"last_name,omitempty" maxLength:"100" doc:"User's last name"`
		NewPassword *string `json:"new_password,omitempty" minLength:"8" doc:"New password (min 8 chars)"`
	}
}

// UploadAvatarInput contains the avatar upload request.
type UploadAvatarInput struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	ContentType   string `header:"Content-Type" doc:"Image content type"`
	RawBody       []byte
}

// GetUserProfileInput contains the user profile request.
type GetUserProfileInput struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	ID            string `path:"id" doc:"User ID"`
}

// RecentBookResponse contains minimal book info for profile display.
type RecentBookResponse struct {
	BookID     string  `json:"book_id" doc:"Book ID"`
	Title      string  `json:"title" doc:"Book title"`
	AuthorName string  `json:"author_name,omitempty" doc:"Primary author name"`
	CoverPath  string  `json:"cover_path,omitempty" doc:"Cover image path"`
	FinishedAt *string `json:"finished_at,omitempty" doc:"When the book was finished (RFC3339)"`
}

// ShelfSummaryResponse contains minimal shelf info for profile display.
type ShelfSummaryResponse struct {
	ID        string `json:"id" doc:"Shelf ID"`
	Name      string `json:"name" doc:"Shelf name"`
	BookCount int    `json:"book_count" doc:"Number of books in the shelf"`
}

// FullProfileResponse contains a complete profile for viewing.
type FullProfileResponse struct {
	UserID            string                `json:"user_id" doc:"User ID"`
	DisplayName       string                `json:"display_name" doc:"User's display name"`
	AvatarType        string                `json:"avatar_type" doc:"Avatar type (auto or image)"`
	AvatarValue       string                `json:"avatar_value,omitempty" doc:"Avatar image path for image type"`
	AvatarColor       string                `json:"avatar_color" doc:"Avatar color for auto type"`
	Tagline           string                `json:"tagline,omitempty" doc:"User's tagline"`
	TotalListenTimeMs int64                 `json:"total_listen_time_ms" doc:"Total listening time in milliseconds"`
	BooksFinished     int                   `json:"books_finished" doc:"Number of books finished"`
	CurrentStreak     int                   `json:"current_streak" doc:"Current listening streak in days"`
	LongestStreak     int                   `json:"longest_streak" doc:"Longest listening streak in days"`
	IsOwnProfile      bool                  `json:"is_own_profile" doc:"Whether viewing own profile"`
	RecentBooks       []RecentBookResponse  `json:"recent_books" doc:"Recently finished books"`
	PublicShelves      []ShelfSummaryResponse `json:"public_shelves" doc:"User's public shelves"`
}

// FullProfileOutput wraps the full profile response for Huma.
type FullProfileOutput struct {
	Body FullProfileResponse
}

// === Handlers ===

func (s *Server) handleGetMyProfile(ctx context.Context, _ *AuthenticatedInput) (*ProfileOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	profile, err := s.services.Profile.GetOrCreateProfile(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &ProfileOutput{
		Body: ProfileResponse{
			UserID:      user.ID,
			FirstName:   user.FirstName,
			LastName:    user.LastName,
			AvatarType:  string(profile.AvatarType),
			AvatarValue: profile.AvatarValue,
			AvatarColor: color.ForUser(user.ID),
			Tagline:     profile.Tagline,
		},
	}, nil
}

func (s *Server) handleUpdateMyProfile(ctx context.Context, input *UpdateProfileInput) (*ProfileOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	req := service.UpdateProfileRequest{}
	if input.Body.AvatarType != nil {
		avatarType := domain.AvatarType(*input.Body.AvatarType)
		req.AvatarType = &avatarType
	}
	if input.Body.Tagline != nil {
		req.Tagline = input.Body.Tagline
	}
	if input.Body.FirstName != nil {
		req.FirstName = input.Body.FirstName
	}
	if input.Body.LastName != nil {
		req.LastName = input.Body.LastName
	}
	if input.Body.NewPassword != nil {
		req.NewPassword = input.Body.NewPassword
	}

	profile, err := s.services.Profile.UpdateProfile(ctx, userID, req)
	if err != nil {
		return nil, err
	}

	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &ProfileOutput{
		Body: ProfileResponse{
			UserID:      user.ID,
			FirstName:   user.FirstName,
			LastName:    user.LastName,
			AvatarType:  string(profile.AvatarType),
			AvatarValue: profile.AvatarValue,
			AvatarColor: color.ForUser(user.ID),
			Tagline:     profile.Tagline,
		},
	}, nil
}

func (s *Server) handleUploadAvatar(ctx context.Context, input *UploadAvatarInput) (*ProfileOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Debug log the content type being received
	s.logger.Info("avatar upload request",
		"user_id", userID,
		"content_type", input.ContentType,
		"body_size", len(input.RawBody),
	)

	// Validate content type
	if !isValidImageType(input.ContentType) {
		s.logger.Warn("invalid avatar content type",
			"content_type", input.ContentType,
			"user_id", userID,
		)
		return nil, huma.Error400BadRequest(
			fmt.Sprintf("invalid image type '%s', must be image/jpeg, image/png, or image/webp", input.ContentType),
		)
	}

	profile, err := s.services.Profile.UploadAvatar(ctx, userID, input.RawBody)
	if err != nil {
		return nil, err
	}

	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &ProfileOutput{
		Body: ProfileResponse{
			UserID:      user.ID,
			FirstName:   user.FirstName,
			LastName:    user.LastName,
			AvatarType:  string(profile.AvatarType),
			AvatarValue: profile.AvatarValue,
			AvatarColor: color.ForUser(user.ID),
			Tagline:     profile.Tagline,
		},
	}, nil
}

func (s *Server) handleGetUserProfile(ctx context.Context, input *GetUserProfileInput) (*FullProfileOutput, error) {
	viewingUserID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	fullProfile, err := s.services.Profile.GetFullProfile(ctx, input.ID, viewingUserID)
	if err != nil {
		return nil, err
	}

	// Convert to response
	resp := FullProfileResponse{
		UserID:            fullProfile.UserID,
		DisplayName:       fullProfile.DisplayName,
		AvatarType:        string(fullProfile.AvatarType),
		AvatarValue:       fullProfile.AvatarValue,
		AvatarColor:       fullProfile.AvatarColor,
		Tagline:           fullProfile.Tagline,
		TotalListenTimeMs: fullProfile.TotalListenTimeMs,
		BooksFinished:     fullProfile.BooksFinished,
		CurrentStreak:     fullProfile.CurrentStreak,
		LongestStreak:     fullProfile.LongestStreak,
		IsOwnProfile:      fullProfile.IsOwnProfile,
		RecentBooks:       make([]RecentBookResponse, 0, len(fullProfile.RecentBooks)),
		PublicShelves:      make([]ShelfSummaryResponse, 0, len(fullProfile.PublicShelves)),
	}

	for _, book := range fullProfile.RecentBooks {
		rb := RecentBookResponse{
			BookID:     book.BookID,
			Title:      book.Title,
			AuthorName: book.AuthorName,
			CoverPath:  book.CoverPath,
		}
		if book.FinishedAt != nil {
			t := book.FinishedAt.Format("2006-01-02T15:04:05Z07:00")
			rb.FinishedAt = &t
		}
		resp.RecentBooks = append(resp.RecentBooks, rb)
	}

	for _, shelf := range fullProfile.PublicShelves {
		resp.PublicShelves = append(resp.PublicShelves, ShelfSummaryResponse{
			ID:        shelf.ID,
			Name:      shelf.Name,
			BookCount: shelf.BookCount,
		})
	}

	return &FullProfileOutput{Body: resp}, nil
}

func (s *Server) handleServeAvatar(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}

	// Remove .jpg extension if present
	if len(id) > 4 && id[len(id)-4:] == ".jpg" {
		id = id[:len(id)-4]
	}

	data, err := s.storage.Avatars.Get(id)
	if err != nil {
		http.Error(w, "avatar not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data)
}

// isValidImageType checks if the content type is a valid image type.
// Handles content types with parameters (e.g., "image/jpeg; charset=utf-8").
func isValidImageType(contentType string) bool {
	// Extract base media type (before any semicolon)
	mediaType := contentType
	if before, _, ok := strings.Cut(contentType, ";"); ok {
		mediaType = strings.TrimSpace(before)
	}

	switch mediaType {
	case "image/jpeg", "image/png", "image/webp":
		return true
	}
	return false
}
