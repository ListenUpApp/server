package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

func TestCreateAndGetInvite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "admin-1")

	now := time.Now()
	invite := &domain.Invite{
		Syncable: domain.Syncable{
			ID:        "inv-1",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Code:      "ABC123",
		Name:      "Alice",
		Email:     "alice@example.com",
		Role:      domain.Role("member"),
		CreatedBy: "admin-1",
		ExpiresAt: now.Add(72 * time.Hour),
	}

	if err := s.CreateInvite(ctx, invite); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	got, err := s.GetInvite(ctx, "inv-1")
	if err != nil {
		t.Fatalf("GetInvite: %v", err)
	}

	// Verify all fields.
	if got.ID != invite.ID {
		t.Errorf("ID: got %q, want %q", got.ID, invite.ID)
	}
	if got.Code != invite.Code {
		t.Errorf("Code: got %q, want %q", got.Code, invite.Code)
	}
	if got.Name != invite.Name {
		t.Errorf("Name: got %q, want %q", got.Name, invite.Name)
	}
	if got.Email != invite.Email {
		t.Errorf("Email: got %q, want %q", got.Email, invite.Email)
	}
	if got.Role != domain.Role("member") {
		t.Errorf("Role: got %q, want %q", got.Role, domain.Role("member"))
	}
	if got.CreatedBy != invite.CreatedBy {
		t.Errorf("CreatedBy: got %q, want %q", got.CreatedBy, invite.CreatedBy)
	}
	if got.ExpiresAt.Unix() != invite.ExpiresAt.Unix() {
		t.Errorf("ExpiresAt: got %v, want %v", got.ExpiresAt, invite.ExpiresAt)
	}
	if got.ClaimedAt != nil {
		t.Errorf("ClaimedAt: expected nil, got %v", got.ClaimedAt)
	}
	if got.ClaimedBy != "" {
		t.Errorf("ClaimedBy: expected empty, got %q", got.ClaimedBy)
	}
	if got.DeletedAt != nil {
		t.Error("DeletedAt: expected nil")
	}

	// Timestamps should round-trip.
	if got.CreatedAt.Unix() != invite.CreatedAt.Unix() {
		t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, invite.CreatedAt)
	}
	if got.UpdatedAt.Unix() != invite.UpdatedAt.Unix() {
		t.Errorf("UpdatedAt: got %v, want %v", got.UpdatedAt, invite.UpdatedAt)
	}
}

func TestGetInvite_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetInvite(ctx, "nonexistent")
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

func TestGetInviteByToken(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "admin-token")

	now := time.Now()
	invite := &domain.Invite{
		Syncable: domain.Syncable{
			ID:        "inv-token-1",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Code:      "TOKEN-XYZ",
		Name:      "Bob",
		Email:     "bob@example.com",
		Role:      domain.Role("admin"),
		CreatedBy: "admin-token",
		ExpiresAt: now.Add(48 * time.Hour),
	}

	if err := s.CreateInvite(ctx, invite); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	got, err := s.GetInviteByToken(ctx, "TOKEN-XYZ")
	if err != nil {
		t.Fatalf("GetInviteByToken: %v", err)
	}

	if got.ID != "inv-token-1" {
		t.Errorf("ID: got %q, want %q", got.ID, "inv-token-1")
	}
	if got.Code != "TOKEN-XYZ" {
		t.Errorf("Code: got %q, want %q", got.Code, "TOKEN-XYZ")
	}
	if got.Name != "Bob" {
		t.Errorf("Name: got %q, want %q", got.Name, "Bob")
	}
	if got.Role != domain.Role("admin") {
		t.Errorf("Role: got %q, want %q", got.Role, domain.Role("admin"))
	}

	// Non-existent token should return not found.
	_, err = s.GetInviteByToken(ctx, "NO-SUCH-TOKEN")
	if err == nil {
		t.Fatal("expected error for non-existent token, got nil")
	}
	var storeErr *store.Error
	if !errors.As(err, &storeErr) {
		t.Fatalf("expected *store.Error, got %T: %v", err, err)
	}
	if storeErr.Code != store.ErrNotFound.Code {
		t.Errorf("expected status %d, got %d", store.ErrNotFound.Code, storeErr.Code)
	}
}

