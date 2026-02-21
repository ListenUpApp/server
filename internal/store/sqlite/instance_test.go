package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

func TestSetAndGetInstanceKey(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.SetInstanceKey(ctx, "server_name", "My Audiobook Server"); err != nil {
		t.Fatalf("SetInstanceKey: %v", err)
	}

	got, err := s.GetInstanceKey(ctx, "server_name")
	if err != nil {
		t.Fatalf("GetInstanceKey: %v", err)
	}

	if got != "My Audiobook Server" {
		t.Errorf("value: got %q, want %q", got, "My Audiobook Server")
	}
}

func TestGetInstanceKey_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetInstanceKey(ctx, "nonexistent_key")
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

func TestSetInstanceKey_Overwrite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.SetInstanceKey(ctx, "version", "1.0.0"); err != nil {
		t.Fatalf("SetInstanceKey (first): %v", err)
	}

	// Overwrite with a new value.
	if err := s.SetInstanceKey(ctx, "version", "2.0.0"); err != nil {
		t.Fatalf("SetInstanceKey (second): %v", err)
	}

	got, err := s.GetInstanceKey(ctx, "version")
	if err != nil {
		t.Fatalf("GetInstanceKey: %v", err)
	}

	if got != "2.0.0" {
		t.Errorf("value: got %q, want %q", got, "2.0.0")
	}
}

func TestGetServerSettings_Defaults(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Before any update, should return defaults.
	got, err := s.GetServerSettings(ctx)
	if err != nil {
		t.Fatalf("GetServerSettings: %v", err)
	}

	defaults := domain.NewServerSettings()

	if got.InboxEnabled != defaults.InboxEnabled {
		t.Errorf("InboxEnabled: got %v, want %v", got.InboxEnabled, defaults.InboxEnabled)
	}
	if got.InboxEnabled != false {
		t.Errorf("InboxEnabled: expected false for defaults, got %v", got.InboxEnabled)
	}
}

func TestUpdateAndGetServerSettings(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	settings := &domain.ServerSettings{
		InboxEnabled: true,
		UpdatedAt:    time.Now(),
	}

	if err := s.UpdateServerSettings(ctx, settings); err != nil {
		t.Fatalf("UpdateServerSettings: %v", err)
	}

	got, err := s.GetServerSettings(ctx)
	if err != nil {
		t.Fatalf("GetServerSettings: %v", err)
	}

	if got.InboxEnabled != true {
		t.Errorf("InboxEnabled: got %v, want true", got.InboxEnabled)
	}

	// Update again to disable.
	settings.InboxEnabled = false
	settings.UpdatedAt = time.Now()

	if err := s.UpdateServerSettings(ctx, settings); err != nil {
		t.Fatalf("UpdateServerSettings (second): %v", err)
	}

	got, err = s.GetServerSettings(ctx)
	if err != nil {
		t.Fatalf("GetServerSettings after second update: %v", err)
	}

	if got.InboxEnabled != false {
		t.Errorf("InboxEnabled: got %v, want false", got.InboxEnabled)
	}
}
