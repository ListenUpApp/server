package service

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestStats(t *testing.T) (*StatsService, *store.Store, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "stats-service-test-*")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")
	testStore, err := store.New(dbPath, nil, store.NewNoopEmitter())
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewStatsService(testStore, logger)

	cleanup := func() {
		testStore.Close()
		os.RemoveAll(tmpDir)
	}

	return svc, testStore, cleanup
}

func createTestBook(t *testing.T, s *store.Store, bookID string, durationMs int64) {
	t.Helper()
	ctx := context.Background()

	book := &domain.Book{
		Syncable: domain.Syncable{
			ID: bookID,
		},
		Title:         "Test Book " + bookID,
		TotalDuration: durationMs,
	}
	book.InitTimestamps()
	require.NoError(t, s.CreateBook(ctx, book))
}

func createTestEvent(t *testing.T, s *store.Store, userID, bookID string, durationMs int64, endedAt time.Time) {
	t.Helper()
	ctx := context.Background()

	event := domain.NewListeningEvent(
		"evt-"+bookID+"-"+endedAt.Format("20060102150405"),
		userID,
		bookID,
		0,
		durationMs,
		endedAt.Add(-time.Duration(durationMs)*time.Millisecond),
		endedAt,
		1.0,
		"device-1",
		"Test Device",
	)
	require.NoError(t, s.CreateListeningEvent(ctx, event))
}

func TestGetUserStats_EmptyUser(t *testing.T) {
	svc, _, cleanup := setupTestStats(t)
	defer cleanup()

	ctx := context.Background()

	stats, err := svc.GetUserStats(ctx, "user-no-history", domain.StatsPeriodWeek)
	require.NoError(t, err)

	assert.Equal(t, domain.StatsPeriodWeek, stats.Period)
	assert.Equal(t, int64(0), stats.TotalListenTimeMs)
	assert.Equal(t, 0, stats.BooksStarted)
	assert.Equal(t, 0, stats.BooksFinished)
	assert.Equal(t, 0, stats.CurrentStreakDays)
	assert.Equal(t, 0, stats.LongestStreakDays)
	assert.Empty(t, stats.DailyListening)
	assert.Empty(t, stats.GenreBreakdown)
}

func TestGetUserStats_DailyAggregation(t *testing.T) {
	svc, testStore, cleanup := setupTestStats(t)
	defer cleanup()

	ctx := context.Background()
	userID := "user-daily"

	// Create book
	createTestBook(t, testStore, "book-1", 3600000)

	// Create events on different days
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())
	yesterday := today.AddDate(0, 0, -1)

	// 30 minutes today
	createTestEvent(t, testStore, userID, "book-1", 1800000, today)
	// 15 minutes yesterday
	createTestEvent(t, testStore, userID, "book-1", 900000, yesterday)

	stats, err := svc.GetUserStats(ctx, userID, domain.StatsPeriodWeek)
	require.NoError(t, err)

	assert.Equal(t, int64(2700000), stats.TotalListenTimeMs) // 45 min total
	assert.Len(t, stats.DailyListening, 2)

	// Find today's entry
	var todayEntry, yesterdayEntry *domain.DailyListening
	for i := range stats.DailyListening {
		d := &stats.DailyListening[i]
		if d.Date.Day() == today.Day() {
			todayEntry = d
		}
		if d.Date.Day() == yesterday.Day() {
			yesterdayEntry = d
		}
	}

	require.NotNil(t, todayEntry)
	require.NotNil(t, yesterdayEntry)
	assert.Equal(t, int64(1800000), todayEntry.ListenTimeMs)
	assert.Equal(t, int64(900000), yesterdayEntry.ListenTimeMs)
	assert.Equal(t, 1, todayEntry.BooksListened)
}

