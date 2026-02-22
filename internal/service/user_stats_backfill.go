package service

import (
	"context"
	"log/slog"
	"time"

	"slices"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// BackfillUserStats computes user_stats from existing events/progress for all users.
// Called at startup if the user_stats table is empty.
func BackfillUserStats(ctx context.Context, st store.Store, logger *slog.Logger) error {
	// Check if any stats exist already
	existing, err := st.GetAllUserStats(ctx)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		logger.Info("user_stats already populated, skipping backfill", "count", len(existing))
		return nil
	}

	users, err := st.ListUsers(ctx)
	if err != nil {
		return err
	}

	logger.Info("backfilling user_stats", "users", len(users))

	const maxReasonableDurationMs = 24 * 60 * 60 * 1000

	for _, user := range users {
		// Calculate total listen time from events
		events, err := st.GetEventsForUser(ctx, user.ID)
		if err != nil {
			logger.Warn("backfill: failed to get events", "user_id", user.ID, "error", err)
			continue
		}

		var totalTimeMs int64
		for _, e := range events {
			if e.DurationMs > 0 && e.DurationMs <= maxReasonableDurationMs {
				totalTimeMs += e.DurationMs
			}
		}

		// Count finished books from progress
		allProgress, err := st.GetStateForUser(ctx, user.ID)
		if err != nil {
			logger.Warn("backfill: failed to get progress", "user_id", user.ID, "error", err)
			continue
		}

		var booksFinished int
		for _, p := range allProgress {
			if p.IsFinished {
				booksFinished++
			}
		}

		// Calculate streak and last listened date
		loc := time.Local
		var lastListenedDate string
		var currentStreak, longestStreak int
		if len(events) > 0 {
			// Find latest event date
			var latest time.Time
			for _, e := range events {
				if e.EndedAt.After(latest) {
					latest = e.EndedAt
				}
			}
			lastListenedDate = latest.In(loc).Format("2006-01-02")

			// Calculate streaks from qualifying dates
			qualifyingDates := computeQualifyingDates(events, loc)
			currentStreak, longestStreak = computeStreaks(qualifyingDates, loc)
		}

		stats := &domain.UserStats{
			UserID:             user.ID,
			TotalListenTimeMs:  totalTimeMs,
			TotalBooksFinished: booksFinished,
			CurrentStreakDays:  currentStreak,
			LongestStreakDays:  longestStreak,
			LastListenedDate:   lastListenedDate,
			UpdatedAt:          time.Now(),
		}

		if err := st.SetUserStats(ctx, stats); err != nil {
			logger.Warn("backfill: failed to save stats", "user_id", user.ID, "error", err)
			continue
		}

		logger.Info("backfilled user stats",
			"user_id", user.ID,
			"total_time_ms", totalTimeMs,
			"books_finished", booksFinished,
		)
	}

	return nil
}

const backfillMinListenMsForStreak = 30 * 1000

// computeQualifyingDates returns sorted dates where user listened >= threshold.
func computeQualifyingDates(events []*domain.ListeningEvent, loc *time.Location) []string {
	dailyTime := make(map[string]int64)
	for _, e := range events {
		dateKey := e.EndedAt.In(loc).Format("2006-01-02")
		dailyTime[dateKey] += e.DurationMs
	}

	var dates []string
	for dateStr, ms := range dailyTime {
		if ms >= backfillMinListenMsForStreak {
			dates = append(dates, dateStr)
		}
	}
	slices.Sort(dates)
	return dates
}

// computeStreaks calculates current and longest streak from sorted qualifying dates.
// Uses AddDate for DST-safe day arithmetic and map for O(1) lookups.
func computeStreaks(qualifyingDates []string, loc *time.Location) (current, longest int) {
	if len(qualifyingDates) == 0 {
		return 0, 0
	}

	// Build set for O(1) lookup
	dateSet := make(map[string]bool, len(qualifyingDates))
	for _, d := range qualifyingDates {
		dateSet[d] = true
	}

	// Calculate longest streak using AddDate (DST-safe)
	streak := 1
	longest = 1
	for i := 1; i < len(qualifyingDates); i++ {
		prev, _ := time.ParseInLocation("2006-01-02", qualifyingDates[i-1], loc)
		nextDay := prev.AddDate(0, 0, 1).Format("2006-01-02")
		if qualifyingDates[i] == nextDay {
			streak++
			if streak > longest {
				longest = streak
			}
		} else {
			streak = 1
		}
	}

	// Calculate current streak
	now := time.Now().In(loc)
	todayStr := now.Format("2006-01-02")
	yesterdayStr := now.AddDate(0, 0, -1).Format("2006-01-02")

	lastDate := qualifyingDates[len(qualifyingDates)-1]
	if lastDate != todayStr && lastDate != yesterdayStr {
		return 0, longest
	}

	current = 1
	checkDate, _ := time.ParseInLocation("2006-01-02", lastDate, loc)
	checkDate = checkDate.AddDate(0, 0, -1)
	for {
		checkStr := checkDate.Format("2006-01-02")
		if dateSet[checkStr] {
			current++
			checkDate = checkDate.AddDate(0, 0, -1)
		} else {
			break
		}
	}

	return current, longest
}
