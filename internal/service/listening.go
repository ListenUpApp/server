package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/listenupapp/listenup-server/internal/color"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
)

// MilestoneRecorder records milestone activities.
// This avoids a circular dependency between ListeningService and ActivityService.
type MilestoneRecorder interface {
	RecordStreakMilestone(ctx context.Context, userID string, days int) error
	RecordListeningMilestone(ctx context.Context, userID string, hours int) error
}

// StreakCalculator calculates user streaks.
// This avoids a circular dependency between ListeningService and SocialService.
type StreakCalculator interface {
	CalculateUserStreak(ctx context.Context, userID string) int
}

// ListeningService handles listening events and playback progress.
type ListeningService struct {
	store                 *store.Store
	events                store.EventEmitter
	readingSessionService *ReadingSessionService
	logger                *slog.Logger
	milestoneRecorder     MilestoneRecorder
	streakCalculator      StreakCalculator
}

// NewListeningService creates a new listening service.
func NewListeningService(store *store.Store, events store.EventEmitter, readingSessionService *ReadingSessionService, logger *slog.Logger) *ListeningService {
	return &ListeningService{
		store:                 store,
		events:                events,
		readingSessionService: readingSessionService,
		logger:                logger,
	}
}

// SetMilestoneRecorder sets the milestone recorder for recording activity milestones.
// This is set after construction to avoid circular dependencies.
func (s *ListeningService) SetMilestoneRecorder(recorder MilestoneRecorder) {
	s.milestoneRecorder = recorder
}

// SetStreakCalculator sets the streak calculator for computing user streaks.
// This is set after construction to avoid circular dependencies.
func (s *ListeningService) SetStreakCalculator(calculator StreakCalculator) {
	s.streakCalculator = calculator
}

// RecordEventRequest contains the data for recording a listening event.
type RecordEventRequest struct {
	EventID         string    `json:"event_id"` // Client-provided ID for idempotency
	BookID          string    `json:"book_id" validate:"required"`
	StartPositionMs int64     `json:"start_position_ms" validate:"gte=0"`
	EndPositionMs   int64     `json:"end_position_ms" validate:"gtfield=StartPositionMs"`
	StartedAt       time.Time `json:"started_at" validate:"required"`
	EndedAt         time.Time `json:"ended_at" validate:"required"`
	PlaybackSpeed   float32   `json:"playback_speed" validate:"gt=0,lte=4"`
	DeviceID        string    `json:"device_id" validate:"required"`
	DeviceName      string    `json:"device_name"`
	Source          string    `json:"source"` // playback, import, or manual (defaults to playback)
}

// RecordEventResponse contains the created event and updated progress.
type RecordEventResponse struct {
	Event    *domain.ListeningEvent `json:"event"`
	Progress *domain.PlaybackState  `json:"progress"`
}