func TestGetUserStats_GenreBreakdown(t *testing.T) {
	svc, testStore, cleanup := setupTestStats(t)
	defer cleanup()

	ctx := context.Background()
	userID := "user-genres"

	// Create genres
	fantasy := &domain.Genre{
		Syncable: domain.Syncable{ID: "genre-fantasy"},
		Name:     "Fantasy",
		Slug:     "fantasy",
		Path:     "/fantasy",
	}
	fantasy.InitTimestamps()
	require.NoError(t, testStore.CreateGenre(ctx, fantasy))

	scifi := &domain.Genre{
		Syncable: domain.Syncable{ID: "genre-scifi"},
		Name:     "Science Fiction",
		Slug:     "science-fiction",
		Path:     "/science-fiction",
	}
	scifi.InitTimestamps()
	require.NoError(t, testStore.CreateGenre(ctx, scifi))

	// Create books with genres
	book1 := &domain.Book{
		Syncable:      domain.Syncable{ID: "book-fantasy"},
		Title:         "Fantasy Book",
		TotalDuration: 3600000,
		GenreIDs:      []string{"genre-fantasy"},
	}
	book1.InitTimestamps()
	require.NoError(t, testStore.CreateBook(ctx, book1))
	require.NoError(t, testStore.AddBookGenre(ctx, "book-fantasy", "genre-fantasy"))

	book2 := &domain.Book{
		Syncable:      domain.Syncable{ID: "book-scifi"},
		Title:         "Sci-Fi Book",
		TotalDuration: 3600000,
		GenreIDs:      []string{"genre-scifi"},
	}
	book2.InitTimestamps()
	require.NoError(t, testStore.CreateBook(ctx, book2))
	require.NoError(t, testStore.AddBookGenre(ctx, "book-scifi", "genre-scifi"))

	// Create events: 60 min fantasy, 30 min scifi
	now := time.Now()
	createTestEvent(t, testStore, userID, "book-fantasy", 3600000, now)
	createTestEvent(t, testStore, userID, "book-scifi", 1800000, now)

	stats, err := svc.GetUserStats(ctx, userID, domain.StatsPeriodWeek)
	require.NoError(t, err)

	assert.Len(t, stats.GenreBreakdown, 2)

	// Fantasy should be first (more time)
	assert.Equal(t, "fantasy", stats.GenreBreakdown[0].GenreSlug)
	assert.Equal(t, "Fantasy", stats.GenreBreakdown[0].GenreName)
	assert.Equal(t, int64(3600000), stats.GenreBreakdown[0].ListenTimeMs)

	// Check percentages add up
	totalPct := 0.0
	for _, g := range stats.GenreBreakdown {
		totalPct += g.Percentage
	}
	assert.InDelta(t, 100.0, totalPct, 0.1)
}

func TestGetUserStats_Streaks(t *testing.T) {
	svc, testStore, cleanup := setupTestStats(t)
	defer cleanup()

	ctx := context.Background()
	userID := "user-streaks"

	createTestBook(t, testStore, "book-1", 3600000)

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())

	// Create 5-day streak ending today
	for i := 0; i < 5; i++ {
		day := today.AddDate(0, 0, -i)
		// 1 minute each day (above 30s threshold)
		createTestEvent(t, testStore, userID, "book-1", 60000, day)
	}

	stats, err := svc.GetUserStats(ctx, userID, domain.StatsPeriodWeek)
	require.NoError(t, err)

	assert.Equal(t, 5, stats.CurrentStreakDays)
	assert.Equal(t, 5, stats.LongestStreakDays)
}

func TestGetUserStats_Streaks_BrokenStreak(t *testing.T) {
	svc, testStore, cleanup := setupTestStats(t)
	defer cleanup()

	ctx := context.Background()
	userID := "user-broken-streak"

	createTestBook(t, testStore, "book-1", 3600000)

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())

	// 3 days ago - 7 day streak
	for i := 3; i < 10; i++ {
		day := today.AddDate(0, 0, -i)
		createTestEvent(t, testStore, userID, "book-1", 60000, day)
	}

	// Gap on days -1 and -2

	// Today - new streak starts
	createTestEvent(t, testStore, userID, "book-1", 60000, today)

	stats, err := svc.GetUserStats(ctx, userID, domain.StatsPeriodAllTime)
	require.NoError(t, err)

	// Current streak is just today
	assert.Equal(t, 1, stats.CurrentStreakDays)
	// Longest was 7 days
	assert.Equal(t, 7, stats.LongestStreakDays)
}

func TestGetUserStats_Streaks_MinimumThreshold(t *testing.T) {
	svc, testStore, cleanup := setupTestStats(t)
	defer cleanup()

	ctx := context.Background()
	userID := "user-threshold"

	createTestBook(t, testStore, "book-1", 3600000)

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())

	// Today: 1 minute (counts)
	createTestEvent(t, testStore, userID, "book-1", 60000, today)

	// Yesterday: 20 seconds (doesn't count - below 30s threshold)
	createTestEvent(t, testStore, userID, "book-1", 20000, today.AddDate(0, 0, -1))

	stats, err := svc.GetUserStats(ctx, userID, domain.StatsPeriodWeek)
	require.NoError(t, err)

	// Current streak should be 1 (just today), not 2
	assert.Equal(t, 1, stats.CurrentStreakDays)
}

func TestGetUserStats_PeriodBounds(t *testing.T) {
	svc, testStore, cleanup := setupTestStats(t)
	defer cleanup()

	ctx := context.Background()
	userID := "user-bounds"

	createTestBook(t, testStore, "book-1", 3600000)

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())

	// Event today
	createTestEvent(t, testStore, userID, "book-1", 1800000, today)

	// Event 10 days ago (outside week)
	createTestEvent(t, testStore, userID, "book-1", 1800000, today.AddDate(0, 0, -10))

	// Week stats should only include today's event
	weekStats, err := svc.GetUserStats(ctx, userID, domain.StatsPeriodWeek)
	require.NoError(t, err)
	assert.Equal(t, int64(1800000), weekStats.TotalListenTimeMs)

	// All-time should include both
	allStats, err := svc.GetUserStats(ctx, userID, domain.StatsPeriodAllTime)
	require.NoError(t, err)
	assert.Equal(t, int64(3600000), allStats.TotalListenTimeMs)
}

