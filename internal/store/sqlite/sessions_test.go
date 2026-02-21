package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// makeTestSession creates a domain.Session with all fields populated for testing.
// It also creates the owning user to satisfy the FK constraint.
func makeTestSession(t *testing.T, s *Store, sessionID, userID string) *domain.Session {
	t.Helper()
	ctx := context.Background()

	// Create the user if it doesn't already exist.
	user := makeTestUser(userID, userID+"@example.com")
	if err := s.CreateUser(ctx, user); err != nil {
		// Ignore duplicate — user may already exist from a previous call.
		if !errors.Is(err, store.ErrAlreadyExists) {
			t.Fatalf("makeTestSession: CreateUser(%s): %v", userID, err)
		}
	}

	now := time.Now()
	return &domain.Session{
		ID:               sessionID,
		UserID:           userID,
		RefreshTokenHash: "$2a$10$fakerefreshtokenhash",
		ExpiresAt:        now.Add(24 * time.Hour),
		CreatedAt:        now,
		LastSeenAt:       now,
		IPAddress:        "192.168.1.42",
		DeviceType:       "mobile",
		Platform:         "iOS",
		PlatformVersion:  "17.2",
		ClientName:       "ListenUp Mobile",
		ClientVersion:    "1.0.0",
		ClientBuild:      "245",
		DeviceName:       "Simon's iPhone",
		DeviceModel:      "iPhone 15 Pro",
		BrowserName:      "",
		BrowserVersion:   "",
	}
}

func TestCreateAndGetSession(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	session := makeTestSession(t, s, "sess-1", "user-sess-1")

	if err := s.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	got, err := s.GetSession(ctx, "sess-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	// Verify all fields.
	if got.ID != session.ID {
		t.Errorf("ID: got %q, want %q", got.ID, session.ID)
	}
	if got.UserID != session.UserID {
		t.Errorf("UserID: got %q, want %q", got.UserID, session.UserID)
	}
	if got.RefreshTokenHash != session.RefreshTokenHash {
		t.Errorf("RefreshTokenHash: got %q, want %q", got.RefreshTokenHash, session.RefreshTokenHash)
	}
	if got.IPAddress != session.IPAddress {
		t.Errorf("IPAddress: got %q, want %q", got.IPAddress, session.IPAddress)
	}
	if got.DeviceType != session.DeviceType {
		t.Errorf("DeviceType: got %q, want %q", got.DeviceType, session.DeviceType)
	}
	if got.Platform != session.Platform {
		t.Errorf("Platform: got %q, want %q", got.Platform, session.Platform)
	}
	if got.PlatformVersion != session.PlatformVersion {
		t.Errorf("PlatformVersion: got %q, want %q", got.PlatformVersion, session.PlatformVersion)
	}
	if got.ClientName != session.ClientName {
		t.Errorf("ClientName: got %q, want %q", got.ClientName, session.ClientName)
	}
	if got.ClientVersion != session.ClientVersion {
		t.Errorf("ClientVersion: got %q, want %q", got.ClientVersion, session.ClientVersion)
	}
	if got.ClientBuild != session.ClientBuild {
		t.Errorf("ClientBuild: got %q, want %q", got.ClientBuild, session.ClientBuild)
	}
	if got.DeviceName != session.DeviceName {
		t.Errorf("DeviceName: got %q, want %q", got.DeviceName, session.DeviceName)
	}
	if got.DeviceModel != session.DeviceModel {
		t.Errorf("DeviceModel: got %q, want %q", got.DeviceModel, session.DeviceModel)
	}
	if got.BrowserName != session.BrowserName {
		t.Errorf("BrowserName: got %q, want %q", got.BrowserName, session.BrowserName)
	}
	if got.BrowserVersion != session.BrowserVersion {
		t.Errorf("BrowserVersion: got %q, want %q", got.BrowserVersion, session.BrowserVersion)
	}

	// Timestamps should round-trip through RFC3339Nano.
	if got.ExpiresAt.Unix() != session.ExpiresAt.Unix() {
		t.Errorf("ExpiresAt: got %v, want %v", got.ExpiresAt, session.ExpiresAt)
	}
	if got.CreatedAt.Unix() != session.CreatedAt.Unix() {
		t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, session.CreatedAt)
	}
	if got.LastSeenAt.Unix() != session.LastSeenAt.Unix() {
		t.Errorf("LastSeenAt: got %v, want %v", got.LastSeenAt, session.LastSeenAt)
	}
}

func TestGetSession_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetSession(ctx, "nonexistent")
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

