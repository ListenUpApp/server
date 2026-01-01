package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
)

// ReadingSessionService manages book reading sessions - tracking when users start and complete books.
type ReadingSessionService struct {
	store  *store.Store
	events store.EventEmitter
	logger *slog.Logger
}

// NewReadingSessionService creates a new reading session service.
func NewReadingSessionService(store *store.Store, events store.EventEmitter, logger *slog.Logger) *ReadingSessionService {
	return &ReadingSessionService{
		store:  store,
		events: events,
		logger: logger,
	}
}

// EnsureActiveSession gets the active session for user+book.
// Creates a new session if none exists or if the existing one is stale (>6 months).
// If stale, marks the old session as abandoned first.
func (s *ReadingSessionService) EnsureActiveSession(ctx context.Context, userID, bookID string) (*domain.BookReadingSession, error) {
	// Get active session
	session, err := s.store.GetActiveSession(ctx, userID, bookID)
	if err != nil {
		return nil, fmt.Errorf("get active session: %w", err)
	}

	now := time.Now()

	// If we have an active session, check if it's stale
	if session != nil {
		if session.IsStale(now) {
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
		} else {
			// Active session is fresh, return it
			return session, nil
		}
	}

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

	// Emit SSE event for new session
	s.events.Emit(sse.NewReadingSessionUpdatedEvent(newSession))

	return newSession, nil
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

	// Emit SSE event for completed session
	s.events.Emit(sse.NewReadingSessionUpdatedEvent(session))

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
	// Get current progress from store
	progress, err := s.store.GetProgress(ctx, session.UserID, session.BookID)
	if err != nil && !errors.Is(err, store.ErrProgressNotFound) {
		return fmt.Errorf("get progress: %w", err)
	}

	// Calculate final values
	var finalProgress float64
	var listenTimeMs int64
	if progress != nil {
		finalProgress = progress.Progress
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

		summary := buildReaderSummary(user, sessions)
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

	// Sort other readers by most recent activity
	slices.SortFunc(response.OtherReaders, func(a, b ReaderSummary) int {
		return b.StartedAt.Compare(a.StartedAt)
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
	AvatarColor        string     `json:"avatar_color"`
	IsCurrentlyReading bool       `json:"is_currently_reading"`
	CurrentProgress    float64    `json:"current_progress,omitempty"`
	StartedAt          time.Time  `json:"started_at"`
	FinishedAt         *time.Time `json:"finished_at,omitempty"`
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

// buildReaderSummary creates a reader summary from user and their sessions.
func buildReaderSummary(user *domain.User, sessions []*domain.BookReadingSession) ReaderSummary {
	// Guard against empty sessions
	if len(sessions) == 0 {
		return ReaderSummary{
			UserID:      user.ID,
			DisplayName: user.Name(),
			AvatarColor: avatarColorForUser(user.ID),
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

	summary := ReaderSummary{
		UserID:             user.ID,
		DisplayName:        user.Name(),
		AvatarColor:        avatarColorForUser(user.ID),
		IsCurrentlyReading: activeSession != nil,
		StartedAt:          mostRecentSession.StartedAt,
		FinishedAt:         mostRecentSession.FinishedAt,
		CompletionCount:    completionCount,
	}

	// If currently reading, get current progress from active session
	if activeSession != nil {
		summary.CurrentProgress = activeSession.FinalProgress
	}

	return summary
}

// avatarColorForUser generates a consistent color for a user based on their ID.
func avatarColorForUser(userID string) string {
	h := 0
	for _, c := range userID {
		h = 31*h + int(c)
	}
	if h < 0 {
		h = -h
	}
	hue := float64(h % 360)

	// Convert HSL to RGB (S=0.4, L=0.65)
	r, g, b := hslToRGB(hue, 0.4, 0.65)

	return fmt.Sprintf("#%02X%02X%02X", r, g, b)
}

// hslToRGB converts HSL color space to RGB.
// h: hue (0-360), s: saturation (0-1), l: lightness (0-1)
// Returns RGB values (0-255).
func hslToRGB(h, s, l float64) (r, g, b uint8) {
	// Normalize hue to 0-1
	h = h / 360.0

	var r1, g1, b1 float64

	if s == 0 {
		// Achromatic (gray)
		r1, g1, b1 = l, l, l
	} else {
		var q float64
		if l < 0.5 {
			q = l * (1 + s)
		} else {
			q = l + s - l*s
		}
		p := 2*l - q

		r1 = hueToRGB(p, q, h+1.0/3.0)
		g1 = hueToRGB(p, q, h)
		b1 = hueToRGB(p, q, h-1.0/3.0)
	}

	// Convert to 0-255 range
	r = uint8(r1 * 255)
	g = uint8(g1 * 255)
	b = uint8(b1 * 255)
	return
}

// hueToRGB is a helper for HSL to RGB conversion.
func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t += 1
	}
	if t > 1 {
		t -= 1
	}
	if t < 1.0/6.0 {
		return p + (q-p)*6*t
	}
	if t < 1.0/2.0 {
		return q
	}
	if t < 2.0/3.0 {
		return p + (q-p)*(2.0/3.0-t)*6
	}
	return p
}

// getAuthorName extracts author name(s) from book contributors.
func getAuthorName(ctx context.Context, store *store.Store, book *domain.Book) string {
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