func TestGetUserStats_StreakCalendar_Intensity(t *testing.T) {
	svc, testStore, cleanup := setupTestStats(t)
	defer cleanup()

	ctx := context.Background()
	userID := "user-intensity"

	createTestBook(t, testStore, "book-1", 3600000)

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())

	// Day 1: 10 minutes
	createTestEvent(t, testStore, userID, "book-1", 600000, today)

	// Day 2: 1 hour (max)
	createTestEvent(t, testStore, userID, "book-1", 3600000, today.AddDate(0, 0, -1))

	// Day 3: 5 minutes
	createTestEvent(t, testStore, userID, "book-1", 300000, today.AddDate(0, 0, -2))

	stats, err := svc.GetUserStats(ctx, userID, domain.StatsPeriodWeek)
	require.NoError(t, err)

	require.NotEmpty(t, stats.StreakCalendar)

	// Find the days in the calendar
	intensities := make(map[string]int)
	for _, day := range stats.StreakCalendar {
		dateKey := day.Date.Format("2006-01-02")
		intensities[dateKey] = day.Intensity
	}

	// The 1-hour day should have intensity 4 (max)
	yesterdayKey := today.AddDate(0, 0, -1).Format("2006-01-02")
	assert.Equal(t, 4, intensities[yesterdayKey])
}

func TestGetUserStats_BooksStartedAndFinished(t *testing.T) {
	svc, testStore, cleanup := setupTestStats(t)
	defer cleanup()

	ctx := context.Background()
	userID := "user-books"

	// Create books
	for i := 1; i <= 3; i++ {
		createTestBook(t, testStore, "book-"+string(rune('0'+i)), 3600000)
	}

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())

	// Book 1: Started and finished today (99% progress)
	event1 := domain.NewListeningEvent(
		"evt-book1-finish",
		userID,
		"book-1",
		0,
		3564000, // 99%
		today.Add(-60*time.Minute),
		today,
		1.0,
		"device-1",
		"Test Device",
	)
	require.NoError(t, testStore.CreateListeningEvent(ctx, event1))

	// Create progress for book 1
	progress1 := domain.NewPlaybackProgress(event1, 3600000)
	require.NoError(t, testStore.UpsertProgress(ctx, progress1))

	// Book 2: Just started
	event2 := domain.NewListeningEvent(
		"evt-book2-start",
		userID,
		"book-2",
		0,
		600000, // 10 min
		today.Add(-30*time.Minute),
		today.Add(-20*time.Minute),
		1.0,
		"device-1",
		"Test Device",
	)
	require.NoError(t, testStore.CreateListeningEvent(ctx, event2))

	progress2 := domain.NewPlaybackProgress(event2, 3600000)
	require.NoError(t, testStore.UpsertProgress(ctx, progress2))

	stats, err := svc.GetUserStats(ctx, userID, domain.StatsPeriodWeek)
	require.NoError(t, err)

	assert.Equal(t, 2, stats.BooksStarted)
	assert.Equal(t, 1, stats.BooksFinished)
}

func TestStatsPeriod_Bounds(t *testing.T) {
	// Test on a Wednesday, 2024-01-10 15:30:00
	now := time.Date(2024, 1, 10, 15, 30, 0, 0, time.UTC)

	tests := []struct {
		period    domain.StatsPeriod
		wantStart time.Time
		wantEnd   time.Time
	}{
		{
			period:    domain.StatsPeriodDay,
			wantStart: time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2024, 1, 11, 0, 0, 0, 0, time.UTC),
		},
		{
			period:    domain.StatsPeriodWeek,
			wantStart: time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC), // Monday
			wantEnd:   time.Date(2024, 1, 11, 0, 0, 0, 0, time.UTC),
		},
		{
			period:    domain.StatsPeriodMonth,
			wantStart: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2024, 1, 11, 0, 0, 0, 0, time.UTC),
		},
		{
			period:    domain.StatsPeriodYear,
			wantStart: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2024, 1, 11, 0, 0, 0, 0, time.UTC),
		},
		{
			period:    domain.StatsPeriodAllTime,
			wantStart: time.Time{}, // Zero time
			wantEnd:   time.Date(2024, 1, 11, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.period), func(t *testing.T) {
			start, end := tt.period.Bounds(now)
			assert.Equal(t, tt.wantStart, start)
			assert.Equal(t, tt.wantEnd, end)
		})
	}
}

func TestStatsPeriod_Valid(t *testing.T) {
	tests := []struct {
		period domain.StatsPeriod
		want   bool
	}{
		{domain.StatsPeriodDay, true},
		{domain.StatsPeriodWeek, true},
		{domain.StatsPeriodMonth, true},
		{domain.StatsPeriodYear, true},
		{domain.StatsPeriodAllTime, true},
		{domain.StatsPeriod("invalid"), false},
		{domain.StatsPeriod(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.period), func(t *testing.T) {
			assert.Equal(t, tt.want, tt.period.Valid())
		})
	}
}
