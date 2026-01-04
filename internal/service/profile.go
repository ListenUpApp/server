package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/listenupapp/listenup-server/internal/auth"
	"github.com/listenupapp/listenup-server/internal/color"
	"github.com/listenupapp/listenup-server/internal/domain"
	domainerrors "github.com/listenupapp/listenup-server/internal/errors"
	"github.com/listenupapp/listenup-server/internal/media/images"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
)

// MaxTaglineLength is the maximum number of characters allowed in a tagline.
const MaxTaglineLength = 60

// MaxAvatarSize is the maximum avatar image size in bytes (2MB).
const MaxAvatarSize = 2 * 1024 * 1024

// ProfileService provides user profile management.
type ProfileService struct {
	store      *store.Store
	avatars    *images.Storage
	sseManager *sse.Manager
	stats      *StatsService
	logger     *slog.Logger
}

// NewProfileService creates a new profile service.
func NewProfileService(
	store *store.Store,
	avatars *images.Storage,
	sseManager *sse.Manager,
	stats *StatsService,
	logger *slog.Logger,
) *ProfileService {
	return &ProfileService{
		store:      store,
		avatars:    avatars,
		sseManager: sseManager,
		stats:      stats,
		logger:     logger,
	}
}

// GetOrCreateProfile returns a user's profile, creating a default if none exists.
func (s *ProfileService) GetOrCreateProfile(ctx context.Context, userID string) (*domain.UserProfile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	profile, err := s.store.GetUserProfile(ctx, userID)
	if err == nil {
		return profile, nil
	}

	if !errors.Is(err, store.ErrProfileNotFound) {
		return nil, fmt.Errorf("get profile: %w", err)
	}

	// Create default profile
	profile = domain.NewUserProfile(userID)
	if err := s.store.SaveUserProfile(ctx, profile); err != nil {
		return nil, fmt.Errorf("create default profile: %w", err)
	}

	s.logger.Info("created default profile", "user_id", userID)
	return profile, nil
}

// UpdateProfileRequest contains optional fields to update.
type UpdateProfileRequest struct {
	AvatarType  *domain.AvatarType
	Tagline     *string
	FirstName   *string
	LastName    *string
	NewPassword *string
}

// UpdateProfile updates a user's profile settings.
func (s *ProfileService) UpdateProfile(ctx context.Context, userID string, req UpdateProfileRequest) (*domain.UserProfile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	profile, err := s.GetOrCreateProfile(ctx, userID)
	if err != nil {
		return nil, err
	}

	if req.AvatarType != nil {
		profile.AvatarType = *req.AvatarType
		// Clear avatar value when switching to auto
		if *req.AvatarType == domain.AvatarTypeAuto {
			// Delete old avatar image if it exists
			if profile.AvatarValue != "" {
				if err := s.avatars.Delete(userID); err != nil {
					s.logger.Warn("failed to delete old avatar", "user_id", userID, "error", err)
				}
			}
			profile.AvatarValue = ""
		}
	}

	if req.Tagline != nil {
		tagline := *req.Tagline
		if len(tagline) > MaxTaglineLength {
			return nil, domainerrors.Validation(fmt.Sprintf("tagline must be %d characters or less", MaxTaglineLength))
		}
		profile.Tagline = tagline
	}

	// Handle firstName, lastName, and password changes
	if req.FirstName != nil || req.LastName != nil || req.NewPassword != nil {
		if err := s.updateUserDetails(ctx, userID, req); err != nil {
			return nil, err
		}
	}

	profile.UpdatedAt = time.Now()

	if err := s.store.SaveUserProfile(ctx, profile); err != nil {
		return nil, fmt.Errorf("save profile: %w", err)
	}

	s.logger.Info("profile updated", "user_id", userID)

	// Broadcast SSE event
	s.broadcastProfileUpdate(ctx, userID, profile)

	return profile, nil
}

