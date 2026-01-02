package service

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// SocialService provides social features like leaderboards.
type SocialService struct {
	store  *store.Store
	logger *slog.Logger
}

// NewSocialService creates a new social service.
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

	// Get all users
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}

	// Build entries for each user
	type userStats struct {
		userID      string
		displayName string
		avatarURL   *string
		timeMs      int64
		booksCount  int
		streakDays  int
	}

	stats := make([]userStats, 0, len(users))
	var communityTotalTimeMs int64
	var communityTotalBooks int
	var communityStreakSum int

	for _, user := range users {
		// Get events for user in period
		events, err := s.store.GetEventsForUserInRange(ctx, user.ID, start, end)
		if err != nil {
			s.logger.Debug("failed to get events for user", "user_id", user.ID, "error", err)
			continue
		}

		// Calculate listening time (only count positive durations)
		var totalTimeMs int64
		booksSet := make(map[string]bool)
		for _, e := range events {
			if e.DurationMs > 0 {
				totalTimeMs += e.DurationMs
			}
			booksSet[e.BookID] = true
		}

		// Get finished books count
		finishedProgress, err := s.store.GetProgressFinishedInRange(ctx, user.ID, start, end)
		if err != nil {
			s.logger.Debug("failed to get finished progress", "user_id", user.ID, "error", err)
		}
		booksFinished := len(finishedProgress)

		// Calculate streak
		streakDays := s.CalculateUserStreak(ctx, user.ID)

		stats = append(stats, userStats{
			userID:      user.ID,
			displayName: user.DisplayName,
			avatarURL:   nil, // Users don't have avatars yet
			timeMs:      totalTimeMs,
			booksCount:  booksFinished,
			streakDays:  streakDays,
		})

		// Aggregate for community stats
		communityTotalTimeMs += totalTimeMs
		communityTotalBooks += booksFinished
		communityStreakSum += streakDays
	}

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

		entries = append(entries, domain.LeaderboardEntry{
			Rank:          i + 1,
			UserID:        stat.userID,
			DisplayName:   stat.displayName,
			AvatarURL:     stat.avatarURL,
			Value:         value,
			ValueLabel:    valueLabel,
			IsCurrentUser: stat.userID == viewingUserID,
		})
	}

	// Calculate community average streak
	var avgStreak float64
	if len(stats) > 0 {
		avgStreak = float64(communityStreakSum) / float64(len(stats))
	}

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
func (s *SocialService) CalculateUserStreak(ctx context.Context, userID string) int {
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