func TestCreateSession_Duplicate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	session := makeTestSession(t, s, "sess-dup", "user-sess-dup")

	if err := s.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Attempt to insert the same session ID again.
	err := s.CreateSession(ctx, session)
	if err == nil {
		t.Fatal("expected error for duplicate session, got nil")
	}
	if !errors.Is(err, store.ErrAlreadyExists) {
		t.Errorf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestListSessions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	s1 := makeTestSession(t, s, "sess-list-1", "user-list-sess")
	s2 := makeTestSession(t, s, "sess-list-2", "user-list-sess")
	s3 := makeTestSession(t, s, "sess-list-3", "user-list-sess")

	for _, sess := range []*domain.Session{s1, s2, s3} {
		if err := s.CreateSession(ctx, sess); err != nil {
			t.Fatalf("CreateSession(%s): %v", sess.ID, err)
		}
	}

	sessions, err := s.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}

	if len(sessions) != 3 {
		t.Fatalf("ListSessions: got %d sessions, want 3", len(sessions))
	}

	ids := make([]string, len(sessions))
	for i, sess := range sessions {
		ids[i] = sess.ID
	}
	if ids[0] != "sess-list-1" || ids[1] != "sess-list-2" || ids[2] != "sess-list-3" {
		t.Errorf("ListSessions: got IDs %v, want [sess-list-1 sess-list-2 sess-list-3]", ids)
	}
}

func TestUpdateSession(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	session := makeTestSession(t, s, "sess-update", "user-sess-update")
	if err := s.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Modify fields.
	session.RefreshTokenHash = "$2a$10$updatedrefreshtoken"
	session.IPAddress = "10.0.0.1"
	session.DeviceType = "desktop"
	session.Platform = "macOS"
	session.PlatformVersion = "14.2"
	session.ClientName = "ListenUp Desktop"
	session.ClientVersion = "2.0.0"
	session.ClientBuild = "500"
	session.DeviceName = "Work MacBook"
	session.DeviceModel = "MacBook Pro 16"
	session.BrowserName = "Safari"
	session.BrowserVersion = "17.2"
	session.LastSeenAt = time.Now().Add(time.Hour)

	if err := s.UpdateSession(ctx, session); err != nil {
		t.Fatalf("UpdateSession: %v", err)
	}

	got, err := s.GetSession(ctx, "sess-update")
	if err != nil {
		t.Fatalf("GetSession after update: %v", err)
	}

	if got.RefreshTokenHash != "$2a$10$updatedrefreshtoken" {
		t.Errorf("RefreshTokenHash: got %q, want %q", got.RefreshTokenHash, "$2a$10$updatedrefreshtoken")
	}
	if got.IPAddress != "10.0.0.1" {
		t.Errorf("IPAddress: got %q, want %q", got.IPAddress, "10.0.0.1")
	}
	if got.DeviceType != "desktop" {
		t.Errorf("DeviceType: got %q, want %q", got.DeviceType, "desktop")
	}
	if got.Platform != "macOS" {
		t.Errorf("Platform: got %q, want %q", got.Platform, "macOS")
	}
	if got.PlatformVersion != "14.2" {
		t.Errorf("PlatformVersion: got %q, want %q", got.PlatformVersion, "14.2")
	}
	if got.ClientName != "ListenUp Desktop" {
		t.Errorf("ClientName: got %q, want %q", got.ClientName, "ListenUp Desktop")
	}
	if got.ClientVersion != "2.0.0" {
		t.Errorf("ClientVersion: got %q, want %q", got.ClientVersion, "2.0.0")
	}
	if got.ClientBuild != "500" {
		t.Errorf("ClientBuild: got %q, want %q", got.ClientBuild, "500")
	}
	if got.DeviceName != "Work MacBook" {
		t.Errorf("DeviceName: got %q, want %q", got.DeviceName, "Work MacBook")
	}
	if got.DeviceModel != "MacBook Pro 16" {
		t.Errorf("DeviceModel: got %q, want %q", got.DeviceModel, "MacBook Pro 16")
	}
	if got.BrowserName != "Safari" {
		t.Errorf("BrowserName: got %q, want %q", got.BrowserName, "Safari")
	}
	if got.BrowserVersion != "17.2" {
		t.Errorf("BrowserVersion: got %q, want %q", got.BrowserVersion, "17.2")
	}
	if got.LastSeenAt.Unix() != session.LastSeenAt.Unix() {
		t.Errorf("LastSeenAt: got %v, want %v", got.LastSeenAt, session.LastSeenAt)
	}
}

func TestUpdateSession_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create the user so FK is satisfied, but don't create the session.
	user := makeTestUser("user-sess-upd-nf", "upd-nf@example.com")
	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	session := &domain.Session{
		ID:         "nonexistent-sess",
		UserID:     "user-sess-upd-nf",
		ExpiresAt:  time.Now().Add(time.Hour),
		CreatedAt:  time.Now(),
		LastSeenAt: time.Now(),
	}

	err := s.UpdateSession(ctx, session)
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

