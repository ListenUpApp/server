package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// makeTestLibrary creates a domain.Library with sensible defaults for testing.
func makeTestLibrary(id, ownerID, name string) *domain.Library {
	now := time.Now()
	return &domain.Library{
		CreatedAt:  now,
		UpdatedAt:  now,
		ID:         id,
		OwnerID:    ownerID,
		Name:       name,
		ScanPaths:  []string{"/media/audiobooks"},
		SkipInbox:  false,
		AccessMode: domain.AccessModeOpen,
	}
}

// createTestOwner inserts a user that can serve as library owner (FK target).
func createTestOwner(t *testing.T, s *Store, id string) {
	t.Helper()
	ctx := context.Background()
	user := makeTestUser(id, id+"@example.com")
	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("createTestOwner(%s): %v", id, err)
	}
}

func TestCreateAndGetLibrary(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	createTestOwner(t, s, "owner-1")

	lib := makeTestLibrary("lib-1", "owner-1", "My Audiobooks")
	lib.ScanPaths = []string{"/media/audiobooks", "/mnt/nas/books"}
	lib.SkipInbox = true
	lib.AccessMode = domain.AccessModeRestricted

	if err := s.CreateLibrary(ctx, lib); err != nil {
		t.Fatalf("CreateLibrary: %v", err)
	}

	got, err := s.GetLibrary(ctx, "lib-1")
	if err != nil {
		t.Fatalf("GetLibrary: %v", err)
	}

	// Verify fields.
	if got.ID != lib.ID {
		t.Errorf("ID: got %q, want %q", got.ID, lib.ID)
	}
	if got.OwnerID != lib.OwnerID {
		t.Errorf("OwnerID: got %q, want %q", got.OwnerID, lib.OwnerID)
	}
	if got.Name != lib.Name {
		t.Errorf("Name: got %q, want %q", got.Name, lib.Name)
	}
	if len(got.ScanPaths) != 2 {
		t.Fatalf("ScanPaths: got %d paths, want 2", len(got.ScanPaths))
	}
	if got.ScanPaths[0] != "/media/audiobooks" {
		t.Errorf("ScanPaths[0]: got %q, want %q", got.ScanPaths[0], "/media/audiobooks")
	}
	if got.ScanPaths[1] != "/mnt/nas/books" {
		t.Errorf("ScanPaths[1]: got %q, want %q", got.ScanPaths[1], "/mnt/nas/books")
	}
	if !got.SkipInbox {
		t.Error("SkipInbox: expected true")
	}
	if got.AccessMode != domain.AccessModeRestricted {
		t.Errorf("AccessMode: got %q, want %q", got.AccessMode, domain.AccessModeRestricted)
	}

	// Timestamps should round-trip through RFC3339Nano.
	if got.CreatedAt.Unix() != lib.CreatedAt.Unix() {
		t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, lib.CreatedAt)
	}
	if got.UpdatedAt.Unix() != lib.UpdatedAt.Unix() {
		t.Errorf("UpdatedAt: got %v, want %v", got.UpdatedAt, lib.UpdatedAt)
	}
}

func TestGetLibrary_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetLibrary(ctx, "nonexistent")
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

