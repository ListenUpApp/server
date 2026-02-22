package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

func TestCreateAndGetListeningEvent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "user-le-1")
	insertTestBook(t, s, "book-le-1", "Test Book", "/books/le-1")

	now := time.Now().UTC()
	event := &domain.ListeningEvent{
		ID:              "le-1",
		UserID:          "user-le-1",
		BookID:          "book-le-1",
		StartPositionMs: 0,
		EndPositionMs:   60000,
		StartedAt:       now.Add(-2 * time.Minute),
		EndedAt:         now.Add(-1 * time.Minute),
		PlaybackSpeed:   1.5,
		DeviceID:        "device-1",
		DeviceName:      "Simon's iPhone",
		Source:          domain.EventSourcePlayback,
		DurationMs:      60000,
		CreatedAt:       now,
	}

	if err := s.CreateListeningEvent(ctx, event); err != nil {
		t.Fatalf("CreateListeningEvent: %v", err)
	}

	got, err := s.GetListeningEvent(ctx, "le-1")
	if err != nil {
		t.Fatalf("GetListeningEvent: %v", err)
	}

	// Verify all fields.
	if got.ID != event.ID {
		t.Errorf("ID: got %q, want %q", got.ID, event.ID)
	}
	if got.UserID != event.UserID {
		t.Errorf("UserID: got %q, want %q", got.UserID, event.UserID)
	}
	if got.BookID != event.BookID {
		t.Errorf("BookID: got %q, want %q", got.BookID, event.BookID)
	}
	if got.StartPositionMs != event.StartPositionMs {
		t.Errorf("StartPositionMs: got %d, want %d", got.StartPositionMs, event.StartPositionMs)
	}
	if got.EndPositionMs != event.EndPositionMs {
		t.Errorf("EndPositionMs: got %d, want %d", got.EndPositionMs, event.EndPositionMs)
	}
	if got.PlaybackSpeed != event.PlaybackSpeed {
		t.Errorf("PlaybackSpeed: got %v, want %v", got.PlaybackSpeed, event.PlaybackSpeed)
	}
	if got.DeviceID != event.DeviceID {
		t.Errorf("DeviceID: got %q, want %q", got.DeviceID, event.DeviceID)
	}
	if got.DeviceName != event.DeviceName {
		t.Errorf("DeviceName: got %q, want %q", got.DeviceName, event.DeviceName)
	}
	if got.Source != event.Source {
		t.Errorf("Source: got %q, want %q", got.Source, event.Source)
	}
	if got.DurationMs != event.DurationMs {
		t.Errorf("DurationMs: got %d, want %d", got.DurationMs, event.DurationMs)
	}

	// Timestamps should round-trip.
	if got.StartedAt.Unix() != event.StartedAt.Unix() {
		t.Errorf("StartedAt: got %v, want %v", got.StartedAt, event.StartedAt)
	}
	if got.EndedAt.Unix() != event.EndedAt.Unix() {
		t.Errorf("EndedAt: got %v, want %v", got.EndedAt, event.EndedAt)
	}
	if got.CreatedAt.Unix() != event.CreatedAt.Unix() {
		t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, event.CreatedAt)
	}
}

func TestGetListeningEvent_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetListeningEvent(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var storeErr *store.Error
	if !errors.As(err, &storeErr) {
		t.Fatalf("expected *store.Error, got %T: %v", err, err)
	}
	if storeErr.Code != store.ErrNotFound.Code {
		t.Errorf("expected status %d, got %d", store.ErrNotFound.Code, storeErr.Code)
	}
}

