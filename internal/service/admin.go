package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/listenupapp/listenup-server/internal/domain"
	domainerrors "github.com/listenupapp/listenup-server/internal/errors"
	"github.com/listenupapp/listenup-server/internal/store"
)

// AdminService handles admin-only user management operations.
type AdminService struct {
	store  *store.Store
	logger *slog.Logger
}

// NewAdminService creates a new admin service.
func NewAdminService(store *store.Store, logger *slog.Logger) *AdminService {
	return &AdminService{
		store:  store,
		logger: logger,
	}
}

// UpdateUserRequest contains the fields that can be updated on a user.
type UpdateUserRequest struct {
	DisplayName *string      `json:"display_name,omitempty"`
	FirstName   *string      `json:"first_name,omitempty"`
	LastName    *string      `json:"last_name,omitempty"`
	Role        *domain.Role `json:"role,omitempty"`
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

	// Soft delete the user
	user.MarkDeleted()
	if err := s.store.UpdateUser(ctx, user); err != nil {
		return fmt.Errorf("delete user: %w", err)
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
