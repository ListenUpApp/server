package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/listenupapp/listenup-server/internal/color"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
)

// ActivityRecorder is the interface for recording activities.
// This avoids a circular dependency between ReadingSessionService and ActivityService.
type ActivityRecorder interface {
	RecordBookStarted(ctx context.Context, userID, bookID string, isReread bool) error
	RecordBookFinished(ctx context.Context, userID, bookID string) error
	RecordListeningSession(ctx context.Context, userID, bookID string, durationMs int64) error
}

// ReadingSessionService manages book reading sessions - tracking when users start and complete books.
type ReadingSessionService struct {
	store            *store.Store
	events           store.EventEmitter
	logger           *slog.Logger
	activityRecorder ActivityRecorder
}

// NewReadingSessionService creates a new reading session service.
func NewReadingSessionService(store *store.Store, events store.EventEmitter, logger *slog.Logger) *ReadingSessionService {
	return &ReadingSessionService{
		store:  store,
		events: events,
		logger: logger,
	}
}

// SetActivityRecorder sets the activity recorder for recording social activities.
// This is set after construction to avoid circular dependencies.
func (s *ReadingSessionService) SetActivityRecorder(recorder ActivityRecorder) {
	s.activityRecorder = recorder
}

// EnsureActiveSession gets the active session for user+book.
// Creates a new session if none exists or if the existing one is stale (>6 months).
// If stale, marks the old session as abandoned first.
func (s *ReadingSessionService) EnsureActiveSession(ctx context.Context, userID, bookID string) (*domain.BookReadingSession, error) {
	// Get active session
	session, err := s.store.GetActiveSession(ctx, userID, bookID)
	if err != nil {
		s.logger.Error("failed to get active session",
			"user_id", userID,
			"book_id", bookID,
			"error", err)
		return nil, fmt.Errorf("get active session: %w", err)
	}

	s.logger.Debug("checked for active session",
		"user_id", userID,
		"book_id", bookID,
		"has_session", session != nil,
		"session_id", func() string {
			if session != nil {
				return session.ID
			}
			return ""
		}())

	now := time.Now()
	wasStale := false

	// If we have an active session, check if it's stale
	if session != nil {
		if session.IsStale(now) {
			s.logger.Info("abandoning stale session",
				"session_id", session.ID,
				"user_id", userID,
				"book_id", bookID,
				"started_at", session.StartedAt)
			// Mark as abandoned before creating new one
			if err := s.abandonSessionInternal(ctx, session); err != nil {
				s.logger.Warn("failed to abandon stale session",
					"session_id", session.ID,
					"user_id", userID,
					"book_id", bookID,
					"error", err)
			}
			// Continue to create new session below
			session = nil
			wasStale = true
		} else {
			// Active session is fresh, return it
			s.logger.Debug("returning existing active session",
				"session_id", session.ID,
				"user_id", userID,
				"book_id", bookID)
			return session, nil
		}
	}

	// Before creating new session, check previous sessions to determine if this is a re-read
	// Get all sessions for this user+book to check history
	previousSessions, err := s.store.GetUserBookSessions(ctx, userID, bookID)
	if err != nil {
		s.logger.Warn("failed to get previous sessions for activity",
			"user_id", userID,
			"book_id", bookID,
			"error", err)
		// Continue anyway - activity recording is not critical
		previousSessions = nil
	}

	// Check if user has completed this book before
	hasCompletedBefore := slices.ContainsFunc(previousSessions, func(sess *domain.BookReadingSession) bool {
		return sess.IsCompleted
	})

	// No active session (or was stale), create new one
	sessionID, err := id.Generate("rsession")
	if err != nil {
		return nil, fmt.Errorf("generate session ID: %w", err)
	}

	newSession := domain.NewBookReadingSession(sessionID, userID, bookID)
	if err := s.store.CreateReadingSession(ctx, newSession); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	s.logger.Debug("created new reading session",
		"session_id", newSession.ID,
		"user_id", userID,
		"book_id", bookID)

	// Emit SSE event for new session (user-specific, for their other devices)
	s.events.Emit(sse.NewReadingSessionUpdatedEvent(newSession))

	// Emit session.started event broadcast to ALL users for "Currently Listening" feature
	s.events.Emit(sse.NewSessionStartedEvent(newSession.ID, userID, bookID, newSession.StartedAt))

	// Record activity for new session
	// Only fire activity if:
	// - First-ever session (no previous sessions) → started_book
	// - New session after completed session → started_book (re-read)
	// - New session after abandoned/stale session → skip (not interesting)
	if s.activityRecorder != nil {
		s.logger.Info("evaluating started_book activity",
			"user_id", userID,
			"book_id", bookID,
			"previous_sessions_count", len(previousSessions),
			"has_completed_before", hasCompletedBefore,
			"was_stale", wasStale)

		if len(previousSessions) == 0 {
			// First-ever session for this user+book
			s.logger.Info("recording first-time started_book activity",
				"user_id", userID,
				"book_id", bookID)
			if err := s.activityRecorder.RecordBookStarted(ctx, userID, bookID, false); err != nil {
				s.logger.Warn("failed to record started activity",
					"user_id", userID,
					"book_id", bookID,
					"error", err)
			}
		} else if hasCompletedBefore && !wasStale {
			// Re-read: user completed before and is starting again
			// (but not if this is just resuming after a stale session)
			s.logger.Info("recording re-read started_book activity",
				"user_id", userID,
				"book_id", bookID)
			if err := s.activityRecorder.RecordBookStarted(ctx, userID, bookID, true); err != nil {
				s.logger.Warn("failed to record re-read activity",
					"user_id", userID,
					"book_id", bookID,
					"error", err)
			}
		} else {
			s.logger.Debug("skipping started_book activity (not first or re-read)",
				"user_id", userID,
				"book_id", bookID,
				"previous_sessions_count", len(previousSessions),
				"has_completed_before", hasCompletedBefore,
				"was_stale", wasStale)
		}
	}

	return newSession, nil
}