func TestGetListeningEventsForBook(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "user-le-book")
	insertTestBook(t, s, "book-le-fb", "Book For Events", "/books/le-fb")

	now := time.Now().UTC()

	event1 := &domain.ListeningEvent{
		ID:              "le-fb-1",
		UserID:          "user-le-book",
		BookID:          "book-le-fb",
		StartPositionMs: 0,
		EndPositionMs:   30000,
		StartedAt:       now.Add(-10 * time.Minute),
		EndedAt:         now.Add(-9 * time.Minute),
		PlaybackSpeed:   1.0,
		DeviceID:        "device-1",
		Source:          domain.EventSourcePlayback,
		DurationMs:      30000,
		CreatedAt:       now.Add(-10 * time.Minute),
	}
	event2 := &domain.ListeningEvent{
		ID:              "le-fb-2",
		UserID:          "user-le-book",
		BookID:          "book-le-fb",
		StartPositionMs: 30000,
		EndPositionMs:   60000,
		StartedAt:       now.Add(-5 * time.Minute),
		EndedAt:         now.Add(-4 * time.Minute),
		PlaybackSpeed:   1.0,
		DeviceID:        "device-1",
		Source:          domain.EventSourcePlayback,
		DurationMs:      30000,
		CreatedAt:       now.Add(-5 * time.Minute),
	}

	if err := s.CreateListeningEvent(ctx, event1); err != nil {
		t.Fatalf("CreateListeningEvent event1: %v", err)
	}
	if err := s.CreateListeningEvent(ctx, event2); err != nil {
		t.Fatalf("CreateListeningEvent event2: %v", err)
	}

	events, err := s.GetListeningEventsForBook(ctx, "book-le-fb")
	if err != nil {
		t.Fatalf("GetListeningEventsForBook: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// Ordered by ended_at DESC, so event2 should be first.
	if events[0].ID != "le-fb-2" {
		t.Errorf("events[0].ID: got %q, want %q", events[0].ID, "le-fb-2")
	}
	if events[1].ID != "le-fb-1" {
		t.Errorf("events[1].ID: got %q, want %q", events[1].ID, "le-fb-1")
	}
}

func TestGetTotalListenTime(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "user-le-total")
	insertTestBook(t, s, "book-le-total", "Book Total Time", "/books/le-total")

	now := time.Now().UTC()

	event1 := &domain.ListeningEvent{
		ID:              "le-total-1",
		UserID:          "user-le-total",
		BookID:          "book-le-total",
		StartPositionMs: 0,
		EndPositionMs:   30000,
		StartedAt:       now.Add(-10 * time.Minute),
		EndedAt:         now.Add(-9 * time.Minute),
		PlaybackSpeed:   1.0,
		DeviceID:        "device-1",
		Source:          domain.EventSourcePlayback,
		DurationMs:      30000,
		CreatedAt:       now.Add(-10 * time.Minute),
	}
	event2 := &domain.ListeningEvent{
		ID:              "le-total-2",
		UserID:          "user-le-total",
		BookID:          "book-le-total",
		StartPositionMs: 30000,
		EndPositionMs:   90000,
		StartedAt:       now.Add(-5 * time.Minute),
		EndedAt:         now.Add(-4 * time.Minute),
		PlaybackSpeed:   1.5,
		DeviceID:        "device-1",
		Source:          domain.EventSourcePlayback,
		DurationMs:      60000,
		CreatedAt:       now.Add(-5 * time.Minute),
	}

	if err := s.CreateListeningEvent(ctx, event1); err != nil {
		t.Fatalf("CreateListeningEvent event1: %v", err)
	}
	if err := s.CreateListeningEvent(ctx, event2); err != nil {
		t.Fatalf("CreateListeningEvent event2: %v", err)
	}

	total, err := s.GetTotalListenTime(ctx, "user-le-total")
	if err != nil {
		t.Fatalf("GetTotalListenTime: %v", err)
	}

	expected := int64(30000 + 60000)
	if total != expected {
		t.Errorf("GetTotalListenTime: got %d, want %d", total, expected)
	}

	// User with no events should return 0.
	insertTestUser(t, s, "user-le-no-events")
	zeroTotal, err := s.GetTotalListenTime(ctx, "user-le-no-events")
	if err != nil {
		t.Fatalf("GetTotalListenTime (no events): %v", err)
	}
	if zeroTotal != 0 {
		t.Errorf("GetTotalListenTime (no events): got %d, want 0", zeroTotal)
	}
}

func TestUpsertAndGetPlaybackState(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "user-ps-1")
	insertTestBook(t, s, "book-ps-1", "State Book", "/books/ps-1")

	now := time.Now().UTC()
	finishedAt := now.Add(-1 * time.Minute)
	state := &domain.PlaybackState{
		UserID:            "user-ps-1",
		BookID:            "book-ps-1",
		CurrentPositionMs: 120000,
		IsFinished:        true,
		FinishedAt:        &finishedAt,
		StartedAt:         now.Add(-1 * time.Hour),
		LastPlayedAt:      now.Add(-1 * time.Minute),
		TotalListenTimeMs: 120000,
		UpdatedAt:         now,
	}

	if err := s.UpsertPlaybackState(ctx, state); err != nil {
		t.Fatalf("UpsertPlaybackState: %v", err)
	}

	got, err := s.GetPlaybackState(ctx, "user-ps-1", "book-ps-1")
	if err != nil {
		t.Fatalf("GetPlaybackState: %v", err)
	}

	// Verify all fields.
	if got.UserID != state.UserID {
		t.Errorf("UserID: got %q, want %q", got.UserID, state.UserID)
	}
	if got.BookID != state.BookID {
		t.Errorf("BookID: got %q, want %q", got.BookID, state.BookID)
	}
	if got.CurrentPositionMs != state.CurrentPositionMs {
		t.Errorf("CurrentPositionMs: got %d, want %d", got.CurrentPositionMs, state.CurrentPositionMs)
	}
	if !got.IsFinished {
		t.Error("IsFinished: expected true")
	}
	if got.FinishedAt == nil {
		t.Fatal("FinishedAt: expected non-nil")
	}
	if got.FinishedAt.Unix() != finishedAt.Unix() {
		t.Errorf("FinishedAt: got %v, want %v", got.FinishedAt, finishedAt)
	}
	if got.TotalListenTimeMs != state.TotalListenTimeMs {
		t.Errorf("TotalListenTimeMs: got %d, want %d", got.TotalListenTimeMs, state.TotalListenTimeMs)
	}

	// Timestamps should round-trip.
	if got.StartedAt.Unix() != state.StartedAt.Unix() {
		t.Errorf("StartedAt: got %v, want %v", got.StartedAt, state.StartedAt)
	}
	if got.LastPlayedAt.Unix() != state.LastPlayedAt.Unix() {
		t.Errorf("LastPlayedAt: got %v, want %v", got.LastPlayedAt, state.LastPlayedAt)
	}
	if got.UpdatedAt.Unix() != state.UpdatedAt.Unix() {
		t.Errorf("UpdatedAt: got %v, want %v", got.UpdatedAt, state.UpdatedAt)
	}
}

