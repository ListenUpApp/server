package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

func TestUpsertAndGetBookPreferences(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "user-bp-1")
	insertTestBook(t, s, "book-bp-1", "Preferences Book", "/books/bp-1")

	now := time.Now().UTC()
	speed := float32(1.75)
	skip := 15

	prefs := &domain.BookPreferences{
		UserID:                    "user-bp-1",
		BookID:                    "book-bp-1",
		PlaybackSpeed:             &speed,
		SkipForwardSec:            &skip,
		HideFromContinueListening: true,
		UpdatedAt:                 now,
	}

	if err := s.UpsertBookPreferences(ctx, prefs); err != nil {
		t.Fatalf("UpsertBookPreferences: %v", err)
	}

	got, err := s.GetBookPreferences(ctx, "user-bp-1", "book-bp-1")
	if err != nil {
		t.Fatalf("GetBookPreferences: %v", err)
	}

	// Verify all fields.
	if got.UserID != prefs.UserID {
		t.Errorf("UserID: got %q, want %q", got.UserID, prefs.UserID)
	}
	if got.BookID != prefs.BookID {
		t.Errorf("BookID: got %q, want %q", got.BookID, prefs.BookID)
	}
	if got.PlaybackSpeed == nil {
		t.Fatal("PlaybackSpeed: expected non-nil")
	}
	if *got.PlaybackSpeed != speed {
		t.Errorf("PlaybackSpeed: got %v, want %v", *got.PlaybackSpeed, speed)
	}
	if got.SkipForwardSec == nil {
		t.Fatal("SkipForwardSec: expected non-nil")
	}
	if *got.SkipForwardSec != skip {
		t.Errorf("SkipForwardSec: got %d, want %d", *got.SkipForwardSec, skip)
	}
	if !got.HideFromContinueListening {
		t.Error("HideFromContinueListening: expected true")
	}
	if got.UpdatedAt.Unix() != now.Unix() {
		t.Errorf("UpdatedAt: got %v, want %v", got.UpdatedAt, now)
	}
}

func TestGetBookPreferences_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetBookPreferences(ctx, "nonexistent-user", "nonexistent-book")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, store.ErrBookPreferencesNotFound) {
		t.Fatalf("expected store.ErrBookPreferencesNotFound, got %T: %v", err, err)
	}
}

func TestUpsertBookPreferences_NilOptionalFields(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "user-bp-nil")
	insertTestBook(t, s, "book-bp-nil", "Nil Fields Book", "/books/bp-nil")

	now := time.Now().UTC()
	prefs := &domain.BookPreferences{
		UserID:                    "user-bp-nil",
		BookID:                    "book-bp-nil",
		PlaybackSpeed:             nil,
		SkipForwardSec:            nil,
		HideFromContinueListening: false,
		UpdatedAt:                 now,
	}

	if err := s.UpsertBookPreferences(ctx, prefs); err != nil {
		t.Fatalf("UpsertBookPreferences: %v", err)
	}

	got, err := s.GetBookPreferences(ctx, "user-bp-nil", "book-bp-nil")
	if err != nil {
		t.Fatalf("GetBookPreferences: %v", err)
	}

	if got.PlaybackSpeed != nil {
		t.Errorf("PlaybackSpeed: expected nil, got %v", *got.PlaybackSpeed)
	}
	if got.SkipForwardSec != nil {
		t.Errorf("SkipForwardSec: expected nil, got %d", *got.SkipForwardSec)
	}
	if got.HideFromContinueListening {
		t.Error("HideFromContinueListening: expected false")
	}
}

func TestUpsertBookPreferences_Update(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "user-bp-upd")
	insertTestBook(t, s, "book-bp-upd", "Update Prefs Book", "/books/bp-upd")

	now := time.Now().UTC()
	speed1 := float32(1.0)
	skip1 := 10

	// First upsert.
	prefs := &domain.BookPreferences{
		UserID:                    "user-bp-upd",
		BookID:                    "book-bp-upd",
		PlaybackSpeed:             &speed1,
		SkipForwardSec:            &skip1,
		HideFromContinueListening: false,
		UpdatedAt:                 now,
	}

	if err := s.UpsertBookPreferences(ctx, prefs); err != nil {
		t.Fatalf("UpsertBookPreferences (first): %v", err)
	}

	// Second upsert with different values.
	speed2 := float32(2.0)
	skip2 := 30
	later := now.Add(5 * time.Minute)

	prefs2 := &domain.BookPreferences{
		UserID:                    "user-bp-upd",
		BookID:                    "book-bp-upd",
		PlaybackSpeed:             &speed2,
		SkipForwardSec:            &skip2,
		HideFromContinueListening: true,
		UpdatedAt:                 later,
	}

	if err := s.UpsertBookPreferences(ctx, prefs2); err != nil {
		t.Fatalf("UpsertBookPreferences (second): %v", err)
	}

	got, err := s.GetBookPreferences(ctx, "user-bp-upd", "book-bp-upd")
	if err != nil {
		t.Fatalf("GetBookPreferences after update: %v", err)
	}

	// Verify updated values.
	if got.PlaybackSpeed == nil {
		t.Fatal("PlaybackSpeed: expected non-nil")
	}
	if *got.PlaybackSpeed != speed2 {
		t.Errorf("PlaybackSpeed: got %v, want %v", *got.PlaybackSpeed, speed2)
	}
	if got.SkipForwardSec == nil {
		t.Fatal("SkipForwardSec: expected non-nil")
	}
	if *got.SkipForwardSec != skip2 {
		t.Errorf("SkipForwardSec: got %d, want %d", *got.SkipForwardSec, skip2)
	}
	if !got.HideFromContinueListening {
		t.Error("HideFromContinueListening: expected true after update")
	}
	if got.UpdatedAt.Unix() != later.Unix() {
		t.Errorf("UpdatedAt: got %v, want %v", got.UpdatedAt, later)
	}
}