// EndPlaybackSession records a listening_session activity when the user pauses/stops playback.
// durationMs is how long they listened in this play session (since pressing play).
func (s *ReadingSessionService) EndPlaybackSession(ctx context.Context, userID, bookID string, durationMs int64) error {
	if s.activityRecorder == nil {
		return nil
	}

	s.logger.Info("ending playback session",
		"user_id", userID,
		"book_id", bookID,
		"duration_ms", durationMs)

	if err := s.activityRecorder.RecordListeningSession(ctx, userID, bookID, durationMs); err != nil {
		return fmt.Errorf("record listening session: %w", err)
	}

	return nil
}

// UpdateSessionProgress updates the session's accumulated listen time.
func (s *ReadingSessionService) UpdateSessionProgress(ctx context.Context, userID, bookID string, listenTimeMs int64) error {
	// Get active session
	session, err := s.store.GetActiveSession(ctx, userID, bookID)
	if err != nil {
		return fmt.Errorf("get active session: %w", err)
	}
	if session == nil {
		return fmt.Errorf("no active session for user %s book %s", userID, bookID)
	}

	// Update progress
	session.UpdateProgress(listenTimeMs)

	// Save
	if err := s.store.UpdateReadingSession(ctx, session); err != nil {
		return fmt.Errorf("update session: %w", err)
	}

	return nil
}

// CompleteSession marks the session as completed (99%+ progress).
func (s *ReadingSessionService) CompleteSession(ctx context.Context, userID, bookID string, progress float64, listenTimeMs int64) error {
	// Get active session
	session, err := s.store.GetActiveSession(ctx, userID, bookID)
	if err != nil {
		return fmt.Errorf("get active session: %w", err)
	}
	if session == nil {
		return fmt.Errorf("no active session for user %s book %s", userID, bookID)
	}

	// Mark as completed
	session.MarkCompleted(progress, listenTimeMs)

	// Save
	if err := s.store.UpdateReadingSession(ctx, session); err != nil {
		return fmt.Errorf("update session: %w", err)
	}

	s.logger.Info("completed reading session",
		"session_id", session.ID,
		"user_id", userID,
		"book_id", bookID,
		"listen_time_ms", listenTimeMs)

	// Emit SSE event for completed session (user-specific)
	s.events.Emit(sse.NewReadingSessionUpdatedEvent(session))

	// Emit session.ended event broadcast to ALL users for "Currently Listening" feature
	s.events.Emit(sse.NewSessionEndedEvent(session.ID))

	// Record finished_book activity
	// Note: listening_session activity is recorded via EndPlaybackSession when the client pauses/stops
	if s.activityRecorder != nil {
		if err := s.activityRecorder.RecordBookFinished(ctx, userID, bookID); err != nil {
			s.logger.Warn("failed to record finished activity",
				"user_id", userID,
				"book_id", bookID,
				"error", err)
		}
	}

	return nil
}

