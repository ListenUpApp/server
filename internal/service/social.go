package service

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	"github.com/listenupapp/listenup-server/internal/color"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// SocialService provides social features like leaderboards.
type SocialService struct {
	store  *store.Store
	logger *slog.Logger
}

// NewSocialService creates a new social service.

// isFirstInSeries returns true if the sequence indicates the book is a series starter.
// Includes empty/unknown sequences, "0", "0.5", "1", "01", "001", "1.0", "Book 1", etc.
func isFirstInSeries(sequence string) bool {
	s := strings.TrimSpace(sequence)
	if s == "" {
		return true
	}
	// Prequels
	if s == "0" || s == "0.5" {
		return true
	}
	// Find the first digit, stripping prefixes like "Book "
	idx := strings.IndexFunc(s, func(r rune) bool { return r >= '0' && r <= '9' })
	if idx == -1 {
		return true // no number found, include rather than hide
	}
	numPart := strings.TrimLeft(s[idx:], "0")
	if numPart == "" {
		return true // all zeros
	}
	if numPart[0] != '1' {
		return false
	}
	if len(numPart) == 1 {
		return true
	}
	return numPart[1] == '.' || numPart[1] == ' '
}

func NewSocialService(store *store.Store, logger *slog.Logger) *SocialService {
	return &SocialService{
		store:  store,
		logger: logger,
	}
}

// minListenMsForStreak is the minimum listening time (30 seconds) to count for streaks.
const minListenMsForStreak = 30 * 1000

