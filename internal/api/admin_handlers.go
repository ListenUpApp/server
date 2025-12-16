package api

import (
	"encoding/json/v2"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	domainerrors "github.com/listenupapp/listenup-server/internal/errors"
	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/service"
)

// Admin Invite Handlers

// handleCreateInvite creates a new invite.
// POST /api/v1/admin/invites
func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	var req service.CreateInviteRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	invite, err := s.services.Invite.CreateInvite(ctx, userID, req)
	if err != nil {
		handleServiceError(w, err, s.logger)
		return
	}

	response.Created(w, invite, s.logger)
}

// handleListInvites returns all invites.
// GET /api/v1/admin/invites
func (s *Server) handleListInvites(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	invites, err := s.services.Invite.ListInvites(ctx)
	if err != nil {
		s.logger.Error("Failed to list invites", "error", err)
		response.InternalError(w, "Failed to list invites", s.logger)
		return
	}

	// Add URL to each invite for client convenience
	inviteResponses := make([]map[string]interface{}, 0, len(invites))
	for _, invite := range invites {
		inviteResponses = append(inviteResponses, map[string]interface{}{
			"id":         invite.ID,
			"code":       invite.Code,
			"name":       invite.Name,
			"email":      invite.Email,
			"role":       invite.Role,
			"created_by": invite.CreatedBy,
			"expires_at": invite.ExpiresAt,
			"claimed_at": invite.ClaimedAt,
			"claimed_by": invite.ClaimedBy,
			"created_at": invite.CreatedAt,
			"updated_at": invite.UpdatedAt,
			"url":        s.services.Invite.GetInviteURL(invite.Code),
		})
	}

	response.Success(w, map[string]interface{}{
		"invites": inviteResponses,
	}, s.logger)
}

// handleDeleteInvite revokes an unclaimed invite.
// DELETE /api/v1/admin/invites/{id}
func (s *Server) handleDeleteInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	inviteID := chi.URLParam(r, "id")

	if inviteID == "" {
		response.BadRequest(w, "Invite ID is required", s.logger)
		return
	}

	if err := s.services.Invite.DeleteInvite(ctx, inviteID); err != nil {
		handleServiceError(w, err, s.logger)
		return
	}

	response.NoContent(w)
}

// Admin User Handlers

// handleListUsers returns all users.
// GET /api/v1/admin/users
func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	users, err := s.services.Admin.ListUsers(ctx)
	if err != nil {
		s.logger.Error("Failed to list users", "error", err)
		response.InternalError(w, "Failed to list users", s.logger)
		return
	}

	// Filter out sensitive fields
	var sanitizedUsers []map[string]interface{}
	for _, u := range users {
		sanitizedUsers = append(sanitizedUsers, map[string]interface{}{
			"id":           u.ID,
			"email":        u.Email,
			"display_name": u.DisplayName,
			"first_name":   u.FirstName,
			"last_name":    u.LastName,
			"is_root":      u.IsRoot,
			"role":         u.Role,
			"invited_by":   u.InvitedBy,
			"created_at":   u.CreatedAt,
			"last_login_at": u.LastLoginAt,
		})
	}

	response.Success(w, map[string]interface{}{
		"users": sanitizedUsers,
	}, s.logger)
}

// handleGetAdminUser returns a single user.
// GET /api/v1/admin/users/{id}
func (s *Server) handleGetAdminUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	targetUserID := chi.URLParam(r, "id")

	if targetUserID == "" {
		response.BadRequest(w, "User ID is required", s.logger)
		return
	}

	user, err := s.services.Admin.GetUser(ctx, targetUserID)
	if err != nil {
		handleServiceError(w, err, s.logger)
		return
	}

	// Filter out sensitive fields
	sanitizedUser := map[string]interface{}{
		"id":           user.ID,
		"email":        user.Email,
		"display_name": user.DisplayName,
		"first_name":   user.FirstName,
		"last_name":    user.LastName,
		"is_root":      user.IsRoot,
		"role":         user.Role,
		"invited_by":   user.InvitedBy,
		"created_at":   user.CreatedAt,
		"last_login_at": user.LastLoginAt,
	}

	response.Success(w, sanitizedUser, s.logger)
}

// handleUpdateAdminUser updates a user's details.
// PATCH /api/v1/admin/users/{id}
func (s *Server) handleUpdateAdminUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	adminUserID := getUserID(ctx)
	targetUserID := chi.URLParam(r, "id")

	if targetUserID == "" {
		response.BadRequest(w, "User ID is required", s.logger)
		return
	}

	var req service.UpdateUserRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	user, err := s.services.Admin.UpdateUser(ctx, adminUserID, targetUserID, req)
	if err != nil {
		handleServiceError(w, err, s.logger)
		return
	}

	// Filter out sensitive fields
	sanitizedUser := map[string]interface{}{
		"id":           user.ID,
		"email":        user.Email,
		"display_name": user.DisplayName,
		"first_name":   user.FirstName,
		"last_name":    user.LastName,
		"is_root":      user.IsRoot,
		"role":         user.Role,
		"invited_by":   user.InvitedBy,
		"created_at":   user.CreatedAt,
		"last_login_at": user.LastLoginAt,
	}

	response.Success(w, sanitizedUser, s.logger)
}

// handleDeleteAdminUser soft-deletes a user.
// DELETE /api/v1/admin/users/{id}
func (s *Server) handleDeleteAdminUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	adminUserID := getUserID(ctx)
	targetUserID := chi.URLParam(r, "id")

	if targetUserID == "" {
		response.BadRequest(w, "User ID is required", s.logger)
		return
	}

	if err := s.services.Admin.DeleteUser(ctx, adminUserID, targetUserID); err != nil {
		handleServiceError(w, err, s.logger)
		return
	}

	response.NoContent(w)
}

// handleServiceError maps service errors to HTTP responses.
func handleServiceError(w http.ResponseWriter, err error, logger *slog.Logger) {
	var domainErr *domainerrors.Error
	if errors.As(err, &domainErr) {
		switch domainErr.Code {
		case domainerrors.CodeNotFound:
			response.NotFound(w, domainErr.Message, logger)
		case domainerrors.CodeAlreadyExists, domainerrors.CodeConflict:
			response.Conflict(w, domainErr.Message, logger)
		case domainerrors.CodeForbidden:
			response.Forbidden(w, domainErr.Message, logger)
		case domainerrors.CodeValidation:
			response.BadRequest(w, domainErr.Message, logger)
		case domainerrors.CodeUnauthorized:
			response.Unauthorized(w, domainErr.Message, logger)
		default:
			response.InternalError(w, domainErr.Message, logger)
		}
		return
	}

	response.InternalError(w, "An unexpected error occurred", logger)
}