// AbandonSession marks the session as abandoned (stopped before 99%).
func (s *ReadingSessionService) AbandonSession(ctx context.Context, userID, bookID string) error {
	// Get active session
	session, err := s.store.GetActiveSession(ctx, userID, bookID)
	if err != nil {
		return fmt.Errorf("get active session: %w", err)
	}
	if session == nil {
		// No active session, nothing to abandon
		return nil
	}

	return s.abandonSessionInternal(ctx, session)
}

// abandonSessionInternal abandons a specific session instance.
func (s *ReadingSessionService) abandonSessionInternal(ctx context.Context, session *domain.BookReadingSession) error {
	// Get book for duration (no access check - if user has a session, they had access)
	// If book doesn't exist (e.g., deleted), we still want to abandon the session
	var bookDurationMs int64
	book, err := s.store.GetBookNoAccessCheck(ctx, session.BookID)
	if err == nil && book != nil {
		bookDurationMs = book.TotalDuration
	}

	// Get current state from store
	progress, err := s.store.GetState(ctx, session.UserID, session.BookID)
	if err != nil && !errors.Is(err, store.ErrProgressNotFound) {
		return fmt.Errorf("get state: %w", err)
	}

	// Calculate final values
	var finalProgress float64
	var listenTimeMs int64
	if progress != nil {
		finalProgress = progress.ComputeProgress(bookDurationMs)
		listenTimeMs = progress.TotalListenTimeMs
	}

	// Mark as abandoned
	session.MarkAbandoned(finalProgress, listenTimeMs)

	// Save
	if err := s.store.UpdateReadingSession(ctx, session); err != nil {
		return fmt.Errorf("update session: %w", err)
	}

	s.logger.Debug("abandoned reading session",
		"session_id", session.ID,
		"user_id", session.UserID,
		"book_id", session.BookID,
		"final_progress", finalProgress)

	// Emit session.ended event broadcast to ALL users for "Currently Listening" feature
	s.events.Emit(sse.NewSessionEndedEvent(session.ID))

	return nil
}

// GetBookReaders retrieves all readers for a book with session summaries.
func (s *ReadingSessionService) GetBookReaders(ctx context.Context, bookID, viewingUserID string, limit int) (*BookReadersResponse, error) {
	// Get all sessions for this book
	allSessions, err := s.store.GetBookSessions(ctx, bookID)
	if err != nil {
		return nil, fmt.Errorf("get book sessions: %w", err)
	}

	// Group sessions by user
	sessionsByUser := make(map[string][]*domain.BookReadingSession)
	for _, session := range allSessions {
		sessionsByUser[session.UserID] = append(sessionsByUser[session.UserID], session)
	}

	// Separate viewing user's sessions
	viewerSessions := sessionsByUser[viewingUserID]
	delete(sessionsByUser, viewingUserID) // Remove from other readers

	// Build response
	response := &BookReadersResponse{
		YourSessions:     buildSessionSummaries(viewerSessions),
		OtherReaders:     []ReaderSummary{},
		TotalReaders:     len(sessionsByUser),
		TotalCompletions: 0,
	}

	// Add viewing user to total count if they have sessions
	if len(viewerSessions) > 0 {
		response.TotalReaders++
	}

	// Get book duration for progress calculation
	book, err := s.store.GetBookNoAccessCheck(ctx, bookID)
	var bookDurationMs int64
	if err == nil && book != nil {
		bookDurationMs = book.TotalDuration
	}

	// Build reader summaries for other users
	for userID, sessions := range sessionsByUser {
		// Get user info
		user, err := s.store.GetUser(ctx, userID)
		if err != nil {
			s.logger.Warn("failed to get user for reader summary",
				"user_id", userID,
				"book_id", bookID,
				"error", err)
			continue
		}

		// Get profile for avatar info (optional - defaults to auto if not found)
		var profile *domain.UserProfile
		if p, err := s.store.GetUserProfile(ctx, userID); err == nil {
			profile = p
		}

		// Get current progress from PlaybackState if user is actively reading
		var currentProgress float64
		hasActiveSession := false
		for _, session := range sessions {
			if session.IsActive() {
				hasActiveSession = true
				break
			}
		}
		if hasActiveSession && bookDurationMs > 0 {
			if state, err := s.store.GetState(ctx, userID, bookID); err == nil && state != nil {
				currentProgress = state.ComputeProgress(bookDurationMs)
			}
		}

		summary := buildReaderSummary(user, profile, sessions, currentProgress)
		response.OtherReaders = append(response.OtherReaders, summary)

		// Count completions
		if summary.CompletionCount > 0 {
			response.TotalCompletions += summary.CompletionCount
		}
	}

	// Count viewer's completions
	for _, session := range viewerSessions {
		if session.IsCompleted {
			response.TotalCompletions++
		}
	}

	// Sort other readers by most recent activity (last read date descending)
	slices.SortFunc(response.OtherReaders, func(a, b ReaderSummary) int {
		return b.LastActivityAt.Compare(a.LastActivityAt)
	})

	// Apply limit if specified
	if limit > 0 && len(response.OtherReaders) > limit {
		response.OtherReaders = response.OtherReaders[:limit]
	}

	return response, nil
}