func TestCreateInvite_DuplicateCode(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "admin-dup")

	now := time.Now()
	inv1 := &domain.Invite{
		Syncable: domain.Syncable{
			ID:        "inv-dup-1",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Code:      "SAME-CODE",
		Name:      "First",
		Email:     "first@example.com",
		Role:      domain.Role("member"),
		CreatedBy: "admin-dup",
		ExpiresAt: now.Add(24 * time.Hour),
	}

	if err := s.CreateInvite(ctx, inv1); err != nil {
		t.Fatalf("CreateInvite inv1: %v", err)
	}

	// Second invite with the same code but different ID.
	inv2 := &domain.Invite{
		Syncable: domain.Syncable{
			ID:        "inv-dup-2",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Code:      "SAME-CODE",
		Name:      "Second",
		Email:     "second@example.com",
		Role:      domain.Role("member"),
		CreatedBy: "admin-dup",
		ExpiresAt: now.Add(24 * time.Hour),
	}

	err := s.CreateInvite(ctx, inv2)
	if err == nil {
		t.Fatal("expected error for duplicate code, got nil")
	}
	if !errors.Is(err, store.ErrAlreadyExists) {
		t.Errorf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestListInvites(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "admin-list")

	now := time.Now()
	inv1 := &domain.Invite{
		Syncable: domain.Syncable{
			ID:        "inv-list-1",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Code:      "LIST-CODE-1",
		Name:      "Invite One",
		Email:     "one@example.com",
		Role:      domain.Role("member"),
		CreatedBy: "admin-list",
		ExpiresAt: now.Add(24 * time.Hour),
	}
	inv2 := &domain.Invite{
		Syncable: domain.Syncable{
			ID:        "inv-list-2",
			CreatedAt: now.Add(time.Second),
			UpdatedAt: now.Add(time.Second),
		},
		Code:      "LIST-CODE-2",
		Name:      "Invite Two",
		Email:     "two@example.com",
		Role:      domain.Role("admin"),
		CreatedBy: "admin-list",
		ExpiresAt: now.Add(48 * time.Hour),
	}

	for _, inv := range []*domain.Invite{inv1, inv2} {
		if err := s.CreateInvite(ctx, inv); err != nil {
			t.Fatalf("CreateInvite(%s): %v", inv.ID, err)
		}
	}

	invites, err := s.ListInvites(ctx)
	if err != nil {
		t.Fatalf("ListInvites: %v", err)
	}

	if len(invites) != 2 {
		t.Fatalf("ListInvites: got %d invites, want 2", len(invites))
	}

	// ListInvites orders by created_at DESC, so inv2 (later) comes first.
	if invites[0].ID != "inv-list-2" {
		t.Errorf("first invite ID: got %q, want %q", invites[0].ID, "inv-list-2")
	}
	if invites[1].ID != "inv-list-1" {
		t.Errorf("second invite ID: got %q, want %q", invites[1].ID, "inv-list-1")
	}
}

func TestUseInvite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "admin-use")
	insertTestUser(t, s, "claimer-1")

	now := time.Now()
	invite := &domain.Invite{
		Syncable: domain.Syncable{
			ID:        "inv-use-1",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Code:      "USE-CODE",
		Name:      "To Be Claimed",
		Email:     "claim@example.com",
		Role:      domain.Role("member"),
		CreatedBy: "admin-use",
		ExpiresAt: now.Add(24 * time.Hour),
	}

	if err := s.CreateInvite(ctx, invite); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	// Mark the invite as claimed.
	claimedAt := time.Now()
	invite.ClaimedAt = &claimedAt
	invite.ClaimedBy = "claimer-1"
	invite.UpdatedAt = time.Now()

	if err := s.UseInvite(ctx, invite); err != nil {
		t.Fatalf("UseInvite: %v", err)
	}

	// Verify the claim persisted.
	got, err := s.GetInvite(ctx, "inv-use-1")
	if err != nil {
		t.Fatalf("GetInvite after UseInvite: %v", err)
	}

	if got.ClaimedAt == nil {
		t.Fatal("ClaimedAt: expected non-nil after UseInvite")
	}
	if got.ClaimedAt.Unix() != claimedAt.Unix() {
		t.Errorf("ClaimedAt: got %v, want %v", got.ClaimedAt, claimedAt)
	}
	if got.ClaimedBy != "claimer-1" {
		t.Errorf("ClaimedBy: got %q, want %q", got.ClaimedBy, "claimer-1")
	}
	if got.UpdatedAt.Unix() != invite.UpdatedAt.Unix() {
		t.Errorf("UpdatedAt: got %v, want %v", got.UpdatedAt, invite.UpdatedAt)
	}
}
