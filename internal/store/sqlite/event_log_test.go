package sqlite

import (
	"context"
	"testing"
	"time"
)

func TestLogAndReplayEvents(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Log a broadcast event.
	id1, err := s.LogEvent(ctx, "book.created", `{"book_id":"b1"}`, "")
	if err != nil {
		t.Fatalf("log broadcast event: %v", err)
	}
	if id1 == 0 {
		t.Fatal("expected non-zero event ID")
	}

	// Log a user-specific event.
	id2, err := s.LogEvent(ctx, "listening.progress_updated", `{"book_id":"b2"}`, "user-1")
	if err != nil {
		t.Fatalf("log user event: %v", err)
	}

	// Log another user-specific event for a different user.
	_, err = s.LogEvent(ctx, "listening.progress_updated", `{"book_id":"b3"}`, "user-2")
	if err != nil {
		t.Fatalf("log user-2 event: %v", err)
	}

	// Replay for user-1: should see broadcast + their own event, NOT user-2's.
	since := time.Now().UTC().Add(-1 * time.Minute)
	entries, err := s.ReplayEvents(ctx, since, "user-1")
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 events for user-1, got %d", len(entries))
	}
	if entries[0].ID != id1 {
		t.Errorf("expected first event ID %d, got %d", id1, entries[0].ID)
	}
	if entries[1].ID != id2 {
		t.Errorf("expected second event ID %d, got %d", id2, entries[1].ID)
	}
	if entries[0].EventType != "book.created" {
		t.Errorf("expected event type book.created, got %s", entries[0].EventType)
	}
	if entries[0].UserID != "" {
		t.Errorf("expected empty user_id for broadcast, got %q", entries[0].UserID)
	}
	if entries[1].UserID != "user-1" {
		t.Errorf("expected user_id user-1, got %q", entries[1].UserID)
	}

	// Replay for user-2: should see broadcast + their own event.
	entries2, err := s.ReplayEvents(ctx, since, "user-2")
	if err != nil {
		t.Fatalf("replay events for user-2: %v", err)
	}
	if len(entries2) != 2 {
		t.Fatalf("expected 2 events for user-2, got %d", len(entries2))
	}
}

func TestReplayEventsSinceID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id1, err := s.LogEvent(ctx, "book.created", `{"id":"1"}`, "")
	if err != nil {
		t.Fatalf("log event 1: %v", err)
	}

	_, err = s.LogEvent(ctx, "book.updated", `{"id":"2"}`, "")
	if err != nil {
		t.Fatalf("log event 2: %v", err)
	}

	_, err = s.LogEvent(ctx, "listening.progress_updated", `{"id":"3"}`, "user-1")
	if err != nil {
		t.Fatalf("log event 3: %v", err)
	}

	// Replay since id1 — should get events 2 and 3 (for user-1).
	entries, err := s.ReplayEventsSinceID(ctx, id1, "user-1")
	if err != nil {
		t.Fatalf("replay since ID: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 events, got %d", len(entries))
	}
	if entries[0].EventType != "book.updated" {
		t.Errorf("expected book.updated, got %s", entries[0].EventType)
	}
	if entries[1].EventType != "listening.progress_updated" {
		t.Errorf("expected listening.progress_updated, got %s", entries[1].EventType)
	}
}

func TestReplayEventsEmpty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	entries, err := s.ReplayEvents(ctx, time.Now().UTC().Add(-1*time.Hour), "user-1")
	if err != nil {
		t.Fatalf("replay empty: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 events, got %d", len(entries))
	}
}

func TestCleanupEventLog(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Insert an event with a manually backdated timestamp.
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO sse_event_log (event_type, payload, created_at) VALUES (?, ?, ?)",
		"book.created", `{}`, time.Now().UTC().Add(-25*time.Hour).Format(time.RFC3339Nano),
	)
	if err != nil {
		t.Fatalf("insert old event: %v", err)
	}

	// Insert a recent event.
	_, err = s.LogEvent(ctx, "book.updated", `{}`, "")
	if err != nil {
		t.Fatalf("log recent event: %v", err)
	}

	// Cleanup events older than 24h.
	deleted, err := s.CleanupEventLog(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	// Verify only the recent event remains.
	entries, err := s.ReplayEvents(ctx, time.Now().UTC().Add(-1*time.Hour), "")
	if err != nil {
		t.Fatalf("replay after cleanup: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 remaining event, got %d", len(entries))
	}
}

func TestReplayEventsUserIsolation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Log events for different users.
	_, _ = s.LogEvent(ctx, "listening.progress_updated", `{"pos":100}`, "alice")
	_, _ = s.LogEvent(ctx, "listening.progress_updated", `{"pos":200}`, "bob")
	_, _ = s.LogEvent(ctx, "book.created", `{"id":"b1"}`, "") // broadcast

	since := time.Now().UTC().Add(-1 * time.Minute)

	// Alice should see her event + broadcast.
	aliceEntries, err := s.ReplayEvents(ctx, since, "alice")
	if err != nil {
		t.Fatalf("alice replay: %v", err)
	}
	if len(aliceEntries) != 2 {
		t.Fatalf("alice: expected 2 events, got %d", len(aliceEntries))
	}

	// Bob should see his event + broadcast.
	bobEntries, err := s.ReplayEvents(ctx, since, "bob")
	if err != nil {
		t.Fatalf("bob replay: %v", err)
	}
	if len(bobEntries) != 2 {
		t.Fatalf("bob: expected 2 events, got %d", len(bobEntries))
	}

	// Verify alice doesn't see bob's event.
	for _, e := range aliceEntries {
		if e.UserID == "bob" {
			t.Error("alice should not see bob's events")
		}
	}
}

// TestStoreImplementsEventLogger verifies that *Store satisfies sse.EventLogger.
func TestStoreImplementsEventLogger(t *testing.T) {
	s := newTestStore(t)
	// Compile-time check: *Store must implement sse.EventLogger.
	var _ interface {
		LogEvent(ctx context.Context, eventType, payload, userID string) (int64, error)
	} = s
}