// GetUserReadingHistory retrieves a user's reading history with book metadata.
func (s *ReadingSessionService) GetUserReadingHistory(ctx context.Context, userID string, limit int) (*UserReadingHistoryResponse, error) {
	// Get user's sessions
	sessions, err := s.store.GetUserReadingSessions(ctx, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("get user sessions: %w", err)
	}

	// Build response with enriched data
	historyItems := make([]ReadingHistorySession, 0, len(sessions))
	completedCount := 0

	for _, session := range sessions {
		// Get book metadata
		book, err := s.store.GetBookNoAccessCheck(ctx, session.BookID)
		if err != nil {
			s.logger.Warn("failed to get book for history session",
				"session_id", session.ID,
				"book_id", session.BookID,
				"error", err)
			continue
		}

		// Get author name
		authorName := getAuthorName(ctx, s.store, book)

		// Get cover info
		var coverPath string
		if book.CoverImage != nil && book.CoverImage.Path != "" {
			coverPath = book.CoverImage.Path
		}

		historyItems = append(historyItems, ReadingHistorySession{
			ID:           session.ID,
			BookID:       session.BookID,
			BookTitle:    book.Title,
			BookAuthor:   authorName,
			CoverPath:    coverPath,
			StartedAt:    session.StartedAt,
			FinishedAt:   session.FinishedAt,
			IsCompleted:  session.IsCompleted,
			ListenTimeMs: session.ListenTimeMs,
		})

		if session.IsCompleted {
			completedCount++
		}
	}

	return &UserReadingHistoryResponse{
		Sessions:       historyItems,
		TotalSessions:  len(historyItems),
		TotalCompleted: completedCount,
	}, nil
}

// Response types

// SessionSummary represents a single reading session.
type SessionSummary struct {
	ID           string     `json:"id"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	IsCompleted  bool       `json:"is_completed"`
	ListenTimeMs int64      `json:"listen_time_ms"`
}

// ReaderSummary represents a user who has read a book.
type ReaderSummary struct {
	UserID             string     `json:"user_id"`
	DisplayName        string     `json:"display_name"`
	AvatarType         string     `json:"avatar_type"`
	AvatarValue        string     `json:"avatar_value,omitempty"`
	AvatarColor        string     `json:"avatar_color"`
	IsCurrentlyReading bool       `json:"is_currently_reading"`
	CurrentProgress    float64    `json:"current_progress,omitempty"`
	StartedAt          time.Time  `json:"started_at"`
	FinishedAt         *time.Time `json:"finished_at,omitempty"`
	LastActivityAt     time.Time  `json:"last_activity_at"` // Most recent of StartedAt or FinishedAt
	CompletionCount    int        `json:"completion_count"`
}

// BookReadersResponse contains all readers of a book.
type BookReadersResponse struct {
	YourSessions     []SessionSummary `json:"your_sessions"`
	OtherReaders     []ReaderSummary  `json:"other_readers"`
	TotalReaders     int              `json:"total_readers"`
	TotalCompletions int              `json:"total_completions"`
}

// ReadingHistorySession represents a session with book metadata.
type ReadingHistorySession struct {
	ID           string     `json:"id"`
	BookID       string     `json:"book_id"`
	BookTitle    string     `json:"book_title"`
	BookAuthor   string     `json:"book_author"`
	CoverPath    string     `json:"cover_path,omitempty"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	IsCompleted  bool       `json:"is_completed"`
	ListenTimeMs int64      `json:"listen_time_ms"`
}