// GetLeaderboard returns the leaderboard for the given category and period.
func (s *SocialService) GetLeaderboard(
	ctx context.Context,
	viewingUserID string,
	period domain.StatsPeriod,
	category domain.LeaderboardCategory,
	limit int,
) (*domain.Leaderboard, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	now := time.Now()
	start, end := period.Bounds(now)

	s.logger.Info("GetLeaderboard called",
		"period", period,
		"category", category,
		"start", start.Format(time.RFC3339),
		"end", end.Format(time.RFC3339),
	)

	// Get all users
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}

	s.logger.Debug("Found users for leaderboard", "count", len(users))

	// Check if this is an all-time request (for caching full stats)
	isAllTime := period == domain.StatsPeriodAllTime

	// Build entries for each user
	type userStats struct {
		userID      string
		displayName string
		avatarURL   *string
		avatarType  string
		avatarValue string
		avatarColor string
		timeMs      int64
		booksCount  int
		streakDays  int
		// All-time totals (only calculated for period=all)
		allTimeMs     int64
		allBooks      int
		currentStreak int
	}

	stats := make([]userStats, 0, len(users))
	var communityTotalTimeMs int64
	var communityTotalBooks int
	var communityStreakSum int

	// For all-time period, use pre-aggregated user_stats (O(1) per user instead of O(events))
	var preAggregated map[string]*domain.UserStats
	if isAllTime {
		allStats, err := s.store.GetAllUserStats(ctx)
		if err != nil {
			s.logger.Warn("failed to get pre-aggregated stats, falling back to events", "error", err)
		} else {
			preAggregated = make(map[string]*domain.UserStats, len(allStats))
			for _, st := range allStats {
				preAggregated[st.UserID] = st
			}
		}
	}

	for _, user := range users {
		var totalTimeMs int64
		var booksFinished int

		if isAllTime && preAggregated != nil {
			// Fast path: use pre-aggregated stats
			if cached, ok := preAggregated[user.ID]; ok {
				totalTimeMs = cached.TotalListenTimeMs
				booksFinished = cached.TotalBooksFinished
			}
		} else {
			// Period-specific: query events in range
			events, err := s.store.GetEventsForUserInRange(ctx, user.ID, start, end)
			if err != nil {
				s.logger.Debug("failed to get events for user", "user_id", user.ID, "error", err)
				continue
			}

			const maxReasonableDurationMs = 24 * 60 * 60 * 1000
			for _, e := range events {
				if e.DurationMs > 0 && e.DurationMs <= maxReasonableDurationMs {
					totalTimeMs += e.DurationMs
				} else if e.DurationMs > maxReasonableDurationMs {
					s.logger.Warn("Skipping event with corrupted duration",
						"event_id", e.ID,
						"user_id", user.ID,
						"duration_ms", e.DurationMs,
						"end_position_ms", e.EndPositionMs,
					)
				}
			}

			finishedProgress, err := s.store.GetStateFinishedInRange(ctx, user.ID, start, end)
			if err != nil {
				s.logger.Debug("failed to get finished progress", "user_id", user.ID, "error", err)
			}
			booksFinished = len(finishedProgress)
		}

		// Calculate streak (always calculated as it's the current streak, not period-based)
		streakDays := s.CalculateUserStreak(ctx, user.ID)

		// Get avatar info from user profile
		avatarType := string(domain.AvatarTypeAuto)
		avatarValue := ""
		avatarColor := color.ForUser(user.ID)
		profile, err := s.store.GetUserProfile(ctx, user.ID)
		if err == nil && profile != nil {
			avatarType = string(profile.AvatarType)
			avatarValue = profile.AvatarValue
		}

		s.logger.Debug("User stats calculated",
			"user_id", user.ID,
			"display_name", user.DisplayName,
			"time_ms", totalTimeMs,
			"time_hours", float64(totalTimeMs)/3600000,
			"books_finished", booksFinished,
			"streak_days", streakDays,
		)

		stat := userStats{
			userID:        user.ID,
			displayName:   user.DisplayName,
			avatarURL:     nil,
			avatarType:    avatarType,
			avatarValue:   avatarValue,
			avatarColor:   avatarColor,
			timeMs:        totalTimeMs,
			booksCount:    booksFinished,
			streakDays:    streakDays,
			currentStreak: streakDays,
		}

		if isAllTime {
			stat.allTimeMs = totalTimeMs
			stat.allBooks = booksFinished
		}

		stats = append(stats, stat)

		communityTotalTimeMs += totalTimeMs
		communityTotalBooks += booksFinished
		communityStreakSum += streakDays
	}

	s.logger.Info("Community stats calculated",
		"period", period,
		"total_users", len(stats),
		"community_time_ms", communityTotalTimeMs,
		"community_time_hours", float64(communityTotalTimeMs)/3600000,
		"community_books", communityTotalBooks,
		"community_streak_sum", communityStreakSum,
	)

	// Sort based on category
	switch category {
	case domain.LeaderboardCategoryTime:
		slices.SortFunc(stats, func(a, b userStats) int {
			if b.timeMs != a.timeMs {
				if b.timeMs > a.timeMs {
					return 1
				}
				return -1
			}
			return 0
		})
	case domain.LeaderboardCategoryBooks:
		slices.SortFunc(stats, func(a, b userStats) int {
			if b.booksCount != a.booksCount {
				return b.booksCount - a.booksCount
			}
			return 0
		})
	case domain.LeaderboardCategoryStreak:
		slices.SortFunc(stats, func(a, b userStats) int {
			if b.streakDays != a.streakDays {
				return b.streakDays - a.streakDays
			}
			return 0
		})
	}

	// Build entries with rank
	entries := make([]domain.LeaderboardEntry, 0, min(limit, len(stats)))
	for i, stat := range stats {
		if i >= limit {
			break
		}

		var value int64
		var valueLabel string

		switch category {
		case domain.LeaderboardCategoryTime:
			value = stat.timeMs
			valueLabel = formatDuration(stat.timeMs)
		case domain.LeaderboardCategoryBooks:
			value = int64(stat.booksCount)
			if stat.booksCount == 1 {
				valueLabel = "1 book"
			} else {
				valueLabel = fmt.Sprintf("%d books", stat.booksCount)
			}
		case domain.LeaderboardCategoryStreak:
			value = int64(stat.streakDays)
			if stat.streakDays == 1 {
				valueLabel = "1 day"
			} else {
				valueLabel = fmt.Sprintf("%d days", stat.streakDays)
			}
		}

		entry := domain.LeaderboardEntry{
			Rank:          i + 1,
			UserID:        stat.userID,
			DisplayName:   stat.displayName,
			AvatarURL:     stat.avatarURL,
			AvatarType:    stat.avatarType,
			AvatarValue:   stat.avatarValue,
			AvatarColor:   stat.avatarColor,
			Value:         value,
			ValueLabel:    valueLabel,
			IsCurrentUser: stat.userID == viewingUserID,
		}

		// Include all-time totals for caching when period=all
		if isAllTime {
			entry.TotalTimeMs = stat.allTimeMs
			entry.TotalBooks = stat.allBooks
			entry.CurrentStreak = stat.currentStreak
		}

		entries = append(entries, entry)
	}

	// Calculate community average streak
	var avgStreak float64
	if len(stats) > 0 {
		avgStreak = float64(communityStreakSum) / float64(len(stats))
	}

	s.logger.Info("Returning leaderboard",
		"period", period,
		"category", category,
		"entries_count", len(entries),
		"community_time_ms", communityTotalTimeMs,
		"community_books", communityTotalBooks,
		"avg_streak", avgStreak,
	)

	return &domain.Leaderboard{
		Category:               category,
		Period:                 period,
		Entries:                entries,
		TotalUsers:             len(users),
		CommunityTotalTimeMs:   communityTotalTimeMs,
		CommunityTotalBooks:    communityTotalBooks,
		CommunityAverageStreak: avgStreak,
	}, nil
}

