package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	domainerrors "github.com/listenupapp/listenup-server/internal/errors"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
)

// AdminService handles admin-only user management operations.
type AdminService struct {
	store                   *store.Store
	logger                  *slog.Logger
	registrationBroadcaster *sse.RegistrationBroadcaster
	lensService             *LensService
}

// NewAdminService creates a new admin service.
func NewAdminService(store *store.Store, logger *slog.Logger, registrationBroadcaster *sse.RegistrationBroadcaster, lensService *LensService) *AdminService {
	return &AdminService{
		store:                   store,
		logger:                  logger,
		registrationBroadcaster: registrationBroadcaster,
		lensService:             lensService,
	}
}

// UpdateUserRequest contains the fields that can be updated on a user.
type UpdateUserRequest struct {
	DisplayName *string             `json:"display_name,omitempty"`
	FirstName   *string             `json:"first_name,omitempty"`
	LastName    *string             `json:"last_name,omitempty"`
	Role        *domain.Role        `json:"role,omitempty"`
	Permissions *PermissionsUpdate  `json:"permissions,omitempty"`
}

// PermissionsUpdate contains optional permission updates.
type PermissionsUpdate struct {
	CanDownload *bool `json:"can_download,omitempty"`
	CanShare    *bool `json:"can_share,omitempty"`
}

// ListUsers returns all non-deleted users.
func (s *AdminService) ListUsers(ctx context.Context) ([]*domain.User, error) {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return users, nil
}

// GetUser returns a user by ID.
func (s *AdminService) GetUser(ctx context.Context, userID string) (*domain.User, error) {
	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			return nil, domainerrors.NotFound("user not found")
		}
		return nil, fmt.Errorf("get user: %w", err)
	}
	return user, nil
}

// UpdateUser updates a user's details.
// Returns an error if trying to demote the only admin or modify root status.
func (s *AdminService) UpdateUser(ctx context.Context, adminUserID, targetUserID string, req UpdateUserRequest) (*domain.User, error) {
	// Get target user
	user, err := s.store.GetUser(ctx, targetUserID)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			return nil, domainerrors.NotFound("user not found")
		}
		return nil, fmt.Errorf("get user: %w", err)
	}

	// Check if trying to change role
	if req.Role != nil && *req.Role != user.Role {
		// Cannot change role of root user
		if user.IsRoot {
			return nil, domainerrors.Forbidden("cannot change role of the root user")
		}

		// If demoting an admin, ensure there's at least one other admin
		if user.Role == domain.RoleAdmin && *req.Role == domain.RoleMember {
			if err := s.ensureOtherAdminExists(ctx, targetUserID); err != nil {
				return nil, err
			}
		}

		user.Role = *req.Role
	}

	// Update optional fields
	if req.DisplayName != nil {
		user.DisplayName = *req.DisplayName
	}
	if req.FirstName != nil {
		user.FirstName = *req.FirstName
	}
	if req.LastName != nil {
		user.LastName = *req.LastName
	}

	// Update permissions if provided
	if req.Permissions != nil {
		if req.Permissions.CanDownload != nil {
			user.Permissions.CanDownload = *req.Permissions.CanDownload
		}
		if req.Permissions.CanShare != nil {
			user.Permissions.CanShare = *req.Permissions.CanShare
		}
	}

	if err := s.store.UpdateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("User updated by admin",
			"admin_id", adminUserID,
			"user_id", targetUserID,
		)
	}

	return user, nil
}