// UserReadingHistoryResponse contains a user's reading history.
type UserReadingHistoryResponse struct {
	Sessions       []ReadingHistorySession `json:"sessions"`
	TotalSessions  int                     `json:"total_sessions"`
	TotalCompleted int                     `json:"total_completed"`
}

// Helper functions

// buildSessionSummaries converts domain sessions to API summaries.
func buildSessionSummaries(sessions []*domain.BookReadingSession) []SessionSummary {
	summaries := make([]SessionSummary, 0, len(sessions))
	for _, session := range sessions {
		summaries = append(summaries, SessionSummary{
			ID:           session.ID,
			StartedAt:    session.StartedAt,
			FinishedAt:   session.FinishedAt,
			IsCompleted:  session.IsCompleted,
			ListenTimeMs: session.ListenTimeMs,
		})
	}
	return summaries
}

// buildReaderSummary creates a reader summary from user, profile, their sessions, and current progress.
// currentProgress is the real-time progress from PlaybackState (0.0-1.0), calculated by the caller.
func buildReaderSummary(user *domain.User, profile *domain.UserProfile, sessions []*domain.BookReadingSession, currentProgress float64) ReaderSummary {
	// Extract avatar info from profile (defaults to auto if no profile)
	avatarType := string(domain.AvatarTypeAuto)
	avatarValue := ""
	if profile != nil {
		avatarType = string(profile.AvatarType)
		avatarValue = profile.AvatarValue
	}

	// Guard against empty sessions
	if len(sessions) == 0 {
		return ReaderSummary{
			UserID:      user.ID,
			DisplayName: user.Name(),
			AvatarType:  avatarType,
			AvatarValue: avatarValue,
			AvatarColor: color.ForUser(user.ID),
		}
	}

	// Find most recent session
	var mostRecentSession *domain.BookReadingSession
	var activeSession *domain.BookReadingSession
	completionCount := 0

	for _, session := range sessions {
		if mostRecentSession == nil || session.StartedAt.After(mostRecentSession.StartedAt) {
			mostRecentSession = session
		}
		if session.IsActive() {
			activeSession = session
		}
		if session.IsCompleted {
			completionCount++
		}
	}

	// Compute last activity: use FinishedAt if available, otherwise StartedAt
	lastActivity := mostRecentSession.StartedAt
	if mostRecentSession.FinishedAt != nil && mostRecentSession.FinishedAt.After(lastActivity) {
		lastActivity = *mostRecentSession.FinishedAt
	}

	summary := ReaderSummary{
		UserID:             user.ID,
		DisplayName:        user.Name(),
		AvatarType:         avatarType,
		AvatarValue:        avatarValue,
		AvatarColor:        color.ForUser(user.ID),
		IsCurrentlyReading: activeSession != nil,
		StartedAt:          mostRecentSession.StartedAt,
		FinishedAt:         mostRecentSession.FinishedAt,
		LastActivityAt:     lastActivity,
		CompletionCount:    completionCount,
		CurrentProgress:    currentProgress, // Use the real-time progress from PlaybackState
	}

	return summary
}

// getAuthorName extracts author name(s) from book contributors.
func getAuthorName(ctx context.Context, store *store.Store, book *domain.Book) string {
	// Collect author contributor IDs
	var authorIDs []string
	for _, contrib := range book.Contributors {
		if slices.Contains(contrib.Roles, domain.RoleAuthor) {
			authorIDs = append(authorIDs, contrib.ContributorID)
		}
	}

	if len(authorIDs) == 0 {
		return ""
	}

	// Fetch contributor details
	contributors, err := store.GetContributorsByIDs(ctx, authorIDs)
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
