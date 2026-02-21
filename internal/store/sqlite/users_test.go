package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// makeTestUser creates a domain.User with sensible defaults for testing.
func makeTestUser(id, email string) *domain.User {
	now := time.Now()
	return &domain.User{
		Syncable: domain.Syncable{
			ID:        id,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Email:        email,
		PasswordHash: "$2a$10$fakehashfortest",
		IsRoot:       false,
		Role:         domain.RoleMember,
		Status:       domain.UserStatusActive,
		DisplayName:  "Test User",
		FirstName:    "Test",
		LastName:     "User",
		LastLoginAt:  now,
		Permissions:  domain.DefaultPermissions(),
	}
}

func TestCreateAndGetUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	user := makeTestUser("user-1", "Alice@Example.com")
	user.IsRoot = true
	user.Role = domain.RoleAdmin

	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	got, err := s.GetUser(ctx, "user-1")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}

	// Verify fields.
	if got.ID != user.ID {
		t.Errorf("ID: got %q, want %q", got.ID, user.ID)
	}
	if got.Email != user.Email {
		t.Errorf("Email: got %q, want %q", got.Email, user.Email)
	}
	if got.PasswordHash != user.PasswordHash {
		t.Errorf("PasswordHash: got %q, want %q", got.PasswordHash, user.PasswordHash)
	}
	if got.IsRoot != true {
		t.Error("IsRoot: expected true")
	}
	if got.Role != domain.RoleAdmin {
		t.Errorf("Role: got %q, want %q", got.Role, domain.RoleAdmin)
	}
	if got.Status != domain.UserStatusActive {
		t.Errorf("Status: got %q, want %q", got.Status, domain.UserStatusActive)
	}
	if got.DisplayName != "Test User" {
		t.Errorf("DisplayName: got %q, want %q", got.DisplayName, "Test User")
	}
	if got.FirstName != "Test" {
		t.Errorf("FirstName: got %q, want %q", got.FirstName, "Test")
	}
	if got.LastName != "User" {
		t.Errorf("LastName: got %q, want %q", got.LastName, "User")
	}
	if !got.Permissions.CanDownload {
		t.Error("CanDownload: expected true")
	}
	if !got.Permissions.CanShare {
		t.Error("CanShare: expected true")
	}
	if got.DeletedAt != nil {
		t.Error("DeletedAt: expected nil")
	}

	// Timestamps should round-trip through RFC3339Nano.
	if got.CreatedAt.Unix() != user.CreatedAt.Unix() {
		t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, user.CreatedAt)
	}
	if got.UpdatedAt.Unix() != user.UpdatedAt.Unix() {
		t.Errorf("UpdatedAt: got %v, want %v", got.UpdatedAt, user.UpdatedAt)
	}
	if got.LastLoginAt.Unix() != user.LastLoginAt.Unix() {
		t.Errorf("LastLoginAt: got %v, want %v", got.LastLoginAt, user.LastLoginAt)
	}
}

func TestGetUser_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetUser(ctx, "nonexistent")
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