// CalculateUserStreak calculates the current streak for a user.
// Exported for use by ListeningService for milestone tracking.
// Optimized: checks pre-aggregated last_listened_date first for early exit.
func (s *SocialService) CalculateUserStreak(ctx context.Context, userID string) int {
	// Fast path: check if user has listened recently via pre-aggregated stats
	cachedStats, err := s.store.GetUserStats(ctx, userID)
	if err == nil && cachedStats != nil && cachedStats.LastListenedDate != "" {
		loc := time.Local
		now := time.Now().In(loc)
		todayStr := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).Format("2006-01-02")
		yesterdayStr := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -1).Format("2006-01-02")

		if cachedStats.LastListenedDate != todayStr && cachedStats.LastListenedDate != yesterdayStr {
			// Haven't listened today or yesterday — streak is broken
			return 0
		}
	}

	events, err := s.store.GetEventsForUser(ctx, userID)
	if err != nil {
		s.logger.Debug("failed to fetch events for streak", "user_id", userID, "error", err)
		return 0
	}

	if len(events) == 0 {
		return 0
	}

	// Build map of date -> total listen time
	loc := time.Local
	dailyTime := make(map[string]int64)
	for _, e := range events {
		year, month, day := e.EndedAt.In(loc).Date()
		dateKey := time.Date(year, month, day, 0, 0, 0, 0, loc).Format("2006-01-02")
		dailyTime[dateKey] += e.DurationMs
	}

	// Build qualifying dates set
	qualifyingSet := make(map[string]bool)
	var qualifyingDates []string
	for dateStr, ms := range dailyTime {
		if ms >= minListenMsForStreak {
			qualifyingSet[dateStr] = true
			qualifyingDates = append(qualifyingDates, dateStr)
		}
	}

	if len(qualifyingDates) == 0 {
		return 0
	}

	slices.Sort(qualifyingDates)

	// Calculate today and yesterday
	now := time.Now().In(loc)
	todayStr := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).Format("2006-01-02")
	yesterdayStr := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -1).Format("2006-01-02")

	// Calculate current streak
	lastQualifyingDate := qualifyingDates[len(qualifyingDates)-1]
	if lastQualifyingDate != todayStr && lastQualifyingDate != yesterdayStr {
		return 0
	}

	currentStreak := 1
	checkDate, _ := time.Parse("2006-01-02", lastQualifyingDate)
	checkDate = checkDate.AddDate(0, 0, -1)

	for {
		checkDateStr := checkDate.Format("2006-01-02")
		if qualifyingSet[checkDateStr] {
			currentStreak++
			checkDate = checkDate.AddDate(0, 0, -1)
		} else {
			break
		}
	}

	return currentStreak
}