// DeleteUser soft-deletes a user.
// Returns an error if trying to delete self, root user, or the last admin.
func (s *AdminService) DeleteUser(ctx context.Context, adminUserID, targetUserID string) error {
	// Cannot delete yourself
	if adminUserID == targetUserID {
		return domainerrors.Forbidden("cannot delete your own account")
	}

	// Get target user
	user, err := s.store.GetUser(ctx, targetUserID)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			return domainerrors.NotFound("user not found")
		}
		return fmt.Errorf("get user: %w", err)
	}

	// Cannot delete root user
	if user.IsRoot {
		return domainerrors.Forbidden("cannot delete the root user")
	}

	// If deleting an admin, ensure there's at least one other admin
	if user.IsAdmin() {
		if err := s.ensureOtherAdminExists(ctx, targetUserID); err != nil {
			return err
		}
	}

	// Broadcast user.deleted SSE event BEFORE the soft delete
	// so the user can receive it while still authenticated.
	// This allows their client to clear auth state and show appropriate message.
	s.store.BroadcastUserDeleted(targetUserID, "Account deleted by administrator")

	// Soft delete the user
	user.MarkDeleted()
	if err := s.store.UpdateUser(ctx, user); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	// Clean up user's lenses
	if err := s.store.DeleteLensesForUser(ctx, targetUserID); err != nil {
		// Log but don't fail - user is already deleted
		if s.logger != nil {
			s.logger.Warn("Failed to delete lenses for deleted user",
				"user_id", targetUserID,
				"error", err,
			)
		}
	}

	// TODO: Invalidate all user sessions

	if s.logger != nil {
		s.logger.Info("User deleted by admin",
			"admin_id", adminUserID,
			"user_id", targetUserID,
			"email", user.Email,
		)
	}

	return nil
}

// ensureOtherAdminExists checks that there's at least one other admin besides the target user.
func (s *AdminService) ensureOtherAdminExists(ctx context.Context, excludeUserID string) error {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}

	for _, u := range users {
		if u.ID != excludeUserID && u.IsAdmin() {
			return nil // Found another admin
		}
	}

	return domainerrors.Forbidden("cannot remove the last admin")
}

// ListPendingUsers returns all users awaiting approval.
func (s *AdminService) ListPendingUsers(ctx context.Context) ([]*domain.User, error) {
	users, err := s.store.ListPendingUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pending users: %w", err)
	}
	return users, nil
}

// ApproveUser approves a pending user, allowing them to log in.
func (s *AdminService) ApproveUser(ctx context.Context, adminUserID, targetUserID string) (*domain.User, error) {
	// Get target user
	user, err := s.store.GetUser(ctx, targetUserID)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			return nil, domainerrors.NotFound("user not found")
		}
		return nil, fmt.Errorf("get user: %w", err)
	}

	// Must be pending
	if !user.IsPending() {
		return nil, domainerrors.Validation("user is not pending approval")
	}

	// Approve the user
	user.Status = domain.UserStatusActive
	user.ApprovedBy = adminUserID
	user.ApprovedAt = time.Now()
	user.Touch()

	if err := s.store.UpdateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}

	// Broadcast SSE event for admin users
	s.store.BroadcastUserApproved(user)

	// Notify the pending user directly via their registration SSE stream
	if s.registrationBroadcaster != nil {
		s.registrationBroadcaster.NotifyApproved(targetUserID)
	}

	// Create default "To Read" lens for the newly approved user (best effort)
	if s.lensService != nil {
		if err := s.lensService.CreateDefaultLens(ctx, targetUserID); err != nil {
			if s.logger != nil {
				s.logger.Warn("Failed to create default lens for approved user",
					"user_id", targetUserID,
					"error", err,
				)
			}
			// Non-fatal: user can still create lenses manually
		}
	}

	if s.logger != nil {
		s.logger.Info("User approved by admin",
			"admin_id", adminUserID,
			"user_id", targetUserID,
			"email", user.Email,
		)
	}

	return user, nil
}

// DenyUser denies a pending user registration request.
// This soft-deletes the user account.
func (s *AdminService) DenyUser(ctx context.Context, adminUserID, targetUserID string) error {
	// Get target user
	user, err := s.store.GetUser(ctx, targetUserID)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			return domainerrors.NotFound("user not found")
		}
		return fmt.Errorf("get user: %w", err)
	}

	// Must be pending
	if !user.IsPending() {
		return domainerrors.Validation("user is not pending approval")
	}

	// Soft delete the user
	user.MarkDeleted()
	if err := s.store.UpdateUser(ctx, user); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	// Notify the pending user directly via their registration SSE stream
	if s.registrationBroadcaster != nil {
		s.registrationBroadcaster.NotifyDenied(targetUserID)
	}

	if s.logger != nil {
		s.logger.Info("User registration denied by admin",
			"admin_id", adminUserID,
			"user_id", targetUserID,
			"email", user.Email,
		)
	}

	return nil
}
