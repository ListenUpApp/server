package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/listenupapp/listenup-server/internal/store"
)

// ListeningService handles listening events and playback progress.
type ListeningService struct {
	store  *store.Store
	events store.EventEmitter
	logger *slog.Logger
}

// NewListeningService creates a new listening service.
func NewListeningService(store *store.Store, events store.EventEmitter, logger *slog.Logger) *ListeningService {
	return &ListeningService{
		store:  store,
		events: events,
		logger: logger,
	}
}

// RecordEventRequest contains the data for recording a listening event.
type RecordEventRequest struct {
	BookID          string    `json:"book_id" validate:"required"`
	StartPositionMs int64     `json:"start_position_ms" validate:"gte=0"`
	EndPositionMs   int64     `json:"end_position_ms" validate:"gtfield=StartPositionMs"`
	StartedAt       time.Time `json:"started_at" validate:"required"`
	EndedAt         time.Time `json:"ended_at" validate:"required"`
	PlaybackSpeed   float32   `json:"playback_speed" validate:"gt=0,lte=4"`
	DeviceID        string    `json:"device_id" validate:"required"`
	DeviceName      string    `json:"device_name"`
}

// RecordEventResponse contains the created event and updated progress.
type RecordEventResponse struct {
	Event    *domain.ListeningEvent   `json:"event"`
	Progress *domain.PlaybackProgress `json:"progress"`
}

// RecordEvent records a listening event and updates progress.
func (s *ListeningService) RecordEvent(ctx context.Context, userID string, req RecordEventRequest) (*RecordEventResponse, error) {
	// Validate request
	if err := validate.Struct(req); err != nil {
		return nil, formatValidationError(err)
	}

	// Get book to verify it exists and get duration
	book, err := s.store.GetBook(ctx, req.BookID, userID)
	if err != nil {
		return nil, fmt.Errorf("book not found: %w", err)
	}

	// Generate event ID
	eventID, err := id.Generate("evt")
	if err != nil {
		return nil, fmt.Errorf("generate event ID: %w", err)
	}

	// Create event
	event := domain.NewListeningEvent(
		eventID,
		userID,
		req.BookID,
		req.StartPositionMs,
		req.EndPositionMs,
		req.StartedAt,
		req.EndedAt,
		req.PlaybackSpeed,
		req.DeviceID,
		req.DeviceName,
	)

	// Store event
	if err := s.store.CreateListeningEvent(ctx, event); err != nil {
		return nil, fmt.Errorf("store event: %w", err)
	}

	// Get or create progress
	progress, err := s.store.GetProgress(ctx, userID, req.BookID)
	if err != nil && !errors.Is(err, store.ErrProgressNotFound) {
		return nil, fmt.Errorf("get progress: %w", err)
	}

	if progress == nil {
		// First event for this book
		progress = domain.NewPlaybackProgress(event, book.TotalDuration)
	} else {
		// Update existing progress
		progress.UpdateFromEvent(event, book.TotalDuration)
	}

	// Store progress
	if err := s.store.UpsertProgress(ctx, progress); err != nil {
		return nil, fmt.Errorf("store progress: %w", err)
	}

	s.logger.Debug("recorded listening event",
		"event_id", event.ID,
		"user_id", userID,
		"book_id", req.BookID,
		"duration_ms", event.DurationMs,
		"progress", progress.Progress,
	)

	return &RecordEventResponse{
		Event:    event,
		Progress: progress,
	}, nil
}

// GetProgress retrieves playback progress for a specific book.
func (s *ListeningService) GetProgress(ctx context.Context, userID, bookID string) (*domain.PlaybackProgress, error) {
	progress, err := s.store.GetProgress(ctx, userID, bookID)
	if err != nil {
		return nil, err
	}
	return progress, nil
}