// formatDuration formats milliseconds as human-readable duration.
func formatDuration(ms int64) string {
	if ms <= 0 {
		return "0m"
	}
	totalMinutes := ms / 60_000
	hours := totalMinutes / 60
	minutes := totalMinutes % 60

	switch {
	case hours == 0:
		return fmt.Sprintf("%dm", minutes)
	case minutes == 0:
		return fmt.Sprintf("%dh", hours)
	default:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
}

// CurrentlyListeningBook represents a book that others are actively reading.
type CurrentlyListeningBook struct {
	Book             *domain.Book
	Readers          []ReaderInfo // Up to 3 readers for avatar display
	TotalReaderCount int
}

// ReaderInfo contains basic user info for avatar display.
type ReaderInfo struct {
	UserID      string
	DisplayName string
	AvatarColor string
	AvatarType  string // "auto" or "image"
	AvatarValue string // Path to image (empty for auto)
}

// GetCurrentlyListening returns books that other users are actively reading.
// Excludes the viewing user's books. Filters by ACL.
// Results are sorted by reader count descending (most popular first).
func (s *SocialService) GetCurrentlyListening(ctx context.Context, viewingUserID string, limit int) ([]CurrentlyListeningBook, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 20 {
		limit = 20
	}

	// Get all active sessions
	activeSessions, err := s.store.GetAllActiveSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting active sessions: %w", err)
	}

	s.logger.Info("GetCurrentlyListening",
		"viewing_user_id", viewingUserID,
		"total_active_sessions", len(activeSessions))

	// Log each active session for debugging
	for i, sess := range activeSessions {
		s.logger.Info("active session",
			"index", i,
			"session_id", sess.ID,
			"user_id", sess.UserID,
			"book_id", sess.BookID,
			"is_viewing_user", sess.UserID == viewingUserID)
	}

	// Get viewing user's accessible books for ACL filtering
	accessibleBooks, err := s.store.GetBooksForUser(ctx, viewingUserID)
	if err != nil {
		return nil, fmt.Errorf("getting accessible books: %w", err)
	}
	accessibleBookIDs := make(map[string]bool, len(accessibleBooks))
	for _, book := range accessibleBooks {
		accessibleBookIDs[book.ID] = true
	}

	s.logger.Info("GetCurrentlyListening ACL",
		"accessible_books_count", len(accessibleBooks))

	// Group sessions by book, excluding viewing user
	type bookReaders struct {
		bookID  string
		readers []ReaderInfo
	}
	bookReadersMap := make(map[string]*bookReaders)

	var excludedSelf, excludedACL int
	for _, session := range activeSessions {
		// Exclude viewing user's sessions
		if session.UserID == viewingUserID {
			excludedSelf++
			s.logger.Info("excluding self session", "session_id", session.ID, "book_id", session.BookID)
			continue
		}
		// ACL filter
		if !accessibleBookIDs[session.BookID] {
			excludedACL++
			s.logger.Info("excluding ACL session", "session_id", session.ID, "book_id", session.BookID, "user_id", session.UserID)
			continue
		}
		s.logger.Info("including session", "session_id", session.ID, "book_id", session.BookID, "user_id", session.UserID)

		// Get or create entry
		br, exists := bookReadersMap[session.BookID]
		if !exists {
			br = &bookReaders{bookID: session.BookID}
			bookReadersMap[session.BookID] = br
		}

		// Get user info for this reader
		user, err := s.store.GetUser(ctx, session.UserID)
		if err != nil {
			s.logger.Debug("failed to get user for session", "user_id", session.UserID, "error", err)
			continue
		}

		// Get profile for avatar info (optional - may not exist)
		avatarType := string(domain.AvatarTypeAuto)
		avatarValue := ""
		profile, err := s.store.GetUserProfile(ctx, session.UserID)
		if err == nil && profile != nil {
			avatarType = string(profile.AvatarType)
			avatarValue = profile.AvatarValue
		}

		br.readers = append(br.readers, ReaderInfo{
			UserID:      user.ID,
			DisplayName: user.Name(),
			AvatarColor: color.ForUser(user.ID),
			AvatarType:  avatarType,
			AvatarValue: avatarValue,
		})
	}

	// Convert to slice and sort by reader count
	books := make([]CurrentlyListeningBook, 0, len(bookReadersMap))
	for bookID, br := range bookReadersMap {
		book, err := s.store.GetBookNoAccessCheck(ctx, bookID)
		if err != nil {
			s.logger.Debug("failed to get book", "book_id", bookID, "error", err)
			continue
		}

		// Limit readers to 3 for avatar display
		displayReaders := br.readers
		if len(displayReaders) > 3 {
			displayReaders = displayReaders[:3]
		}

		books = append(books, CurrentlyListeningBook{
			Book:             book,
			Readers:          displayReaders,
			TotalReaderCount: len(br.readers),
		})
	}

	// Sort by reader count descending
	slices.SortFunc(books, func(a, b CurrentlyListeningBook) int {
		return b.TotalReaderCount - a.TotalReaderCount
	})

	// Apply limit
	if len(books) > limit {
		books = books[:limit]
	}

	s.logger.Info("GetCurrentlyListening result",
		"excluded_self", excludedSelf,
		"excluded_acl", excludedACL,
		"result_books", len(books))

	return books, nil
}