// RecordEvent records a listening event and updates progress.
// If EventID is provided, this operation is idempotent - calling it multiple times
// with the same EventID will only create the event once.
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

	// Use client-provided ID if set, otherwise generate one
	var eventID string
	if req.EventID != "" {
		eventID = req.EventID

		// Idempotency check: if event already exists, return success
		existing, err := s.store.GetListeningEvent(ctx, eventID)
		if err == nil && existing != nil {
			// Event already exists - return it (idempotent)
			s.logger.Debug("listening event already exists (idempotent)",
				"event_id", eventID,
				"user_id", userID,
			)
			// Still need to get state for the response
			progress, _ := s.store.GetState(ctx, userID, req.BookID)
			return &RecordEventResponse{
				Event:    existing,
				Progress: progress,
			}, nil
		}
	} else {
		// No client ID - generate one
		var err error
		eventID, err = id.Generate("evt")
		if err != nil {
			return nil, fmt.Errorf("generate event ID: %w", err)
		}
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

	// Override source if provided (defaults to playback)
	if req.Source != "" {
		event.Source = req.Source
	}

	// Store event
	if err := s.store.CreateListeningEvent(ctx, event); err != nil {
		return nil, fmt.Errorf("store event: %w", err)
	}

	// Get or create state
	existingProgress, err := s.store.GetState(ctx, userID, req.BookID)
	if err != nil && !errors.Is(err, store.ErrProgressNotFound) {
		return nil, fmt.Errorf("get state: %w", err)
	}

	// Track if book was previously finished
	wasFinished := existingProgress != nil && existingProgress.IsFinished

	var progress *domain.PlaybackState
	if existingProgress == nil {
		// First event for this book
		progress = domain.NewPlaybackState(event, book.TotalDuration)
	} else {
		// Update existing progress
		progress = existingProgress
		progress.UpdateFromEvent(event, book.TotalDuration)
	}

	// Store state
	if err := s.store.UpsertState(ctx, progress); err != nil {
		return nil, fmt.Errorf("store state: %w", err)
	}

	// Track reading session (non-blocking - don't fail if session operations fail)
	if _, err := s.readingSessionService.EnsureActiveSession(ctx, userID, req.BookID); err != nil {
		s.logger.Warn("failed to ensure reading session", "error", err, "user_id", userID, "book_id", req.BookID)
	}

	if err := s.readingSessionService.UpdateSessionProgress(ctx, userID, req.BookID, progress.TotalListenTimeMs); err != nil {
		s.logger.Warn("failed to update session progress", "error", err, "user_id", userID, "book_id", req.BookID)
	}

	// Check if just completed (99%+)
	if progress.IsFinished && !wasFinished {
		if err := s.readingSessionService.CompleteSession(ctx, userID, req.BookID, progress.ComputeProgress(book.TotalDuration), progress.TotalListenTimeMs); err != nil {
			s.logger.Warn("failed to complete session", "error", err, "user_id", userID, "book_id", req.BookID)
		}
	}

	s.logger.Debug("recorded listening event",
		"event_id", event.ID,
		"user_id", userID,
		"book_id", req.BookID,
		"duration_ms", event.DurationMs,
		"progress", progress.ComputeProgress(book.TotalDuration),
	)

	// Emit SSE events so other devices and UI can update
	s.events.Emit(sse.NewProgressUpdatedEvent(userID, progress, book.TotalDuration))
	s.events.Emit(sse.NewListeningEventCreatedEvent(userID, event))

	// Check for milestone crossings (non-blocking)
	s.checkMilestones(ctx, userID, progress.TotalListenTimeMs)

	// Broadcast updated user stats for leaderboard caching (non-blocking)
	s.broadcastUserStatsUpdate(ctx, userID)

	// Note: listening_session activities are created when the reading session ends
	// (CompleteSession or AbandonSession), not per-event, to avoid spamming the feed.

	return &RecordEventResponse{
		Event:    event,
		Progress: progress,
	}, nil
}

// checkMilestones checks for and records any milestone crossings.
// This is non-blocking and logs errors instead of returning them.
func (s *ListeningService) checkMilestones(ctx context.Context, userID string, newTotalListenTimeMs int64) {
	if s.milestoneRecorder == nil {
		return
	}

	// Get previous milestone state
	prevState, err := s.store.GetUserMilestoneState(ctx, userID)
	if err != nil {
		s.logger.Warn("failed to get milestone state", "user_id", userID, "error", err)
		return
	}

	// Initialize if nil (first time tracking)
	var prevStreakDays, prevListenHours int
	if prevState != nil {
		prevStreakDays = prevState.LastStreakDays
		prevListenHours = prevState.LastListenHoursTotal
	}

	// Calculate current values
	currentListenHours := int(newTotalListenTimeMs / (1000 * 60 * 60))

	var currentStreak int
	if s.streakCalculator != nil {
		currentStreak = s.streakCalculator.CalculateUserStreak(ctx, userID)
	}

	// Check for streak milestone
	if currentStreak > prevStreakDays && domain.IsStreakMilestone(currentStreak) {
		if err := s.milestoneRecorder.RecordStreakMilestone(ctx, userID, currentStreak); err != nil {
			s.logger.Warn("failed to record streak milestone",
				"user_id", userID,
				"days", currentStreak,
				"error", err)
		}
	}

	// Check for listening hours milestone
	if crossed, hours := domain.CrossedListeningMilestone(prevListenHours, currentListenHours); crossed {
		if err := s.milestoneRecorder.RecordListeningMilestone(ctx, userID, hours); err != nil {
			s.logger.Warn("failed to record listening milestone",
				"user_id", userID,
				"hours", hours,
				"error", err)
		}
	}

	// Update milestone state
	if err := s.store.UpdateUserMilestoneState(ctx, userID, currentStreak, currentListenHours); err != nil {
		s.logger.Warn("failed to update milestone state",
			"user_id", userID,
			"error", err)
	}
}

