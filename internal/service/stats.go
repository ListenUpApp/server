package service

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// StatsService provides detailed listening statistics.
type StatsService struct {
	store  *store.Store
	logger *slog.Logger
}

// NewStatsService creates a new stats service.
func NewStatsService(store *store.Store, logger *slog.Logger) *StatsService {
	return &StatsService{
		store:  store,
		logger: logger,
	}
}

// minListenMs is the minimum listening time (30 seconds) to count as a "day" for streaks.
// This prevents accidental starts from counting.
const minListenMs = 30 * 1000

// GetUserStats returns detailed statistics for a user and time period.
func (s *StatsService) GetUserStats(
	ctx context.Context,
	userID string,
	period domain.StatsPeriod,
) (*domain.UserStatsDetailed, error) {
	now := time.Now()
	start, end := period.Bounds(now)

	s.logger.Info("calculating user stats",
		"user_id", userID,
		"period", period,
		"range_start", start.Format(time.RFC3339),
		"range_end", end.Format(time.RFC3339),
	)

	// Fetch events in range
	events, err := s.store.GetEventsForUserInRange(ctx, userID, start, end)
	if err != nil {
		return nil, err
	}

	// Calculate total time from events for debugging
	var totalTimeMs int64
	for _, e := range events {
		totalTimeMs += e.DurationMs
		s.logger.Debug("event in range",
			"event_id", e.ID,
			"ended_at", e.EndedAt.Format(time.RFC3339),
			"duration_ms", e.DurationMs,
		)
	}
	s.logger.Info("found events for stats",
		"user_id", userID,
		"event_count", len(events),
		"total_time_ms", totalTimeMs,
	)

	// Fetch finished books in range
	finishedProgress, err := s.store.GetProgressFinishedInRange(ctx, userID, start, end)
	if err != nil {
		return nil, err
	}

	// Fetch all progress for books started count
	allProgress, err := s.store.GetProgressForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Calculate headline stats
	stats := &domain.UserStatsDetailed{
		Period:        period,
		StartDate:     start,
		EndDate:       end,
		BooksFinished: len(finishedProgress),
	}

	// Count books started in period
	for _, p := range allProgress {
		if (start.IsZero() || !p.StartedAt.Before(start)) && p.StartedAt.Before(end) {
			stats.BooksStarted++
		}
	}

	// Aggregate events
	dailyMap := make(map[string]*domain.DailyListening)
	genreMap := make(map[string]int64)
	booksPerDay := make(map[string]map[string]bool) // date -> bookID -> true

	for _, e := range events {
		stats.TotalListenTimeMs += e.DurationMs

		// Daily aggregation
		dateKey := e.EndedAt.Format("2006-01-02")
		if dailyMap[dateKey] == nil {
			dailyMap[dateKey] = &domain.DailyListening{
				Date: time.Date(
					e.EndedAt.Year(), e.EndedAt.Month(), e.EndedAt.Day(),
					0, 0, 0, 0, e.EndedAt.Location(),
				),
			}
			booksPerDay[dateKey] = make(map[string]bool)
		}
		dailyMap[dateKey].ListenTimeMs += e.DurationMs
		booksPerDay[dateKey][e.BookID] = true

		// Genre aggregation - computed from current book metadata
		// (so stats reflect latest genre assignments, not historical)
		book, err := s.store.GetBook(ctx, e.BookID, userID)
		if err == nil && book != nil && len(book.GenreIDs) > 0 {
			// Fetch genres to get slugs
			genres, err := s.store.GetGenresByIDs(ctx, book.GenreIDs)
			if err == nil {
				for _, genre := range genres {
					genreMap[genre.Slug] += e.DurationMs
				}
			}
		}
		// Books without genres are excluded from genre breakdown
	}

	// Build daily listening slice
	stats.DailyListening = make([]domain.DailyListening, 0, len(dailyMap))
	for dateKey, daily := range dailyMap {
		daily.BooksListened = len(booksPerDay[dateKey])
		stats.DailyListening = append(stats.DailyListening, *daily)
	}

	// Sort by date ascending
	slices.SortFunc(stats.DailyListening, func(a, b domain.DailyListening) int {
		return a.Date.Compare(b.Date)
	})

	// Build genre breakdown
	var totalGenreTime int64
	for _, ms := range genreMap {
		totalGenreTime += ms
	}

	stats.GenreBreakdown = make([]domain.GenreListening, 0, len(genreMap))
	for slug, ms := range genreMap {
		pct := 0.0
		if totalGenreTime > 0 {
			pct = float64(ms) / float64(totalGenreTime) * 100
		}
		stats.GenreBreakdown = append(stats.GenreBreakdown, domain.GenreListening{
			GenreSlug:    slug,
			GenreName:    s.genreDisplayName(slug),
			ListenTimeMs: ms,
			Percentage:   pct,
		})
	}

	// Sort genres by time descending, keep top 5
	slices.SortFunc(stats.GenreBreakdown, func(a, b domain.GenreListening) int {
		if b.ListenTimeMs != a.ListenTimeMs {
			if b.ListenTimeMs > a.ListenTimeMs {
				return 1
			}
			return -1
		}
		return 0
	})
	if len(stats.GenreBreakdown) > 5 {
		stats.GenreBreakdown = stats.GenreBreakdown[:5]
	}

	// Calculate streaks
	stats.CurrentStreakDays, stats.LongestStreakDays = s.calculateStreaks(ctx, userID)

	// Build streak calendar (past 12 weeks)
	stats.StreakCalendar = s.buildStreakCalendar(ctx, userID, now)

	return stats, nil
}