func TestCreateLibrary_Duplicate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	createTestOwner(t, s, "owner-dup")

	lib1 := makeTestLibrary("lib-dup", "owner-dup", "Library One")
	if err := s.CreateLibrary(ctx, lib1); err != nil {
		t.Fatalf("CreateLibrary lib1: %v", err)
	}

	// Same ID, different name.
	lib2 := makeTestLibrary("lib-dup", "owner-dup", "Library Two")
	err := s.CreateLibrary(ctx, lib2)
	if err == nil {
		t.Fatal("expected error for duplicate ID, got nil")
	}
	if !errors.Is(err, store.ErrAlreadyExists) {
		t.Errorf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestListLibraries(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	createTestOwner(t, s, "owner-list")

	lib1 := makeTestLibrary("lib-list-1", "owner-list", "First Library")
	lib2 := makeTestLibrary("lib-list-2", "owner-list", "Second Library")
	lib3 := makeTestLibrary("lib-list-3", "owner-list", "Third Library")

	for _, lib := range []*domain.Library{lib1, lib2, lib3} {
		if err := s.CreateLibrary(ctx, lib); err != nil {
			t.Fatalf("CreateLibrary(%s): %v", lib.ID, err)
		}
	}

	libraries, err := s.ListLibraries(ctx)
	if err != nil {
		t.Fatalf("ListLibraries: %v", err)
	}

	if len(libraries) != 3 {
		t.Fatalf("ListLibraries: got %d libraries, want 3", len(libraries))
	}

	// Verify order (by created_at ASC).
	ids := make([]string, len(libraries))
	for i, lib := range libraries {
		ids[i] = lib.ID
	}
	if ids[0] != "lib-list-1" || ids[1] != "lib-list-2" || ids[2] != "lib-list-3" {
		t.Errorf("ListLibraries: got IDs %v, want [lib-list-1 lib-list-2 lib-list-3]", ids)
	}
}

func TestUpdateLibrary(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	createTestOwner(t, s, "owner-update")

	lib := makeTestLibrary("lib-update", "owner-update", "Original Name")
	if err := s.CreateLibrary(ctx, lib); err != nil {
		t.Fatalf("CreateLibrary: %v", err)
	}

	// Modify fields.
	lib.Name = "Updated Name"
	lib.ScanPaths = []string{"/new/path/one", "/new/path/two"}
	lib.SkipInbox = true
	lib.AccessMode = domain.AccessModeRestricted
	lib.UpdatedAt = time.Now()

	if err := s.UpdateLibrary(ctx, lib); err != nil {
		t.Fatalf("UpdateLibrary: %v", err)
	}

	got, err := s.GetLibrary(ctx, "lib-update")
	if err != nil {
		t.Fatalf("GetLibrary after update: %v", err)
	}

	if got.Name != "Updated Name" {
		t.Errorf("Name: got %q, want %q", got.Name, "Updated Name")
	}
	if len(got.ScanPaths) != 2 {
		t.Fatalf("ScanPaths: got %d paths, want 2", len(got.ScanPaths))
	}
	if got.ScanPaths[0] != "/new/path/one" {
		t.Errorf("ScanPaths[0]: got %q, want %q", got.ScanPaths[0], "/new/path/one")
	}
	if got.ScanPaths[1] != "/new/path/two" {
		t.Errorf("ScanPaths[1]: got %q, want %q", got.ScanPaths[1], "/new/path/two")
	}
	if !got.SkipInbox {
		t.Error("SkipInbox: expected true after update")
	}
	if got.AccessMode != domain.AccessModeRestricted {
		t.Errorf("AccessMode: got %q, want %q", got.AccessMode, domain.AccessModeRestricted)
	}
}

func TestUpdateLibrary_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	lib := makeTestLibrary("nonexistent-lib", "owner-x", "Ghost Library")

	err := s.UpdateLibrary(ctx, lib)
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

func TestLibrary_ScanPathsRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	createTestOwner(t, s, "owner-paths")

	lib := makeTestLibrary("lib-paths", "owner-paths", "Multi-Path Library")
	lib.ScanPaths = []string{
		"/media/audiobooks",
		"/mnt/nas/audio",
		"/home/user/books",
		"/tmp/imports",
	}

	if err := s.CreateLibrary(ctx, lib); err != nil {
		t.Fatalf("CreateLibrary: %v", err)
	}

	got, err := s.GetLibrary(ctx, "lib-paths")
	if err != nil {
		t.Fatalf("GetLibrary: %v", err)
	}

	if len(got.ScanPaths) != 4 {
		t.Fatalf("ScanPaths: got %d paths, want 4", len(got.ScanPaths))
	}

	expected := []string{
		"/media/audiobooks",
		"/mnt/nas/audio",
		"/home/user/books",
		"/tmp/imports",
	}
	for i, want := range expected {
		if got.ScanPaths[i] != want {
			t.Errorf("ScanPaths[%d]: got %q, want %q", i, got.ScanPaths[i], want)
		}
	}
}

func TestLibrary_EmptyScanPaths(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	createTestOwner(t, s, "owner-empty")

	lib := makeTestLibrary("lib-empty", "owner-empty", "Empty Paths Library")
	lib.ScanPaths = []string{}

	if err := s.CreateLibrary(ctx, lib); err != nil {
		t.Fatalf("CreateLibrary: %v", err)
	}

	got, err := s.GetLibrary(ctx, "lib-empty")
	if err != nil {
		t.Fatalf("GetLibrary: %v", err)
	}

	if got.ScanPaths == nil {
		t.Fatal("ScanPaths: expected non-nil empty slice, got nil")
	}
	if len(got.ScanPaths) != 0 {
		t.Errorf("ScanPaths: got %d paths, want 0", len(got.ScanPaths))
	}
}
