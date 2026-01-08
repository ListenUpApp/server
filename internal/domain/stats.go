package domain

import "time"

// StatsPeriod represents a time window for statistics queries.
type StatsPeriod string

// StatsPeriod constants for time window queries.
const (
	StatsPeriodDay     StatsPeriod = "day"
	StatsPeriodWeek    StatsPeriod = "week"
	StatsPeriodMonth   StatsPeriod = "month"
	StatsPeriodYear    StatsPeriod = "year"
	StatsPeriodAllTime StatsPeriod = "all"
)

// Valid returns true if the period is a recognized value.
func (p StatsPeriod) Valid() bool {
	switch p {
	case StatsPeriodDay, StatsPeriodWeek, StatsPeriodMonth, StatsPeriodYear, StatsPeriodAllTime:
		return true
	default:
		return false
	}
}

// Bounds returns the start and end times for a period relative to now.
// Start is inclusive, end is exclusive. End is always end of today (midnight tomorrow).
func (p StatsPeriod) Bounds(now time.Time) (start, end time.Time) {
	// Normalize to start of day in local time
	year, month, day := now.Date()
	loc := now.Location()
	today := time.Date(year, month, day, 0, 0, 0, 0, loc)
	endOfToday := today.Add(24 * time.Hour)

	switch p {
	case StatsPeriodDay:
		return today, endOfToday
	case StatsPeriodWeek:
		// Week starts on Monday (ISO standard)
		weekday := int(today.Weekday())
		if weekday == 0 {
			weekday = 7 // Sunday = 7
		}
		startOfWeek := today.AddDate(0, 0, -(weekday - 1))
		return startOfWeek, endOfToday
	case StatsPeriodMonth:
		startOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, loc)
		return startOfMonth, endOfToday
	case StatsPeriodYear:
		startOfYear := time.Date(year, 1, 1, 0, 0, 0, 0, loc)
		return startOfYear, endOfToday
	case StatsPeriodAllTime:
		return time.Time{}, endOfToday // Zero time = beginning of time
	default:
		return today, endOfToday
	}
}

// DailyListening represents listening activity for a single day.
type DailyListening struct {
	Date          time.Time `json:"date"`
	ListenTimeMs  int64     `json:"listen_time_ms"`
	BooksListened int       `json:"books_listened"` // Distinct books listened to
}

// GenreListening represents listening time for a genre.
type GenreListening struct {
	GenreSlug    string  `json:"genre_slug"`
	GenreName    string  `json:"genre_name"`
	ListenTimeMs int64   `json:"listen_time_ms"`
	Percentage   float64 `json:"percentage"` // 0-100
}

// StreakDay represents a single day in the streak calendar.
type StreakDay struct {
	Date         time.Time `json:"date"`
	HasListened  bool      `json:"has_listened"`
	ListenTimeMs int64     `json:"listen_time_ms"`
	Intensity    int       `json:"intensity"` // 0-4 for visual gradient (0=none, 4=max)
}

// UserStatsDetailed contains comprehensive listening statistics.
type UserStatsDetailed struct {
	// Query context
	Period    StatsPeriod `json:"period"`
	StartDate time.Time   `json:"start_date"`
	EndDate   time.Time   `json:"end_date"`

	// Headline numbers
	TotalListenTimeMs int64 `json:"total_listen_time_ms"`
	BooksStarted      int   `json:"books_started"`
	BooksFinished     int   `json:"books_finished"`

	// Streaks
	CurrentStreakDays int `json:"current_streak_days"`
	LongestStreakDays int `json:"longest_streak_days"`

	// Chart data
	DailyListening []DailyListening `json:"daily_listening"`
	GenreBreakdown []GenreListening `json:"genre_breakdown"`

	// Streak calendar: past 12 weeks (84 days)
	StreakCalendar []StreakDay `json:"streak_calendar,omitempty"`
}