// GetProgress retrieves playback state for a specific book.
func (s *ListeningService) GetProgress(ctx context.Context, userID, bookID string) (*domain.PlaybackState, error) {
	progress, err := s.store.GetState(ctx, userID, bookID)
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
			Progress:          progress.ComputeProgress(book.TotalDuration),
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
// Delegates to the shared getAuthorName function.
func (s *ListeningService) getAuthorName(ctx context.Context, book *domain.Book) string {
	return getAuthorName(ctx, s.store, book)
}

// ResetProgress removes all progress for a user+book.
func (s *ListeningService) ResetProgress(ctx context.Context, userID, bookID string) error {
	// Abandon active reading session before resetting progress
	if err := s.readingSessionService.AbandonSession(ctx, userID, bookID); err != nil {
		s.logger.Warn("failed to abandon session on reset", "error", err, "user_id", userID, "book_id", bookID)
		// Continue with reset even if abandon fails
	}

	return s.store.DeleteState(ctx, userID, bookID)
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
	allProgress, err := s.store.GetStateForUser(ctx, userID)
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

	// Check completion status by looking at state
	for userID := range listeners {
		progress, err := s.store.GetState(ctx, userID, bookID)
		if err == nil && progress != nil && progress.IsFinished {
			finished[userID] = true
		}
	}

	stats.CompletedCount = len(finished)

	return stats, nil
}

// MarkComplete marks a book as finished regardless of current position.
// This allows users to mark a book as complete manually (e.g., DNF at 90%).
func (s *ListeningService) MarkComplete(ctx context.Context, userID, bookID string, startedAt, finishedAt *time.Time) (*domain.PlaybackState, error) {
	// Get book for duration (no access check - if user is marking complete, they had access)
	book, err := s.store.GetBookNoAccessCheck(ctx, bookID)
	if err != nil {
		return nil, fmt.Errorf("book not found: %w", err)
	}

	state, err := s.store.GetState(ctx, userID, bookID)
	if err != nil && !errors.Is(err, store.ErrProgressNotFound) {
		return nil, fmt.Errorf("getting state: %w", err)
	}

	now := time.Now()
	if finishedAt == nil {
		finishedAt = &now
	}

	if state == nil {
		// Create new state if none exists
		startedAtValue := now
		if startedAt != nil {
			startedAtValue = *startedAt
		}
		state = &domain.PlaybackState{
			UserID:            userID,
			BookID:            bookID,
			CurrentPositionMs: 0,
			IsFinished:        true,
			FinishedAt:        finishedAt,
			StartedAt:         startedAtValue,
			LastPlayedAt:      now,
			UpdatedAt:         now,
		}
	} else {
		state.IsFinished = true
		state.FinishedAt = finishedAt
		if startedAt != nil {
			state.StartedAt = *startedAt
		}
		state.UpdatedAt = now
	}

	if err := s.store.UpsertState(ctx, state); err != nil {
		return nil, fmt.Errorf("saving state: %w", err)
	}

	// Complete any active reading session
	if err := s.readingSessionService.CompleteSession(ctx, userID, bookID, state.ComputeProgress(book.TotalDuration), state.TotalListenTimeMs); err != nil {
		s.logger.Warn("failed to complete session on mark complete", "error", err, "user_id", userID, "book_id", bookID)
	}

	s.logger.Info("marked book as complete",
		"user_id", userID,
		"book_id", bookID,
		"finished_at", finishedAt,
	)

	// Emit SSE event
	s.events.Emit(sse.NewProgressUpdatedEvent(userID, state, book.TotalDuration))

	return state, nil
}

// DiscardProgress removes playback state for a book.
// If keepHistory is true (default), listening events are preserved.
func (s *ListeningService) DiscardProgress(ctx context.Context, userID, bookID string, keepHistory bool) error {
	// Abandon active reading session before discarding progress
	if err := s.readingSessionService.AbandonSession(ctx, userID, bookID); err != nil {
		s.logger.Warn("failed to abandon session on discard", "error", err, "user_id", userID, "book_id", bookID)
		// Continue with discard even if abandon fails
	}

	// Delete state
	if err := s.store.DeleteState(ctx, userID, bookID); err != nil {
		return fmt.Errorf("deleting state: %w", err)
	}

	// Optionally delete events (rare - GDPR style purge)
	if !keepHistory {
		if err := s.store.DeleteEventsForUserBook(ctx, userID, bookID); err != nil {
			return fmt.Errorf("deleting events: %w", err)
		}
	}

	s.logger.Info("discarded progress",
		"user_id", userID,
		"book_id", bookID,
		"keep_history", keepHistory,
	)

	// Emit SSE event
	s.events.Emit(sse.NewProgressDeletedEvent(userID, bookID))

	return nil
}

// RestartBook resets a book to listen again from the beginning.
// Preserves history but clears current position and finished status.
func (s *ListeningService) RestartBook(ctx context.Context, userID, bookID string) (*domain.PlaybackState, error) {
	// Get book for duration (no access check - if user is restarting, they had access)
	book, err := s.store.GetBookNoAccessCheck(ctx, bookID)
	if err != nil {
		return nil, fmt.Errorf("book not found: %w", err)
	}

	state, err := s.store.GetState(ctx, userID, bookID)
	if err != nil && !errors.Is(err, store.ErrProgressNotFound) {
		return nil, fmt.Errorf("getting state: %w", err)
	}

	now := time.Now()

	if state == nil {
		state = &domain.PlaybackState{
			UserID:    userID,
			BookID:    bookID,
			StartedAt: now,
		}
	}

	// Preserve original StartedAt if it exists
	state.CurrentPositionMs = 0
	state.IsFinished = false
	state.FinishedAt = nil
	state.LastPlayedAt = now
	state.UpdatedAt = now

	if err := s.store.UpsertState(ctx, state); err != nil {
		return nil, fmt.Errorf("saving state: %w", err)
	}

	// This will create a new reading session (marked as re-read if previously completed)
	if _, err := s.readingSessionService.EnsureActiveSession(ctx, userID, bookID); err != nil {
		s.logger.Warn("failed to create session on restart", "error", err, "user_id", userID, "book_id", bookID)
	}

	s.logger.Info("restarted book",
		"user_id", userID,
		"book_id", bookID,
	)

	// Emit SSE event
	s.events.Emit(sse.NewProgressUpdatedEvent(userID, state, book.TotalDuration))

	return state, nil
}

// broadcastUserStatsUpdate broadcasts updated all-time stats for a user.
// Called after listening events to keep leaderboard caches fresh.
func (s *ListeningService) broadcastUserStatsUpdate(ctx context.Context, userID string) {
	// Get user info
	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		s.logger.Warn("failed to get user for stats broadcast", "user_id", userID, "error", err)
		return
	}

	// Get user profile for avatar info
	profile, _ := s.store.GetUserProfile(ctx, userID)
	avatarType := string(domain.AvatarTypeAuto)
	avatarValue := ""
	if profile != nil {
		avatarType = string(profile.AvatarType)
		avatarValue = profile.AvatarValue
	}

	// Calculate total listening time from all events
	allEvents, err := s.store.GetEventsForUser(ctx, userID)
	if err != nil {
		s.logger.Warn("failed to get events for stats broadcast", "user_id", userID, "error", err)
		return
	}

	// Only count valid durations (same logic as leaderboard calculation)
	const maxReasonableDurationMs = 24 * 60 * 60 * 1000 // 24 hours
	var totalTimeMs int64
	for _, e := range allEvents {
		if e.DurationMs > 0 && e.DurationMs <= maxReasonableDurationMs {
			totalTimeMs += e.DurationMs
		}
	}

	// Get finished books count
	allProgress, err := s.store.GetStateForUser(ctx, userID)
	if err != nil {
		s.logger.Warn("failed to get progress for stats broadcast", "user_id", userID, "error", err)
		return
	}

	var totalBooks int
	for _, p := range allProgress {
		if p.IsFinished {
			totalBooks++
		}
	}

	// Get current streak
	var currentStreak int
	if s.streakCalculator != nil {
		currentStreak = s.streakCalculator.CalculateUserStreak(ctx, userID)
	}

	// Broadcast the update
	s.events.Emit(sse.NewUserStatsUpdatedEvent(sse.UserStatsUpdatedEventData{
		UserID:        userID,
		DisplayName:   user.DisplayName,
		AvatarType:    avatarType,
		AvatarValue:   avatarValue,
		AvatarColor:   color.ForUser(userID),
		TotalTimeMs:   totalTimeMs,
		TotalBooks:    totalBooks,
		CurrentStreak: currentStreak,
	}))

	s.logger.Debug("broadcast user stats update",
		"user_id", userID,
		"total_time_ms", totalTimeMs,
		"total_books", totalBooks,
		"current_streak", currentStreak,
	)
}