// GetRandomBooks returns a random selection of books for discovery.
// Series-aware: only shows first book in series (or standalone books).
// Excludes books the user has already started.
func (s *SocialService) GetRandomBooks(ctx context.Context, viewingUserID string, limit int) ([]*domain.Book, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 20 {
		limit = 20
	}

	// Get all accessible books
	books, err := s.store.GetBooksForUser(ctx, viewingUserID)
	if err != nil {
		return nil, fmt.Errorf("getting accessible books: %w", err)
	}

	// Get user's state to exclude books they've started
	allProgress, err := s.store.GetStateForUser(ctx, viewingUserID)
	if err != nil {
		s.logger.Debug("failed to get user state", "user_id", viewingUserID, "error", err)
		// Continue without filtering - better to show something
		allProgress = nil
	}
	startedBooks := make(map[string]bool, len(allProgress))
	for _, p := range allProgress {
		startedBooks[p.BookID] = true
	}

	// Filter: series-aware and not started
	var candidates []*domain.Book
	for _, book := range books {
		// Skip if already started
		if startedBooks[book.ID] {
			continue
		}

		// Series check: include if standalone OR first in series
		if len(book.Series) > 0 {
			// Has series - check if any is first (or prequel)
			isFirstInAnySeries := false
			for _, series := range book.Series {
				if isFirstInSeries(series.Sequence) {
					isFirstInAnySeries = true
					break
				}
			}
			// If it's in a series but not first, skip it
			if !isFirstInAnySeries {
				continue
			}
		}
		// Standalone books (no series) always included

		candidates = append(candidates, book)
	}

	// Shuffle randomly
	shuffleBooks(candidates)

	// Apply limit
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	return candidates, nil
}

// shuffleBooks shuffles a slice of books in place using Fisher-Yates.
func shuffleBooks(books []*domain.Book) {
	rand.Shuffle(len(books), func(i, j int) {
		books[i], books[j] = books[j], books[i]
	})
}