// calculateStreaks computes current and longest listening streaks.
func (s *StatsService) calculateStreaks(ctx context.Context, userID string) (current, longest int) {
	// Get all events (we need full history for streaks)
	events, err := s.store.GetEventsForUser(ctx, userID)
	if err != nil {
		s.logger.Debug("failed to fetch events for streak calculation", "error", err)
		return 0, 0
	}

	if len(events) == 0 {
		return 0, 0
	}

	// Build map of date -> total listen time (use local timezone consistently)
	loc := time.Local
	dailyTime := make(map[string]int64)
	for _, e := range events {
		// Normalize to date in local timezone
		year, month, day := e.EndedAt.In(loc).Date()
		dateKey := time.Date(year, month, day, 0, 0, 0, 0, loc).Format("2006-01-02")
		dailyTime[dateKey] += e.DurationMs
	}

	// Convert to sorted list of "qualifying" dates (>= 30 seconds)
	var qualifyingDates []string
	for dateStr, ms := range dailyTime {
		if ms >= minListenMs {
			qualifyingDates = append(qualifyingDates, dateStr)
		}
	}

	if len(qualifyingDates) == 0 {
		return 0, 0
	}

	slices.Sort(qualifyingDates)

	// Create a set for O(1) lookup
	qualifyingSet := make(map[string]bool)
	for _, d := range qualifyingDates {
		qualifyingSet[d] = true
	}

	// Calculate today and yesterday as date strings
	now := time.Now().In(loc)
	todayStr := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).Format("2006-01-02")
	yesterdayStr := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -1).Format("2006-01-02")

	// Find longest streak
	longestStreak := 1
	currentRun := 1

	for i := 1; i < len(qualifyingDates); i++ {
		currDate, _ := time.Parse("2006-01-02", qualifyingDates[i])
		prevDate, _ := time.Parse("2006-01-02", qualifyingDates[i-1])
		expectedPrev := currDate.AddDate(0, 0, -1)

		if prevDate.Equal(expectedPrev) {
			currentRun++
		} else {
			if currentRun > longestStreak {
				longestStreak = currentRun
			}
			currentRun = 1
		}
	}

	if currentRun > longestStreak {
		longestStreak = currentRun
	}

	// Calculate current streak (must include today or yesterday)
	currentStreak := 0
	lastQualifyingDate := qualifyingDates[len(qualifyingDates)-1]

	if lastQualifyingDate == todayStr || lastQualifyingDate == yesterdayStr {
		// Count backwards from last qualifying date
		currentStreak = 1
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
	}

	return currentStreak, longestStreak
}

// buildStreakCalendar creates the past 12 weeks of streak data.
func (s *StatsService) buildStreakCalendar(ctx context.Context, userID string, now time.Time) []domain.StreakDay {
	const days = 84 // 12 weeks

	// Get events for past 12 weeks
	start := now.AddDate(0, 0, -days).Truncate(24 * time.Hour)
	end := now.Add(24 * time.Hour)

	events, err := s.store.GetEventsForUserInRange(ctx, userID, start, end)
	if err != nil {
		s.logger.Debug("failed to fetch events for streak calendar", "error", err)
		return nil
	}

	// Aggregate by day
	dailyTime := make(map[string]int64)
	for _, e := range events {
		dateKey := e.EndedAt.Format("2006-01-02")
		dailyTime[dateKey] += e.DurationMs
	}

	// Find max for intensity calculation
	var maxTime int64
	for _, ms := range dailyTime {
		if ms > maxTime {
			maxTime = ms
		}
	}

	// Build calendar
	calendar := make([]domain.StreakDay, 0, days)
	for i := days - 1; i >= 0; i-- {
		date := now.AddDate(0, 0, -i).Truncate(24 * time.Hour)
		dateKey := date.Format("2006-01-02")

		ms := dailyTime[dateKey]
		hasListened := ms >= minListenMs

		intensity := 0
		if hasListened && maxTime > 0 {
			// Scale to 1-4 based on relative listening
			ratio := float64(ms) / float64(maxTime)
			intensity = int(ratio*3) + 1
			if intensity > 4 {
				intensity = 4
			}
		}

		calendar = append(calendar, domain.StreakDay{
			Date:         date,
			HasListened:  hasListened,
			ListenTimeMs: ms,
			Intensity:    intensity,
		})
	}

	return calendar
}

// genreDisplayName converts a slug to display name.
func (s *StatsService) genreDisplayName(slug string) string {
	// Convert slug to title case
	words := strings.Split(slug, "-")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
