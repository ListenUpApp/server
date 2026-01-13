package abs

import (
	"testing"
	"time"
)

func TestSessionToEvent(t *testing.T) {
	converter := NewConverter()

	session := &Session{
		ID:            "abs-session-1",
		UserID:        "abs-user-1",
		LibraryItemID: "abs-item-1",
		MediaType:     "book",
		StartTime:     0,           // Position at start: 0s
		CurrentTime:   300,         // Position at end: 5min
		Duration:      300,         // 5 minutes listened
		TimeListening: 300,
		StartedAt:     1704067200000, // 2024-01-01 00:00:00 UTC
		UpdatedAt:     1704067500000, // 2024-01-01 00:05:00 UTC (5 min later)
	}

	event := converter.SessionToEvent(session, "lu-user-1", "lu-book-1")

	if event.UserID != "lu-user-1" {
		t.Errorf("UserID = %q, want %q", event.UserID, "lu-user-1")
	}
	if event.BookID != "lu-book-1" {
		t.Errorf("BookID = %q, want %q", event.BookID, "lu-book-1")
	}
	if event.StartPositionMs != 0 {
		t.Errorf("StartPositionMs = %d, want %d", event.StartPositionMs, 0)
	}
	if event.EndPositionMs != 300000 {
		t.Errorf("EndPositionMs = %d, want %d", event.EndPositionMs, 300000)
	}
	if event.DurationMs != 300000 {
		t.Errorf("DurationMs = %d, want %d", event.DurationMs, 300000)
	}
	if event.DeviceID != "abs-import" {
		t.Errorf("DeviceID = %q, want %q", event.DeviceID, "abs-import")
	}
	if event.ID == "" {
		t.Error("ID should not be empty")
	}
}

func TestProgressToEvents(t *testing.T) {
	converter := NewConverter()

	progress := &MediaProgress{
		ID:            "progress-1",
		LibraryItemID: "abs-item-1",
		MediaItemType: "book",
		CurrentTime:   3600, // 1 hour in
		Progress:      0.5,
		IsFinished:    false,
		LastUpdate:    1704067200000,
		StartedAt:     1704000000000,
	}

	events := converter.ProgressToEvents(progress, "lu-user-1", "lu-book-1")

	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.UserID != "lu-user-1" {
		t.Errorf("UserID = %q, want %q", event.UserID, "lu-user-1")
	}
	if event.BookID != "lu-book-1" {
		t.Errorf("BookID = %q, want %q", event.BookID, "lu-book-1")
	}
	if event.StartPositionMs != 0 {
		t.Errorf("StartPositionMs = %d, want %d", event.StartPositionMs, 0)
	}
	if event.EndPositionMs != 3600000 {
		t.Errorf("EndPositionMs = %d, want %d", event.EndPositionMs, 3600000)
	}
}

func TestProgressToEventsNoProgress(t *testing.T) {
	converter := NewConverter()

	progress := &MediaProgress{
		ID:            "progress-1",
		LibraryItemID: "abs-item-1",
		MediaItemType: "book",
		CurrentTime:   0, // No progress
		Progress:      0,
	}

	events := converter.ProgressToEvents(progress, "lu-user-1", "lu-book-1")

	if len(events) != 0 {
		t.Errorf("Expected 0 events for no progress, got %d", len(events))
	}
}

func TestConvertSessions(t *testing.T) {
	converter := NewConverter()

	sessions := []Session{
		{
			ID:            "s1",
			UserID:        "u1",
			LibraryItemID: "i1",
			MediaType:     "book",
			StartTime:     0,
			CurrentTime:   100,
			Duration:      100,
			StartedAt:     1704067200000, // Earlier
			UpdatedAt:     1704067300000,
		},
		{
			ID:            "s2",
			UserID:        "u1",
			LibraryItemID: "i1",
			MediaType:     "podcast", // Should be skipped
			StartTime:     0,
			CurrentTime:   50,
			Duration:      50,
			StartedAt:     1704067400000,
			UpdatedAt:     1704067450000,
		},
		{
			ID:            "s3",
			UserID:        "u1",
			LibraryItemID: "i1",
			MediaType:     "book",
			StartTime:     100,
			CurrentTime:   200,
			Duration:      100,
			StartedAt:     1704067500000, // Later
			UpdatedAt:     1704067600000,
		},
	}

	events := converter.ConvertSessions(sessions, "lu-user", "lu-book")

	// Should have 2 events (podcast skipped)
	if len(events) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(events))
	}

	// Events should be sorted by start time
	if events[0].StartedAt.After(events[1].StartedAt) {
		t.Error("Events should be sorted by StartedAt ascending")
	}
}

func TestCalculateSessionStats(t *testing.T) {
	sessions := []Session{
		{
			ID:            "s1",
			UserID:        "u1",
			LibraryItemID: "i1",
			MediaType:     "book",
			Duration:      300, // 5 min
			StartedAt:     1704067200000,
		},
		{
			ID:            "s2",
			UserID:        "u2",
			LibraryItemID: "i1",
			MediaType:     "book",
			Duration:      600, // 10 min
			StartedAt:     1704153600000, // Next day
		},
		{
			ID:            "s3",
			UserID:        "u1",
			LibraryItemID: "i2",
			MediaType:     "book",
			Duration:      900, // 15 min
			StartedAt:     1704240000000, // Day after
		},
	}

	stats := CalculateSessionStats(sessions)

	if stats.TotalSessions != 3 {
		t.Errorf("TotalSessions = %d, want %d", stats.TotalSessions, 3)
	}
	if stats.TotalDurationMs != 1800000 { // 30 min total
		t.Errorf("TotalDurationMs = %d, want %d", stats.TotalDurationMs, 1800000)
	}
	if stats.UniqueUsers != 2 {
		t.Errorf("UniqueUsers = %d, want %d", stats.UniqueUsers, 2)
	}
	if stats.UniqueBooks != 2 {
		t.Errorf("UniqueBooks = %d, want %d", stats.UniqueBooks, 2)
	}

	// Check earliest/latest
	expectedEarliest := time.UnixMilli(1704067200000)
	expectedLatest := time.UnixMilli(1704240000000)

	if !stats.EarliestSession.Equal(expectedEarliest) {
		t.Errorf("EarliestSession = %v, want %v", stats.EarliestSession, expectedEarliest)
	}
	if !stats.LatestSession.Equal(expectedLatest) {
		t.Errorf("LatestSession = %v, want %v", stats.LatestSession, expectedLatest)
	}
}

func TestCalculateSessionStatsEmpty(t *testing.T) {
	stats := CalculateSessionStats(nil)

	if stats.TotalSessions != 0 {
		t.Errorf("TotalSessions = %d, want 0", stats.TotalSessions)
	}
}