// GetContinueListening returns in-progress books with book details for display.
// Returns a display-ready response that doesn't require client-side joins.
func (s *ListeningService) GetContinueListening(ctx context.Context, userID string, limit int) ([]*domain.ContinueListeningItem, error) {
	if limit <= 0 {
		limit = 10 // Default limit
	}

	// Get progress entries
	progressList, err := s.store.GetContinueListening(ctx, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("get progress: %w", err)
	}

	// Enrich with book details
	// Use GetBookNoAccessCheck because if a user has progress on a book,
	// they should see it in continue listening regardless of current collection access
	// (they had access when they listened to it)
	items := make([]*domain.ContinueListeningItem, 0, len(progressList))
	for _, progress := range progressList {
		// Fetch book details without access check
		book, err := s.store.GetBookNoAccessCheck(ctx, progress.BookID)
		if err != nil {
			s.logger.Warn("Book not found for progress", "book_id", progress.BookID, "error", err)
			continue // Skip items where book is missing
		}

		// Get author name by looking up contributor IDs with author role
		authorName := s.getAuthorName(ctx, book)

		// Get cover path and BlurHash from CoverImage if present
		var coverPath *string
		var coverBlurHash *string
		if book.CoverImage != nil {
			if book.CoverImage.Path != "" {
				coverPath = &book.CoverImage.Path
			}
			if book.CoverImage.BlurHash != "" {
				coverBlurHash = &book.CoverImage.BlurHash
			}
		}

		items = append(items, &domain.ContinueListeningItem{
			BookID:            progress.BookID,
			CurrentPositionMs: progress.CurrentPositionMs,
			Progress:          progress.Progress,
			LastPlayedAt:      progress.LastPlayedAt,
			Title:             book.Title,
			AuthorName:        authorName,
			CoverPath:         coverPath,
			CoverBlurHash:     coverBlurHash,
			TotalDurationMs:   book.TotalDuration,
		})
	}

	return items, nil
}

// getAuthorName extracts author name(s) from book contributors.
func (s *ListeningService) getAuthorName(ctx context.Context, book *domain.Book) string {
	// Collect author contributor IDs
	var authorIDs []string
	for _, contrib := range book.Contributors {
		for _, role := range contrib.Roles {
			if role == domain.RoleAuthor {
				authorIDs = append(authorIDs, contrib.ContributorID)
				break
			}
		}
	}

	if len(authorIDs) == 0 {
		return ""
	}

	// Fetch contributor details
	contributors, err := s.store.GetContributorsByIDs(ctx, authorIDs)
	if err != nil || len(contributors) == 0 {
		return ""
	}

	// Build author string
	authorName := contributors[0].Name
	if len(contributors) > 1 {
		authorName += " et al."
	}
	return authorName
}

// ResetProgress removes all progress for a user+book.
func (s *ListeningService) ResetProgress(ctx context.Context, userID, bookID string) error {
	return s.store.DeleteProgress(ctx, userID, bookID)
}

// GetUserSettings retrieves user playback settings.
func (s *ListeningService) GetUserSettings(ctx context.Context, userID string) (*domain.UserSettings, error) {
	return s.store.GetOrCreateUserSettings(ctx, userID)
}

// UpdateUserSettingsRequest contains fields that can be updated.
type UpdateUserSettingsRequest struct {
	DefaultPlaybackSpeed   *float32 `json:"default_playback_speed" validate:"omitempty,gt=0,lte=4"`
	DefaultSkipForwardSec  *int     `json:"default_skip_forward_sec" validate:"omitempty,gte=5,lte=300"`
	DefaultSkipBackwardSec *int     `json:"default_skip_backward_sec" validate:"omitempty,gte=5,lte=300"`
	DefaultSleepTimerMin   *int     `json:"default_sleep_timer_min" validate:"omitempty,gte=1,lte=480"`
	ShakeToResetSleepTimer *bool    `json:"shake_to_reset_sleep_timer"`
}