func TestCreateUser_DuplicateEmail(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	u1 := makeTestUser("user-1", "duplicate@example.com")
	if err := s.CreateUser(ctx, u1); err != nil {
		t.Fatalf("CreateUser u1: %v", err)
	}

	// Same email, different ID.
	u2 := makeTestUser("user-2", "duplicate@example.com")
	err := s.CreateUser(ctx, u2)
	if err == nil {
		t.Fatal("expected error for duplicate email, got nil")
	}
	if !errors.Is(err, store.ErrAlreadyExists) {
		t.Errorf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestCreateUser_DuplicateID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	u1 := makeTestUser("user-dup-id", "first@example.com")
	if err := s.CreateUser(ctx, u1); err != nil {
		t.Fatalf("CreateUser u1: %v", err)
	}

	// Same ID, different email.
	u2 := makeTestUser("user-dup-id", "second@example.com")
	err := s.CreateUser(ctx, u2)
	if err == nil {
		t.Fatal("expected error for duplicate ID, got nil")
	}
	if !errors.Is(err, store.ErrAlreadyExists) {
		t.Errorf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestGetUserByEmail(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	user := makeTestUser("user-email", "Bob@Example.com")
	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Exact match should work.
	got, err := s.GetUserByEmail(ctx, "Bob@Example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if got.ID != "user-email" {
		t.Errorf("ID: got %q, want %q", got.ID, "user-email")
	}

	// Different case should NOT match (exact match).
	_, err = s.GetUserByEmail(ctx, "bob@example.com")
	if err == nil {
		t.Fatal("expected not found for different case, got nil")
	}
}

func TestGetUserByEmailLower(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	user := makeTestUser("user-lower", "Carol@Example.COM")
	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Case-insensitive lookup should find the user.
	tests := []string{
		"Carol@Example.COM",
		"carol@example.com",
		"CAROL@EXAMPLE.COM",
		"  carol@example.com  ", // with whitespace
	}
	for _, email := range tests {
		got, err := s.GetUserByEmailLower(ctx, email)
		if err != nil {
			t.Errorf("GetUserByEmailLower(%q): %v", email, err)
			continue
		}
		if got.ID != "user-lower" {
			t.Errorf("GetUserByEmailLower(%q): ID = %q, want %q", email, got.ID, "user-lower")
		}
	}

	// Completely different email should not match.
	_, err := s.GetUserByEmailLower(ctx, "nobody@example.com")
	if err == nil {
		t.Fatal("expected not found, got nil")
	}
}

func TestListUsers(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create several users.
	u1 := makeTestUser("user-list-1", "list1@example.com")
	u2 := makeTestUser("user-list-2", "list2@example.com")
	u3 := makeTestUser("user-list-3", "list3@example.com")

	for _, u := range []*domain.User{u1, u2, u3} {
		if err := s.CreateUser(ctx, u); err != nil {
			t.Fatalf("CreateUser(%s): %v", u.ID, err)
		}
	}

	// Soft-delete the third user.
	if err := s.DeleteUser(ctx, "user-list-3"); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}

	users, err := s.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}

	if len(users) != 2 {
		t.Fatalf("ListUsers: got %d users, want 2", len(users))
	}

	// Verify order (by created_at ASC) and that deleted user is excluded.
	ids := make([]string, len(users))
	for i, u := range users {
		ids[i] = u.ID
	}
	if ids[0] != "user-list-1" || ids[1] != "user-list-2" {
		t.Errorf("ListUsers: got IDs %v, want [user-list-1 user-list-2]", ids)
	}
}

func TestUpdateUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	user := makeTestUser("user-update", "update@example.com")
	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Modify fields.
	user.DisplayName = "Updated Name"
	user.FirstName = "Updated"
	user.LastName = "Person"
	user.Role = domain.RoleAdmin
	user.IsRoot = true
	user.Permissions.CanDownload = false
	user.Permissions.CanShare = false
	user.Status = domain.UserStatusPending
	// Create the approving user first to satisfy FK constraint.
	adminUser := makeTestUser("admin-user", "admin@example.com")
	if err := s.CreateUser(ctx, adminUser); err != nil {
		t.Fatalf("CreateUser admin: %v", err)
	}
	user.ApprovedBy = "admin-user"
	user.ApprovedAt = time.Now()
	user.Touch()

	if err := s.UpdateUser(ctx, user); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	got, err := s.GetUser(ctx, "user-update")
	if err != nil {
		t.Fatalf("GetUser after update: %v", err)
	}

	if got.DisplayName != "Updated Name" {
		t.Errorf("DisplayName: got %q, want %q", got.DisplayName, "Updated Name")
	}
	if got.FirstName != "Updated" {
		t.Errorf("FirstName: got %q, want %q", got.FirstName, "Updated")
	}
	if got.LastName != "Person" {
		t.Errorf("LastName: got %q, want %q", got.LastName, "Person")
	}
	if got.Role != domain.RoleAdmin {
		t.Errorf("Role: got %q, want %q", got.Role, domain.RoleAdmin)
	}
	if !got.IsRoot {
		t.Error("IsRoot: expected true")
	}
	if got.Permissions.CanDownload {
		t.Error("CanDownload: expected false after update")
	}
	if got.Permissions.CanShare {
		t.Error("CanShare: expected false after update")
	}
	if got.Status != domain.UserStatusPending {
		t.Errorf("Status: got %q, want %q", got.Status, domain.UserStatusPending)
	}
	if got.ApprovedBy != "admin-user" {
		t.Errorf("ApprovedBy: got %q, want %q", got.ApprovedBy, "admin-user")
	}
	if got.ApprovedAt.IsZero() {
		t.Error("ApprovedAt: expected non-zero after update")
	}
}

func TestUpdateUser_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	user := makeTestUser("nonexistent-user", "nope@example.com")

	err := s.UpdateUser(ctx, user)
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

func TestDeleteUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	user := makeTestUser("user-delete", "delete@example.com")
	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Verify user exists before delete.
	_, err := s.GetUser(ctx, "user-delete")
	if err != nil {
		t.Fatalf("GetUser before delete: %v", err)
	}

	// Soft delete.
	if err := s.DeleteUser(ctx, "user-delete"); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}

	// GetUser should return not found.
	_, err = s.GetUser(ctx, "user-delete")
	if err == nil {
		t.Fatal("expected not found after delete, got nil")
	}
	var storeErr *store.Error
	if !errors.As(err, &storeErr) {
		t.Fatalf("expected *store.Error, got %T: %v", err, err)
	}
	if storeErr.Code != store.ErrNotFound.Code {
		t.Errorf("expected status %d, got %d", store.ErrNotFound.Code, storeErr.Code)
	}

	// Deleting again should return not found (already deleted).
	err = s.DeleteUser(ctx, "user-delete")
	if err == nil {
		t.Fatal("expected not found on double delete, got nil")
	}
	if !errors.As(err, &storeErr) {
		t.Fatalf("expected *store.Error on double delete, got %T: %v", err, err)
	}
}

func TestDeleteUser_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	err := s.DeleteUser(ctx, "never-existed")
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

func TestCreateUser_ApprovedAtZeroValue(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// User with zero ApprovedAt (not yet approved).
	user := makeTestUser("user-no-approval", "noapproval@example.com")
	// ApprovedAt is zero by default from makeTestUser.

	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	got, err := s.GetUser(ctx, "user-no-approval")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}

	if !got.ApprovedAt.IsZero() {
		t.Errorf("ApprovedAt: expected zero value, got %v", got.ApprovedAt)
	}
}

func TestCreateUser_PermissionsDisabled(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	user := makeTestUser("user-no-perms", "noperms@example.com")
	user.Permissions.CanDownload = false
	user.Permissions.CanShare = false

	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	got, err := s.GetUser(ctx, "user-no-perms")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}

	if got.Permissions.CanDownload {
		t.Error("CanDownload: expected false")
	}
	if got.Permissions.CanShare {
		t.Error("CanShare: expected false")
	}
}
