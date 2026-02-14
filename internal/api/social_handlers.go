package api

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/domain"
)

func (s *Server) registerSocialRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "getLeaderboard",
		Method:      http.MethodGet,
		Path:        "/api/v1/social/leaderboard",
		Summary:     "Get leaderboard",
		Description: "Returns the community leaderboard for the specified category and period",
		Tags:        []string{"Social"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetLeaderboard)

	huma.Register(s.api, huma.Operation{
		OperationID: "getActivityFeed",
		Method:      http.MethodGet,
		Path:        "/api/v1/social/feed",
		Summary:     "Get activity feed",
		Description: "Returns recent community activity, filtered by viewer's book access",
		Tags:        []string{"Social"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetActivityFeed)

	huma.Register(s.api, huma.Operation{
		OperationID: "getBookReaders",
		Method:      http.MethodGet,
		Path:        "/api/v1/books/{id}/readers",
		Summary:     "Get book readers",
		Description: "Returns all users who have read or are reading this book",
		Tags:        []string{"Social"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetBookReaders)

	huma.Register(s.api, huma.Operation{
		OperationID: "getUserReadingHistory",
		Method:      http.MethodGet,
		Path:        "/api/v1/users/me/reading-sessions",
		Summary:     "Get user reading history",
		Description: "Returns the current user's reading session history",
		Tags:        []string{"Social"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetUserReadingHistory)

	huma.Register(s.api, huma.Operation{
		OperationID: "getCurrentlyListening",
		Method:      http.MethodGet,
		Path:        "/api/v1/social/currently-listening",
		Summary:     "Get books others are reading",
		Description: "Returns books that other users are actively listening to, with reader avatars",
		Tags:        []string{"Social"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetCurrentlyListening)

	huma.Register(s.api, huma.Operation{
		OperationID: "getDiscoverBooks",
		Method:      http.MethodGet,
		Path:        "/api/v1/social/discover",
		Summary:     "Get random books for discovery",
		Description: "Returns random books for discovery, series-aware (only first book in series shown)",
		Tags:        []string{"Social"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetDiscoverBooks)
}

// === DTOs ===

// GetLeaderboardInput contains parameters for getting the leaderboard.
type GetLeaderboardInput struct {
	Authorization string `header:"Authorization"`
	Period        string `query:"period" enum:"week,month,year,all" default:"week" doc:"Time period for leaderboard"`
	Category      string `query:"category" enum:"time,books,streak" default:"time" doc:"Ranking category"`
	Limit         int    `query:"limit" doc:"Max entries (default 10, max 50)"`
}

// LeaderboardEntryResponse represents a single leaderboard entry.
type LeaderboardEntryResponse struct {
	Rank          int     `json:"rank" doc:"Position in leaderboard"`
	UserID        string  `json:"user_id" doc:"User ID"`
	DisplayName   string  `json:"display_name" doc:"User display name"`
	AvatarURL     *string `json:"avatar_url,omitempty" doc:"Avatar URL if available"`
	AvatarType    string  `json:"avatar_type" doc:"Avatar type (auto or image)"`
	AvatarValue   string  `json:"avatar_value,omitempty" doc:"Avatar image path (for image type)"`
	AvatarColor   string  `json:"avatar_color" doc:"Generated avatar color (hex)"`
	Value         int64   `json:"value" doc:"Numeric value (time ms, count, or days)"`
	ValueLabel    string  `json:"value_label" doc:"Human-readable value"`
	IsCurrentUser bool    `json:"is_current_user" doc:"Whether this is the requesting user"`
	// All-time totals for caching (only included when period=all)
	TotalTimeMs   *int64 `json:"total_time_ms,omitempty" doc:"All-time listening time in ms (period=all only)"`
	TotalBooks    *int   `json:"total_books,omitempty" doc:"All-time books finished (period=all only)"`
	CurrentStreak *int   `json:"current_streak,omitempty" doc:"Current streak in days (period=all only)"`
}

// CommunityStatsResponse contains aggregate community statistics.
type CommunityStatsResponse struct {
	TotalTimeMs      int64   `json:"total_time_ms" doc:"Community total listening time"`
	TotalTimeLabel   string  `json:"total_time_label" doc:"Human-readable total time"`
	TotalBooks       int     `json:"total_books" doc:"Community total books finished"`
	AverageStreak    float64 `json:"average_streak" doc:"Average streak days across users"`
	ActiveUsersCount int     `json:"active_users_count" doc:"Number of users with activity"`
}

// LeaderboardResponse contains the full leaderboard data.
type LeaderboardResponse struct {
	Category       string                     `json:"category" doc:"Ranking category"`
	Period         string                     `json:"period" doc:"Time period"`
	Entries        []LeaderboardEntryResponse `json:"entries" doc:"Ranked entries"`
	CommunityStats CommunityStatsResponse     `json:"community_stats" doc:"Aggregate stats"`
}

// LeaderboardOutput wraps the leaderboard response for Huma.
type LeaderboardOutput struct {
	Body LeaderboardResponse
}

// === Handlers ===

func (s *Server) handleGetLeaderboard(ctx context.Context, input *GetLeaderboardInput) (*LeaderboardOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Parse and validate period
	period := domain.StatsPeriod(input.Period)
	if !period.Valid() {
		period = domain.StatsPeriodWeek
	}

	// Parse and validate category
	category := domain.LeaderboardCategory(input.Category)
	if !category.Valid() {
		category = domain.LeaderboardCategoryTime
	}

	// Get leaderboard from service
	leaderboard, err := s.services.Social.GetLeaderboard(ctx, userID, period, category, input.Limit)
	if err != nil {
		return nil, err
	}

	// Check if this is an all-time request (for including cache fields)
	isAllTime := period == domain.StatsPeriodAllTime

	// Convert entries to response format
	entries := make([]LeaderboardEntryResponse, len(leaderboard.Entries))
	for i := range leaderboard.Entries {
		e := &leaderboard.Entries[i]
		entry := LeaderboardEntryResponse{
			Rank:          e.Rank,
			UserID:        e.UserID,
			DisplayName:   e.DisplayName,
			AvatarURL:     e.AvatarURL,
			AvatarType:    e.AvatarType,
			AvatarValue:   e.AvatarValue,
			AvatarColor:   e.AvatarColor,
			Value:         e.Value,
			ValueLabel:    e.ValueLabel,
			IsCurrentUser: e.IsCurrentUser,
		}

		// Include all-time totals for client caching when period=all
		if isAllTime {
			totalTimeMs := e.TotalTimeMs
			totalBooks := e.TotalBooks
			currentStreak := e.CurrentStreak
			entry.TotalTimeMs = &totalTimeMs
			entry.TotalBooks = &totalBooks
			entry.CurrentStreak = &currentStreak
		}

		entries[i] = entry
	}

	// Format community total time (handle negative/invalid values)
	totalTimeMs := max(leaderboard.CommunityTotalTimeMs, 0)
	totalMinutes := totalTimeMs / 60_000
	hours := totalMinutes / 60
	minutes := totalMinutes % 60
	var totalTimeLabel string
	switch {
	case hours == 0:
		totalTimeLabel = formatMinutes(minutes)
	case minutes == 0:
		totalTimeLabel = formatHours(hours)
	default:
		totalTimeLabel = formatHoursMinutes(hours, minutes)
	}

	return &LeaderboardOutput{
		Body: LeaderboardResponse{
			Category: string(leaderboard.Category),
			Period:   string(leaderboard.Period),
			Entries:  entries,
			CommunityStats: CommunityStatsResponse{
				TotalTimeMs:      leaderboard.CommunityTotalTimeMs,
				TotalTimeLabel:   totalTimeLabel,
				TotalBooks:       leaderboard.CommunityTotalBooks,
				AverageStreak:    leaderboard.CommunityAverageStreak,
				ActiveUsersCount: leaderboard.TotalUsers,
			},
		},
	}, nil
}

func formatMinutes(m int64) string {
	if m == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", m)
}

func formatHours(h int64) string {
	if h == 1 {
		return "1 hour"
	}
	return fmt.Sprintf("%d hours", h)
}

func formatHoursMinutes(h, m int64) string {
	return fmt.Sprintf("%dh %dm", h, m)
}

// === Book Readers DTOs ===

// GetBookReadersInput contains parameters for getting book readers.
type GetBookReadersInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
	Limit         int    `query:"limit" default:"10" doc:"Max readers to return"`
}

// SessionSummaryResponse represents a single reading session in API format.
type SessionSummaryResponse struct {
	ID           string  `json:"id" doc:"Session ID"`
	StartedAt    string  `json:"started_at" doc:"When session started (RFC3339)"`
	FinishedAt   *string `json:"finished_at,omitempty" doc:"When session finished (RFC3339)"`
	IsCompleted  bool    `json:"is_completed" doc:"Whether session was completed"`
	ListenTimeMs int64   `json:"listen_time_ms" doc:"Total listen time in ms"`
}

// ReaderSummaryResponse represents a user who has read a book in API format.
type ReaderSummaryResponse struct {
	UserID             string  `json:"user_id" doc:"User ID"`
	DisplayName        string  `json:"display_name" doc:"User display name"`
	AvatarType         string  `json:"avatar_type" doc:"Avatar type (auto or image)"`
	AvatarValue        string  `json:"avatar_value,omitempty" doc:"Avatar image path for image type"`
	AvatarColor        string  `json:"avatar_color" doc:"Generated avatar color"`
	IsCurrentlyReading bool    `json:"is_currently_reading" doc:"Whether user is actively reading"`
	CurrentProgress    float64 `json:"current_progress,omitempty" doc:"Current progress (0-1)"`
	StartedAt          string  `json:"started_at" doc:"When user first started (RFC3339)"`
	FinishedAt         *string `json:"finished_at,omitempty" doc:"When user last finished (RFC3339)"`
	CompletionCount    int     `json:"completion_count" doc:"Number of times completed"`
}

// BookReadersResponse contains all readers of a book.
type BookReadersResponse struct {
	YourSessions     []SessionSummaryResponse `json:"your_sessions" doc:"Your reading sessions"`
	OtherReaders     []ReaderSummaryResponse  `json:"other_readers" doc:"Other users who read this book"`
	TotalReaders     int                      `json:"total_readers" doc:"Total number of readers"`
	TotalCompletions int                      `json:"total_completions" doc:"Total completions across all users"`
}

// GetBookReadersOutput wraps the book readers response for Huma.
type GetBookReadersOutput struct {
	Body BookReadersResponse
}

// === User Reading History DTOs ===

// GetUserReadingHistoryInput contains parameters for getting user reading history.
type GetUserReadingHistoryInput struct {
	Authorization string `header:"Authorization"`
	Limit         int    `query:"limit" default:"20" doc:"Max sessions to return"`
}

// ReadingHistorySessionResponse represents a session with book metadata in API format.
type ReadingHistorySessionResponse struct {
	ID           string  `json:"id" doc:"Session ID"`
	BookID       string  `json:"book_id" doc:"Book ID"`
	BookTitle    string  `json:"book_title" doc:"Book title"`
	BookAuthor   string  `json:"book_author,omitempty" doc:"Book author(s)"`
	CoverPath    string  `json:"cover_path,omitempty" doc:"Cover image path"`
	StartedAt    string  `json:"started_at" doc:"When session started (RFC3339)"`
	FinishedAt   *string `json:"finished_at,omitempty" doc:"When session finished (RFC3339)"`
	IsCompleted  bool    `json:"is_completed" doc:"Whether session was completed"`
	ListenTimeMs int64   `json:"listen_time_ms" doc:"Total listen time in ms"`
}

// UserReadingHistoryResponse contains a user's reading history.
type UserReadingHistoryResponse struct {
	Sessions       []ReadingHistorySessionResponse `json:"sessions" doc:"Reading sessions"`
	TotalSessions  int                             `json:"total_sessions" doc:"Total number of sessions"`
	TotalCompleted int                             `json:"total_completed" doc:"Number of completed sessions"`
}

// GetUserReadingHistoryOutput wraps the user reading history response for Huma.
type GetUserReadingHistoryOutput struct {
	Body UserReadingHistoryResponse
}

// === Book Readers Handler ===

func (s *Server) handleGetBookReaders(ctx context.Context, input *GetBookReadersInput) (*GetBookReadersOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	// ACL check - verify user can access this book
	_, err = s.store.GetBook(ctx, input.ID, userID)
	if err != nil {
		return nil, err
	}

	// Validate and cap limit
	limit := input.Limit
	if limit <= 0 {
		limit = 10
	} else if limit > 100 {
		limit = 100
	}

	// Get readers from service
	result, err := s.services.ReadingSession.GetBookReaders(ctx, input.ID, userID, limit)
	if err != nil {
		return nil, err
	}

	// Convert to API format
	yourSessions := make([]SessionSummaryResponse, len(result.YourSessions))
	for i, session := range result.YourSessions {
		yourSessions[i] = SessionSummaryResponse{
			ID:           session.ID,
			StartedAt:    session.StartedAt.Format(time.RFC3339),
			IsCompleted:  session.IsCompleted,
			ListenTimeMs: session.ListenTimeMs,
		}
		if session.FinishedAt != nil {
			finishedAt := session.FinishedAt.Format(time.RFC3339)
			yourSessions[i].FinishedAt = &finishedAt
		}
	}

	otherReaders := make([]ReaderSummaryResponse, len(result.OtherReaders))
	for i := range result.OtherReaders {
		reader := &result.OtherReaders[i]
		otherReaders[i] = ReaderSummaryResponse{
			UserID:             reader.UserID,
			DisplayName:        reader.DisplayName,
			AvatarType:         reader.AvatarType,
			AvatarValue:        reader.AvatarValue,
			AvatarColor:        reader.AvatarColor,
			IsCurrentlyReading: reader.IsCurrentlyReading,
			CurrentProgress:    reader.CurrentProgress,
			StartedAt:          reader.StartedAt.Format(time.RFC3339),
			CompletionCount:    reader.CompletionCount,
		}
		if reader.FinishedAt != nil {
			finishedAt := reader.FinishedAt.Format(time.RFC3339)
			otherReaders[i].FinishedAt = &finishedAt
		}
	}

	return &GetBookReadersOutput{
		Body: BookReadersResponse{
			YourSessions:     yourSessions,
			OtherReaders:     otherReaders,
			TotalReaders:     result.TotalReaders,
			TotalCompletions: result.TotalCompletions,
		},
	}, nil
}

// === User Reading History Handler ===

func (s *Server) handleGetUserReadingHistory(ctx context.Context, input *GetUserReadingHistoryInput) (*GetUserReadingHistoryOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}

	// Get reading history from service
	result, err := s.services.ReadingSession.GetUserReadingHistory(ctx, userID, limit)
	if err != nil {
		return nil, err
	}

	// Convert to API format
	sessions := make([]ReadingHistorySessionResponse, len(result.Sessions))
	for i := range result.Sessions {
		session := &result.Sessions[i]
		sessions[i] = ReadingHistorySessionResponse{
			ID:           session.ID,
			BookID:       session.BookID,
			BookTitle:    session.BookTitle,
			BookAuthor:   session.BookAuthor,
			CoverPath:    session.CoverPath,
			StartedAt:    session.StartedAt.Format(time.RFC3339),
			IsCompleted:  session.IsCompleted,
			ListenTimeMs: session.ListenTimeMs,
		}
		if session.FinishedAt != nil {
			finishedAt := session.FinishedAt.Format(time.RFC3339)
			sessions[i].FinishedAt = &finishedAt
		}
	}

	return &GetUserReadingHistoryOutput{
		Body: UserReadingHistoryResponse{
			Sessions:       sessions,
			TotalSessions:  result.TotalSessions,
			TotalCompleted: result.TotalCompleted,
		},
	}, nil
}

// === Activity Feed DTOs ===

// GetActivityFeedInput contains parameters for getting the activity feed.
type GetActivityFeedInput struct {
	Authorization string `header:"Authorization"`
	Limit         int    `query:"limit" default:"20" doc:"Max activities to return (max 50)"`
	Before        string `query:"before" doc:"Pagination cursor (RFC3339 timestamp or timestamp|activity_id)"`
}

// ActivityResponse represents a single activity in API format.
type ActivityResponse struct {
	ID              string `json:"id" doc:"Activity ID"`
	Type            string `json:"type" doc:"Activity type (started_book, finished_book, streak_milestone, listening_milestone, shelf_created)"`
	CreatedAt       string `json:"created_at" doc:"When activity occurred (RFC3339)"`
	UserID          string `json:"user_id" doc:"User who performed the activity"`
	UserDisplayName string `json:"user_display_name" doc:"User display name"`
	UserAvatarColor string `json:"user_avatar_color" doc:"Generated avatar color"`
	UserAvatarType  string `json:"user_avatar_type" doc:"Avatar type (auto or image)"`
	UserAvatarValue string `json:"user_avatar_value,omitempty" doc:"Avatar image path (for image type)"`

	// Book activities
	BookID         string `json:"book_id,omitempty" doc:"Book ID (for book activities)"`
	BookTitle      string `json:"book_title,omitempty" doc:"Book title"`
	BookAuthorName string `json:"book_author_name,omitempty" doc:"Book author name"`
	BookCoverPath  string `json:"book_cover_path,omitempty" doc:"Book cover path"`
	IsReread       bool   `json:"is_reread,omitempty" doc:"Whether this is a re-read (for started_book)"`

	// Milestone activities
	MilestoneValue int    `json:"milestone_value,omitempty" doc:"Milestone value (days or hours)"`
	MilestoneUnit  string `json:"milestone_unit,omitempty" doc:"Milestone unit (days or hours)"`

	// Shelf activities
	ShelfID   string `json:"shelf_id,omitempty" doc:"Shelf ID (for shelf activities)"`
	ShelfName string `json:"shelf_name,omitempty" doc:"Shelf name"`
}

// ActivityFeedResponse contains the activity feed data.
type ActivityFeedResponse struct {
	Activities []ActivityResponse `json:"activities" doc:"Activity entries"`
	NextCursor *string            `json:"next_cursor,omitempty" doc:"Pagination cursor for next page"`
}

// GetActivityFeedOutput wraps the activity feed response for Huma.
type GetActivityFeedOutput struct {
	Body ActivityFeedResponse
}

// === Activity Feed Handler ===

func (s *Server) handleGetActivityFeed(ctx context.Context, input *GetActivityFeedInput) (*GetActivityFeedOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Validate and cap limit
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	} else if limit > 50 {
		limit = 50
	}

	// Parse 'before' cursor — supports both plain RFC3339 and composite "RFC3339|activity_id" format
	var before *time.Time
	var beforeID string
	if input.Before != "" {
		if parts := strings.SplitN(input.Before, "|", 2); len(parts) == 2 {
			// Composite cursor: timestamp|activity_id
			t, err := time.Parse(time.RFC3339, parts[0])
			if err == nil {
				before = &t
				beforeID = parts[1]
			}
		} else {
			// Legacy plain timestamp cursor
			t, err := time.Parse(time.RFC3339, input.Before)
			if err == nil {
				before = &t
			}
		}
	}

	// Get activities from service
	activities, err := s.services.Activity.GetFeed(ctx, userID, limit, before, beforeID)
	if err != nil {
		return nil, err
	}

	// Convert to API format
	responses := make([]ActivityResponse, len(activities))
	for i, a := range activities {
		responses[i] = ActivityResponse{
			ID:              a.ID,
			Type:            string(a.Type),
			CreatedAt:       a.CreatedAt.Format(time.RFC3339),
			UserID:          a.UserID,
			UserDisplayName: a.UserDisplayName,
			UserAvatarColor: a.UserAvatarColor,
			UserAvatarType:  a.UserAvatarType,
			UserAvatarValue: a.UserAvatarValue,
			BookID:          a.BookID,
			BookTitle:       a.BookTitle,
			BookAuthorName:  a.BookAuthorName,
			BookCoverPath:   a.BookCoverPath,
			IsReread:        a.IsReread,
			MilestoneValue:  a.MilestoneValue,
			MilestoneUnit:   a.MilestoneUnit,
			ShelfID:         a.ShelfID,
			ShelfName:       a.ShelfName,
		}
	}

	// Set pagination cursor using composite format (timestamp|activity_id) for deterministic pagination
	var nextCursor *string
	if len(activities) == limit && len(activities) > 0 {
		last := activities[len(activities)-1]
		cursor := last.CreatedAt.Format(time.RFC3339) + "|" + last.ID
		nextCursor = &cursor
	}

	return &GetActivityFeedOutput{
		Body: ActivityFeedResponse{
			Activities: responses,
			NextCursor: nextCursor,
		},
	}, nil
}

// === Currently Listening DTOs ===

// GetCurrentlyListeningInput contains parameters for getting currently listening books.
type GetCurrentlyListeningInput struct {
	Authorization string `header:"Authorization"`
	Limit         int    `query:"limit" default:"10" doc:"Max books to return (max 20)"`
}

// CurrentlyListeningReaderResponse represents a reader for avatar display.
type CurrentlyListeningReaderResponse struct {
	UserID      string `json:"user_id" doc:"User ID"`
	DisplayName string `json:"display_name" doc:"User display name"`
	AvatarColor string `json:"avatar_color" doc:"Generated avatar color (hex)"`
	AvatarType  string `json:"avatar_type" doc:"Avatar type (auto or image)"`
	AvatarValue string `json:"avatar_value,omitempty" doc:"Avatar image path (for image type)"`
}

// CurrentlyListeningBookResponse represents a book that others are reading.
type CurrentlyListeningBookResponse struct {
	ID               string                             `json:"id" doc:"Book ID"`
	Title            string                             `json:"title" doc:"Book title"`
	AuthorName       string                             `json:"author_name,omitempty" doc:"Book author(s)"`
	CoverPath        string                             `json:"cover_path,omitempty" doc:"Cover image path"`
	CoverBlurHash    string                             `json:"cover_blur_hash,omitempty" doc:"Cover blur hash"`
	DurationMs       int64                              `json:"duration_ms" doc:"Total duration in ms"`
	Readers          []CurrentlyListeningReaderResponse `json:"readers" doc:"Up to 3 readers for avatar display"`
	TotalReaderCount int                                `json:"total_reader_count" doc:"Total number of active readers"`
}

// CurrentlyListeningResponse contains the currently listening books.
type CurrentlyListeningResponse struct {
	Books []CurrentlyListeningBookResponse `json:"books" doc:"Books that others are reading"`
}

// GetCurrentlyListeningOutput wraps the currently listening response for Huma.
type GetCurrentlyListeningOutput struct {
	Body CurrentlyListeningResponse
}

// === Discover Books DTOs ===

// GetDiscoverBooksInput contains parameters for getting discovery books.
type GetDiscoverBooksInput struct {
	Authorization string `header:"Authorization"`
	Limit         int    `query:"limit" default:"10" doc:"Max books to return (max 20)"`
}

// DiscoverBookResponse represents a book for discovery.
type DiscoverBookResponse struct {
	ID            string  `json:"id" doc:"Book ID"`
	Title         string  `json:"title" doc:"Book title"`
	AuthorName    string  `json:"author_name,omitempty" doc:"Book author(s)"`
	CoverPath     string  `json:"cover_path,omitempty" doc:"Cover image path"`
	CoverBlurHash string  `json:"cover_blur_hash,omitempty" doc:"Cover blur hash"`
	DurationMs    int64   `json:"duration_ms" doc:"Total duration in ms"`
	SeriesName    *string `json:"series_name,omitempty" doc:"Series name if part of a series"`
}

// DiscoverBooksResponse contains discovery books.
type DiscoverBooksResponse struct {
	Books []DiscoverBookResponse `json:"books" doc:"Random books for discovery"`
}

// GetDiscoverBooksOutput wraps the discover books response for Huma.
type GetDiscoverBooksOutput struct {
	Body DiscoverBooksResponse
}

// === Currently Listening Handler ===

func (s *Server) handleGetCurrentlyListening(ctx context.Context, input *GetCurrentlyListeningInput) (*GetCurrentlyListeningOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Get currently listening books from service
	books, err := s.services.Social.GetCurrentlyListening(ctx, userID, input.Limit)
	if err != nil {
		return nil, err
	}

	// Convert to API format
	responses := make([]CurrentlyListeningBookResponse, len(books))
	for i, b := range books {
		// Get author name
		authorName := s.getAuthorNameFromBook(ctx, b.Book)

		// Get cover info
		var coverPath, coverBlurHash string
		if b.Book.CoverImage != nil {
			coverPath = b.Book.CoverImage.Path
			coverBlurHash = b.Book.CoverImage.BlurHash
		}

		// Convert readers
		readers := make([]CurrentlyListeningReaderResponse, len(b.Readers))
		for j, r := range b.Readers {
			readers[j] = CurrentlyListeningReaderResponse{
				UserID:      r.UserID,
				DisplayName: r.DisplayName,
				AvatarColor: r.AvatarColor,
				AvatarType:  r.AvatarType,
				AvatarValue: r.AvatarValue,
			}
		}

		responses[i] = CurrentlyListeningBookResponse{
			ID:               b.Book.ID,
			Title:            b.Book.Title,
			AuthorName:       authorName,
			CoverPath:        coverPath,
			CoverBlurHash:    coverBlurHash,
			DurationMs:       b.Book.TotalDuration,
			Readers:          readers,
			TotalReaderCount: b.TotalReaderCount,
		}
	}

	return &GetCurrentlyListeningOutput{
		Body: CurrentlyListeningResponse{
			Books: responses,
		},
	}, nil
}

// === Discover Books Handler ===

func (s *Server) handleGetDiscoverBooks(ctx context.Context, input *GetDiscoverBooksInput) (*GetDiscoverBooksOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Get random books from service
	books, err := s.services.Social.GetRandomBooks(ctx, userID, input.Limit)
	if err != nil {
		return nil, err
	}

	// Convert to API format
	responses := make([]DiscoverBookResponse, len(books))
	for i, book := range books {
		// Get author name
		authorName := s.getAuthorNameFromBook(ctx, book)

		// Get cover info
		var coverPath, coverBlurHash string
		if book.CoverImage != nil {
			coverPath = book.CoverImage.Path
			coverBlurHash = book.CoverImage.BlurHash
		}

		// Get series name if applicable
		var seriesName *string
		if len(book.Series) > 0 {
			// Get the first series name
			series, err := s.store.GetSeries(ctx, book.Series[0].SeriesID)
			if err == nil && series != nil {
				seriesName = &series.Name
			}
		}

		responses[i] = DiscoverBookResponse{
			ID:            book.ID,
			Title:         book.Title,
			AuthorName:    authorName,
			CoverPath:     coverPath,
			CoverBlurHash: coverBlurHash,
			DurationMs:    book.TotalDuration,
			SeriesName:    seriesName,
		}
	}

	return &GetDiscoverBooksOutput{
		Body: DiscoverBooksResponse{
			Books: responses,
		},
	}, nil
}

// getAuthorNameFromBook extracts author name(s) from book contributors.
func (s *Server) getAuthorNameFromBook(ctx context.Context, book *domain.Book) string {
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