func TestGetPlaybackState_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetPlaybackState(ctx, "nonexistent-user", "nonexistent-book")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, store.ErrProgressNotFound) {
		t.Fatalf("expected store.ErrProgressNotFound, got %T: %v", err, err)
	}
}

func TestGetContinueListening(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userID := "user-cl"
	insertTestUser(t, s, userID)
	insertTestBook(t, s, "book-cl-1", "In Progress Book", "/books/cl-1")
	insertTestBook(t, s, "book-cl-2", "Finished Book", "/books/cl-2")
	insertTestBook(t, s, "book-cl-3", "Hidden Book", "/books/cl-3")
	insertTestBook(t, s, "book-cl-4", "Zero Position Book", "/books/cl-4")

	now := time.Now().UTC()

	// State 1: In-progress book with position > 0 (should be returned).
	state1 := &domain.PlaybackState{
		UserID:            userID,
		BookID:            "book-cl-1",
		CurrentPositionMs: 50000,
		IsFinished:        false,
		StartedAt:         now.Add(-2 * time.Hour),
		LastPlayedAt:      now.Add(-10 * time.Minute),
		TotalListenTimeMs: 50000,
		UpdatedAt:         now,
	}

	// State 2: Finished book (should NOT be returned).
	finishedAt := now.Add(-30 * time.Minute)
	state2 := &domain.PlaybackState{
		UserID:            userID,
		BookID:            "book-cl-2",
		CurrentPositionMs: 300000,
		IsFinished:        true,
		FinishedAt:        &finishedAt,
		StartedAt:         now.Add(-3 * time.Hour),
		LastPlayedAt:      now.Add(-30 * time.Minute),
		TotalListenTimeMs: 300000,
		UpdatedAt:         now,
	}

	// State 3: In-progress, not finished, but hidden via book_preferences (should NOT be returned).
	state3 := &domain.PlaybackState{
		UserID:            userID,
		BookID:            "book-cl-3",
		CurrentPositionMs: 25000,
		IsFinished:        false,
		StartedAt:         now.Add(-1 * time.Hour),
		LastPlayedAt:      now.Add(-5 * time.Minute),
		TotalListenTimeMs: 25000,
		UpdatedAt:         now,
	}

	// State 4: Position at 0 (should NOT be returned).
	state4 := &domain.PlaybackState{
		UserID:            userID,
		BookID:            "book-cl-4",
		CurrentPositionMs: 0,
		IsFinished:        false,
		StartedAt:         now.Add(-30 * time.Minute),
		LastPlayedAt:      now.Add(-20 * time.Minute),
		TotalListenTimeMs: 0,
		UpdatedAt:         now,
	}

	for _, st := range []*domain.PlaybackState{state1, state2, state3, state4} {
		if err := s.UpsertPlaybackState(ctx, st); err != nil {
			t.Fatalf("UpsertPlaybackState(%s): %v", st.BookID, err)
		}
	}

	// Hide book-cl-3 via book preferences.
	if err := s.UpsertBookPreferences(ctx, &domain.BookPreferences{
		UserID:                    userID,
		BookID:                    "book-cl-3",
		HideFromContinueListening: true,
		UpdatedAt:                 time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpsertBookPreferences: %v", err)
	}

	results, err := s.GetContinueListening(ctx, userID, 10)
	if err != nil {
		t.Fatalf("GetContinueListening: %v", err)
	}

	// Only state1 should be returned: in-progress, position > 0, not hidden.
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].BookID != "book-cl-1" {
		t.Errorf("BookID: got %q, want %q", results[0].BookID, "book-cl-1")
	}
	if results[0].CurrentPositionMs != 50000 {
		t.Errorf("CurrentPositionMs: got %d, want %d", results[0].CurrentPositionMs, 50000)
	}
}