func TestDeleteSession(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	session := makeTestSession(t, s, "sess-delete", "user-sess-delete")
	if err := s.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Verify the session exists.
	_, err := s.GetSession(ctx, "sess-delete")
	if err != nil {
		t.Fatalf("GetSession before delete: %v", err)
	}

	// Hard delete.
	if err := s.DeleteSession(ctx, "sess-delete"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	// GetSession should return not found.
	_, err = s.GetSession(ctx, "sess-delete")
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
}

func TestDeleteSession_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	err := s.DeleteSession(ctx, "never-existed")
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

func TestGetSessionsByUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create sessions for user A.
	sA1 := makeTestSession(t, s, "sess-ua-1", "user-a")
	sA2 := makeTestSession(t, s, "sess-ua-2", "user-a")
	sA3 := makeTestSession(t, s, "sess-ua-3", "user-a")

	// Create a session for user B.
	sB1 := makeTestSession(t, s, "sess-ub-1", "user-b")

	for _, sess := range []*domain.Session{sA1, sA2, sA3, sB1} {
		if err := s.CreateSession(ctx, sess); err != nil {
			t.Fatalf("CreateSession(%s): %v", sess.ID, err)
		}
	}

	// Fetch sessions for user A only.
	sessions, err := s.GetSessionsByUser(ctx, "user-a")
	if err != nil {
		t.Fatalf("GetSessionsByUser: %v", err)
	}

	if len(sessions) != 3 {
		t.Fatalf("GetSessionsByUser: got %d sessions, want 3", len(sessions))
	}

	for _, sess := range sessions {
		if sess.UserID != "user-a" {
			t.Errorf("unexpected UserID %q, want %q", sess.UserID, "user-a")
		}
	}

	// Fetch sessions for user B.
	sessionsB, err := s.GetSessionsByUser(ctx, "user-b")
	if err != nil {
		t.Fatalf("GetSessionsByUser(user-b): %v", err)
	}
	if len(sessionsB) != 1 {
		t.Fatalf("GetSessionsByUser(user-b): got %d sessions, want 1", len(sessionsB))
	}
	if sessionsB[0].ID != "sess-ub-1" {
		t.Errorf("GetSessionsByUser(user-b): got ID %q, want %q", sessionsB[0].ID, "sess-ub-1")
	}

	// Non-existent user should return empty slice, not error.
	sessionsNone, err := s.GetSessionsByUser(ctx, "user-nonexistent")
	if err != nil {
		t.Fatalf("GetSessionsByUser(nonexistent): %v", err)
	}
	if len(sessionsNone) != 0 {
		t.Errorf("GetSessionsByUser(nonexistent): got %d sessions, want 0", len(sessionsNone))
	}
}

func TestDeleteExpiredSessions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	now := time.Now()

	// Create two expired sessions.
	expired1 := makeTestSession(t, s, "sess-exp-1", "user-exp")
	expired1.ExpiresAt = now.Add(-2 * time.Hour)

	expired2 := makeTestSession(t, s, "sess-exp-2", "user-exp")
	expired2.ExpiresAt = now.Add(-1 * time.Hour)

	// Create two valid sessions.
	valid1 := makeTestSession(t, s, "sess-valid-1", "user-exp")
	valid1.ExpiresAt = now.Add(24 * time.Hour)

	valid2 := makeTestSession(t, s, "sess-valid-2", "user-exp")
	valid2.ExpiresAt = now.Add(48 * time.Hour)

	for _, sess := range []*domain.Session{expired1, expired2, valid1, valid2} {
		if err := s.CreateSession(ctx, sess); err != nil {
			t.Fatalf("CreateSession(%s): %v", sess.ID, err)
		}
	}

	// Verify all four sessions exist.
	all, err := s.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("ListSessions before cleanup: got %d, want 4", len(all))
	}

	// Delete expired sessions.
	deleted, err := s.DeleteExpiredSessions(ctx)
	if err != nil {
		t.Fatalf("DeleteExpiredSessions: %v", err)
	}
	if deleted != 2 {
		t.Errorf("DeleteExpiredSessions: deleted %d, want 2", deleted)
	}

	// Only valid sessions should remain.
	remaining, err := s.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions after cleanup: %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("ListSessions after cleanup: got %d, want 2", len(remaining))
	}

	ids := make(map[string]bool)
	for _, sess := range remaining {
		ids[sess.ID] = true
	}
	if !ids["sess-valid-1"] {
		t.Error("expected sess-valid-1 to survive cleanup")
	}
	if !ids["sess-valid-2"] {
		t.Error("expected sess-valid-2 to survive cleanup")
	}
	if ids["sess-exp-1"] {
		t.Error("expected sess-exp-1 to be deleted")
	}
	if ids["sess-exp-2"] {
		t.Error("expected sess-exp-2 to be deleted")
	}

	// Calling again with no expired sessions should return 0.
	deleted2, err := s.DeleteExpiredSessions(ctx)
	if err != nil {
		t.Fatalf("DeleteExpiredSessions (second call): %v", err)
	}
	if deleted2 != 0 {
		t.Errorf("DeleteExpiredSessions (second call): deleted %d, want 0", deleted2)
	}
}
