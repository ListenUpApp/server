package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

func TestCreateAndGetReadingSession(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "user-rs-1")
	insertTestBook(t, s, "book-rs-1", "Reading Session Book", "/books/rs-1")

	now := time.Now().UTC()
	session := &domain.BookReadingSession{
		ID:            "rs-1",
		UserID:        "user-rs-1",
		BookID:        "book-rs-1",
		StartedAt:     now.Add(-1 * time.Hour),
		FinishedAt:    nil,
		IsCompleted:   false,
		FinalProgress: 0.45,
		ListenTimeMs:  90000,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.CreateReadingSession(ctx, session); err != nil {
		t.Fatalf("CreateReadingSession: %v", err)
	}

	sessions, err := s.GetReadingSessions(ctx, "user-rs-1", "book-rs-1")
	if err != nil {
		t.Fatalf("GetReadingSessions: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	got := sessions[0]

	// Verify all fields.
	if got.ID != session.ID {
		t.Errorf("ID: got %q, want %q", got.ID, session.ID)
	}
	if got.UserID != session.UserID {
		t.Errorf("UserID: got %q, want %q", got.UserID, session.UserID)
	}
	if got.BookID != session.BookID {
		t.Errorf("BookID: got %q, want %q", got.BookID, session.BookID)
	}
	if got.FinishedAt != nil {
		t.Errorf("FinishedAt: expected nil, got %v", got.FinishedAt)
	}
	if got.IsCompleted {
		t.Error("IsCompleted: expected false")
	}
	if got.FinalProgress != session.FinalProgress {
		t.Errorf("FinalProgress: got %v, want %v", got.FinalProgress, session.FinalProgress)
	}
	if got.ListenTimeMs != session.ListenTimeMs {
		t.Errorf("ListenTimeMs: got %d, want %d", got.ListenTimeMs, session.ListenTimeMs)
	}

	// Timestamps should round-trip.
	if got.StartedAt.Unix() != session.StartedAt.Unix() {
		t.Errorf("StartedAt: got %v, want %v", got.StartedAt, session.StartedAt)
	}
	if got.CreatedAt.Unix() != session.CreatedAt.Unix() {
		t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, session.CreatedAt)
	}
	if got.UpdatedAt.Unix() != session.UpdatedAt.Unix() {
		t.Errorf("UpdatedAt: got %v, want %v", got.UpdatedAt, session.UpdatedAt)
	}
}

func TestGetActiveReadingSession(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "user-rs-active")
	insertTestBook(t, s, "book-rs-active", "Active Session Book", "/books/rs-active")

	now := time.Now().UTC()

	// Create an active session (FinishedAt is nil).
	session := &domain.BookReadingSession{
		ID:            "rs-active-1",
		UserID:        "user-rs-active",
		BookID:        "book-rs-active",
		StartedAt:     now.Add(-30 * time.Minute),
		FinishedAt:    nil,
		IsCompleted:   false,
		FinalProgress: 0.25,
		ListenTimeMs:  45000,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.CreateReadingSession(ctx, session); err != nil {
		t.Fatalf("CreateReadingSession: %v", err)
	}

	got, err := s.GetActiveReadingSession(ctx, "user-rs-active", "book-rs-active")
	if err != nil {
		t.Fatalf("GetActiveReadingSession: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil active session, got nil")
	}
	if got.ID != "rs-active-1" {
		t.Errorf("ID: got %q, want %q", got.ID, "rs-active-1")
	}
	if got.FinishedAt != nil {
		t.Errorf("FinishedAt: expected nil, got %v", got.FinishedAt)
	}
}

func TestGetActiveReadingSession_None(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "user-rs-none")
	insertTestBook(t, s, "book-rs-none", "No Active Session Book", "/books/rs-none")

	// No sessions created at all.
	got, err := s.GetActiveReadingSession(ctx, "user-rs-none", "book-rs-none")
	if err != nil {
		t.Fatalf("GetActiveReadingSession: unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestUpdateReadingSession(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "user-rs-upd")
	insertTestBook(t, s, "book-rs-upd", "Update Session Book", "/books/rs-upd")

	now := time.Now().UTC()

	// Create an active session.
	session := &domain.BookReadingSession{
		ID:            "rs-upd-1",
		UserID:        "user-rs-upd",
		BookID:        "book-rs-upd",
		StartedAt:     now.Add(-1 * time.Hour),
		FinishedAt:    nil,
		IsCompleted:   false,
		FinalProgress: 0.50,
		ListenTimeMs:  60000,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.CreateReadingSession(ctx, session); err != nil {
		t.Fatalf("CreateReadingSession: %v", err)
	}

	// Update to finish the session.
	finishedAt := now.Add(10 * time.Minute)
	later := now.Add(10 * time.Minute)
	session.FinishedAt = &finishedAt
	session.IsCompleted = true
	session.FinalProgress = 1.0
	session.ListenTimeMs = 180000
	session.UpdatedAt = later

	if err := s.UpdateReadingSession(ctx, session); err != nil {
		t.Fatalf("UpdateReadingSession: %v", err)
	}

	sessions, err := s.GetReadingSessions(ctx, "user-rs-upd", "book-rs-upd")
	if err != nil {
		t.Fatalf("GetReadingSessions after update: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	got := sessions[0]
	if !got.IsCompleted {
		t.Error("IsCompleted: expected true after update")
	}
	if got.FinalProgress != 1.0 {
		t.Errorf("FinalProgress: got %v, want 1.0", got.FinalProgress)
	}
	if got.ListenTimeMs != 180000 {
		t.Errorf("ListenTimeMs: got %d, want 180000", got.ListenTimeMs)
	}
	if got.FinishedAt == nil {
		t.Fatal("FinishedAt: expected non-nil after update")
	}
	if got.FinishedAt.Unix() != finishedAt.Unix() {
		t.Errorf("FinishedAt: got %v, want %v", got.FinishedAt, finishedAt)
	}
	if got.UpdatedAt.Unix() != later.Unix() {
		t.Errorf("UpdatedAt: got %v, want %v", got.UpdatedAt, later)
	}

	// The active session query should return nil now that it is finished.
	active, err := s.GetActiveReadingSession(ctx, "user-rs-upd", "book-rs-upd")
	if err != nil {
		t.Fatalf("GetActiveReadingSession after finish: %v", err)
	}
	if active != nil {
		t.Errorf("expected nil active session after finish, got %+v", active)
	}
}

func TestUpdateReadingSession_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC()
	session := &domain.BookReadingSession{
		ID:            "rs-nonexistent",
		UserID:        "user-x",
		BookID:        "book-x",
		StartedAt:     now,
		IsCompleted:   false,
		FinalProgress: 0,
		ListenTimeMs:  0,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	err := s.UpdateReadingSession(ctx, session)
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