// UpdateUserSettings updates user playback settings.
func (s *ListeningService) UpdateUserSettings(ctx context.Context, userID string, req UpdateUserSettingsRequest) (*domain.UserSettings, error) {
	if err := validate.Struct(req); err != nil {
		return nil, formatValidationError(err)
	}

	settings, err := s.store.GetOrCreateUserSettings(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Apply updates
	if req.DefaultPlaybackSpeed != nil {
		settings.DefaultPlaybackSpeed = *req.DefaultPlaybackSpeed
	}
	if req.DefaultSkipForwardSec != nil {
		settings.DefaultSkipForwardSec = *req.DefaultSkipForwardSec
	}
	if req.DefaultSkipBackwardSec != nil {
		settings.DefaultSkipBackwardSec = *req.DefaultSkipBackwardSec
	}
	if req.DefaultSleepTimerMin != nil {
		settings.DefaultSleepTimerMin = req.DefaultSleepTimerMin
	}
	if req.ShakeToResetSleepTimer != nil {
		settings.ShakeToResetSleepTimer = *req.ShakeToResetSleepTimer
	}

	settings.UpdatedAt = time.Now()

	if err := s.store.UpsertUserSettings(ctx, settings); err != nil {
		return nil, err
	}

	return settings, nil
}

// GetBookPreferences retrieves per-book preferences.
func (s *ListeningService) GetBookPreferences(ctx context.Context, userID, bookID string) (*domain.BookPreferences, error) {
	prefs, err := s.store.GetBookPreferences(ctx, userID, bookID)
	if errors.Is(err, store.ErrBookPreferencesNotFound) {
		// Return empty preferences (all defaults)
		return domain.NewBookPreferences(userID, bookID), nil
	}
	if err != nil {
		return nil, err
	}
	if prefs == nil {
		// Return empty preferences (all defaults)
		return domain.NewBookPreferences(userID, bookID), nil
	}
	return prefs, nil
}

// UpdateBookPreferencesRequest contains fields that can be updated.
type UpdateBookPreferencesRequest struct {
	PlaybackSpeed             *float32 `json:"playback_speed" validate:"omitempty,gt=0,lte=4"`
	SkipForwardSec            *int     `json:"skip_forward_sec" validate:"omitempty,gte=5,lte=300"`
	HideFromContinueListening *bool    `json:"hide_from_continue_listening"`
}

// UpdateBookPreferences updates per-book preferences.
func (s *ListeningService) UpdateBookPreferences(ctx context.Context, userID, bookID string, req UpdateBookPreferencesRequest) (*domain.BookPreferences, error) {
	if err := validate.Struct(req); err != nil {
		return nil, formatValidationError(err)
	}

	prefs, err := s.store.GetBookPreferences(ctx, userID, bookID)
	if errors.Is(err, store.ErrBookPreferencesNotFound) {
		prefs = nil
		err = nil
	}
	if err != nil {
		return nil, err
	}
	if prefs == nil {
		prefs = domain.NewBookPreferences(userID, bookID)
	}

	// Apply updates
	if req.PlaybackSpeed != nil {
		prefs.PlaybackSpeed = req.PlaybackSpeed
	}
	if req.SkipForwardSec != nil {
		prefs.SkipForwardSec = req.SkipForwardSec
	}
	if req.HideFromContinueListening != nil {
		prefs.HideFromContinueListening = *req.HideFromContinueListening
	}

	prefs.UpdatedAt = time.Now()

	if err := s.store.UpsertBookPreferences(ctx, prefs); err != nil {
		return nil, err
	}

	return prefs, nil
}

// UserStats contains listening statistics for a user.
type UserStats struct {
	TotalListenTimeMs int64 `json:"total_listen_time_ms"`
	BooksStarted      int   `json:"books_started"`
	BooksFinished     int   `json:"books_finished"`
}

// GetUserStats calculates listening statistics for a user.
func (s *ListeningService) GetUserStats(ctx context.Context, userID string) (*UserStats, error) {
	allProgress, err := s.store.GetProgressForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	stats := &UserStats{}
	for _, p := range allProgress {
		stats.TotalListenTimeMs += p.TotalListenTimeMs
		stats.BooksStarted++
		if p.IsFinished {
			stats.BooksFinished++
		}
	}

	return stats, nil
}

// BookStats contains listening statistics for a book.
type BookStats struct {
	TotalListenTimeMs int64 `json:"total_listen_time_ms"`
	TotalListeners    int   `json:"total_listeners"`
	CompletedCount    int   `json:"completed_count"`
}

// GetBookStats calculates listening statistics for a book.
func (s *ListeningService) GetBookStats(ctx context.Context, bookID string) (*BookStats, error) {
	events, err := s.store.GetEventsForBook(ctx, bookID)
	if err != nil {
		return nil, err
	}

	stats := &BookStats{}
	listeners := make(map[string]bool)
	finished := make(map[string]bool)

	for _, e := range events {
		stats.TotalListenTimeMs += e.DurationMs
		listeners[e.UserID] = true
	}

	stats.TotalListeners = len(listeners)

	// Check completion status by looking at progress
	for userID := range listeners {
		progress, err := s.store.GetProgress(ctx, userID, bookID)
		if err == nil && progress != nil && progress.IsFinished {
			finished[userID] = true
		}
	}

	stats.CompletedCount = len(finished)

	return stats, nil
}