// updateUserDetails handles updating user fields (firstName, lastName, password).
func (s *ProfileService) updateUserDetails(ctx context.Context, userID string, req UpdateProfileRequest) error {
	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	userChanged := false

	// Update first name
	if req.FirstName != nil {
		user.FirstName = *req.FirstName
		userChanged = true
		s.logger.Debug("updating first name", "user_id", userID)
	}

	// Update last name
	if req.LastName != nil {
		user.LastName = *req.LastName
		userChanged = true
		s.logger.Debug("updating last name", "user_id", userID)
	}

	// Handle password change
	if req.NewPassword != nil {
		// Validate new password
		if len(*req.NewPassword) < 8 {
			return domainerrors.Validation("new password must be at least 8 characters")
		}

		// Hash new password
		newHash, err := auth.HashPassword(*req.NewPassword)
		if err != nil {
			return fmt.Errorf("hash password: %w", err)
		}

		user.PasswordHash = newHash
		userChanged = true
		s.logger.Info("password changed", "user_id", userID)
	}

	// Save user if changed
	if userChanged {
		if err := s.store.UpdateUser(ctx, user); err != nil {
			return fmt.Errorf("save user: %w", err)
		}
	}

	return nil
}

// UploadAvatar saves an avatar image and updates the profile.
func (s *ProfileService) UploadAvatar(ctx context.Context, userID string, imageData []byte) (*domain.UserProfile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(imageData) == 0 {
		return nil, domainerrors.Validation("image data cannot be empty")
	}

	if len(imageData) > MaxAvatarSize {
		return nil, domainerrors.Validation(fmt.Sprintf("image too large, max %d bytes", MaxAvatarSize))
	}

	// Save image to storage
	if err := s.avatars.Save(userID, imageData); err != nil {
		return nil, fmt.Errorf("save avatar image: %w", err)
	}

	// Update profile
	profile, err := s.GetOrCreateProfile(ctx, userID)
	if err != nil {
		return nil, err
	}

	profile.AvatarType = domain.AvatarTypeImage
	profile.AvatarValue = fmt.Sprintf("/avatars/%s.jpg", userID)
	profile.UpdatedAt = time.Now()

	if err := s.store.SaveUserProfile(ctx, profile); err != nil {
		return nil, fmt.Errorf("save profile: %w", err)
	}

	s.logger.Info("avatar uploaded", "user_id", userID)

	// Broadcast SSE event
	s.broadcastProfileUpdate(ctx, userID, profile)

	return profile, nil
}

// FullUserProfile contains everything needed to render a profile page.
type FullUserProfile struct {
	// Identity
	UserID      string            `json:"user_id"`
	DisplayName string            `json:"display_name"`
	AvatarType  domain.AvatarType `json:"avatar_type"`
	AvatarValue string            `json:"avatar_value,omitempty"`
	AvatarColor string            `json:"avatar_color"`
	Tagline     string            `json:"tagline,omitempty"`

	// Stats
	TotalListenTimeMs int64 `json:"total_listen_time_ms"`
	BooksFinished     int   `json:"books_finished"`
	CurrentStreak     int   `json:"current_streak"`
	LongestStreak     int   `json:"longest_streak"`

	// Recent activity (filtered by viewer's ACL)
	RecentBooks  []RecentBookSummary `json:"recent_books"`
	PublicLenses []LensSummary       `json:"public_lenses"`

	// Meta
	IsOwnProfile bool `json:"is_own_profile"`
}

// RecentBookSummary contains minimal book info for profile display.
type RecentBookSummary struct {
	BookID     string     `json:"book_id"`
	Title      string     `json:"title"`
	AuthorName string     `json:"author_name,omitempty"`
	CoverPath  string     `json:"cover_path,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// LensSummary contains minimal lens info for profile display.
type LensSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	BookCount int    `json:"book_count"`
}

// GetFullProfile returns a complete profile for display.
// viewingUserID is used for ACL filtering on recent books.
func (s *ProfileService) GetFullProfile(ctx context.Context, profileUserID, viewingUserID string) (*FullUserProfile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Get user
	user, err := s.store.GetUser(ctx, profileUserID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	// Get profile
	profile, err := s.GetOrCreateProfile(ctx, profileUserID)
	if err != nil {
		return nil, fmt.Errorf("get profile: %w", err)
	}

	// Get all-time stats
	stats, err := s.stats.GetUserStats(ctx, profileUserID, domain.StatsPeriodAllTime)
	if err != nil {
		return nil, fmt.Errorf("get stats: %w", err)
	}

	// Get recent finished books (filtered by viewer's access)
	recentBooks, err := s.getRecentBooksFiltered(ctx, profileUserID, viewingUserID, 5)
	if err != nil {
		s.logger.Warn("failed to get recent books", "error", err)
		recentBooks = []RecentBookSummary{}
	}

	// Get public lenses
	lenses, err := s.getPublicLenses(ctx, profileUserID)
	if err != nil {
		s.logger.Warn("failed to get lenses", "error", err)
		lenses = []LensSummary{}
	}

	return &FullUserProfile{
		UserID:            user.ID,
		DisplayName:       user.Name(),
		AvatarType:        profile.AvatarType,
		AvatarValue:       profile.AvatarValue,
		AvatarColor:       color.ForUser(user.ID),
		Tagline:           profile.Tagline,
		TotalListenTimeMs: stats.TotalListenTimeMs,
		BooksFinished:     stats.BooksFinished,
		CurrentStreak:     stats.CurrentStreakDays,
		LongestStreak:     stats.LongestStreakDays,
		RecentBooks:       recentBooks,
		PublicLenses:      lenses,
		IsOwnProfile:      profileUserID == viewingUserID,
	}, nil
}

// getRecentBooksFiltered returns the profile user's recently finished books,
// filtered to only books the viewing user can access.
func (s *ProfileService) getRecentBooksFiltered(ctx context.Context, profileUserID, viewingUserID string, limit int) ([]RecentBookSummary, error) {
	// Get profile user's finished progress
	finishedProgress, err := s.store.GetProgressFinishedInRange(ctx, profileUserID, time.Time{}, time.Now())
	if err != nil {
		return nil, err
	}

	// Sort by finished time descending
	// finishedProgress is already sorted by FinishedAt in store

	// Get viewing user's accessible book IDs
	accessibleBooks, err := s.store.GetBooksForUser(ctx, viewingUserID)
	if err != nil {
		return nil, err
	}
	accessibleSet := make(map[string]bool, len(accessibleBooks))
	for _, book := range accessibleBooks {
		accessibleSet[book.ID] = true
	}

	// Filter and build response
	var result []RecentBookSummary
	seenBooks := make(map[string]bool)

	for _, progress := range finishedProgress {
		if !accessibleSet[progress.BookID] {
			continue
		}
		if seenBooks[progress.BookID] {
			continue
		}
		seenBooks[progress.BookID] = true

		book, err := s.store.GetBook(ctx, progress.BookID, viewingUserID)
		if err != nil {
			continue
		}

		result = append(result, RecentBookSummary{
			BookID:     book.ID,
			Title:      book.Title,
			AuthorName: getAuthorName(ctx, s.store, book),
			CoverPath:  getBookCoverPath(book),
			FinishedAt: progress.FinishedAt,
		})

		if len(result) >= limit {
			break
		}
	}

	return result, nil
}

// getPublicLenses returns a user's lenses.
// Note: Currently returns all lenses for the user. A future IsPublic field could enable filtering.
func (s *ProfileService) getPublicLenses(ctx context.Context, userID string) ([]LensSummary, error) {
	lenses, err := s.store.ListLensesByOwner(ctx, userID)
	if err != nil {
		return nil, err
	}

	result := make([]LensSummary, 0, len(lenses))
	for _, lens := range lenses {
		result = append(result, LensSummary{
			ID:        lens.ID,
			Name:      lens.Name,
			BookCount: len(lens.BookIDs),
		})
	}

	return result, nil
}

// broadcastProfileUpdate sends an SSE event for profile changes.
func (s *ProfileService) broadcastProfileUpdate(ctx context.Context, userID string, profile *domain.UserProfile) {
	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		s.logger.Warn("failed to get user for SSE event", "user_id", userID, "error", err)
		return
	}

	s.sseManager.Emit(sse.NewProfileUpdatedEvent(sse.ProfileUpdatedEventData{
		UserID:      userID,
		FirstName:   user.FirstName,
		LastName:    user.LastName,
		AvatarType:  string(profile.AvatarType),
		AvatarValue: profile.AvatarValue,
		AvatarColor: color.ForUser(userID),
		Tagline:     profile.Tagline,
	}))
}
