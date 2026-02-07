package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/service"
)

func (s *Server) registerAdminRoutes() {
	// Invite management
	huma.Register(s.api, huma.Operation{
		OperationID: "createInvite",
		Method:      http.MethodPost,
		Path:        "/api/v1/admin/invites",
		Summary:     "Create invite",
		Description: "Creates a new invite code",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleCreateInvite)

	huma.Register(s.api, huma.Operation{
		OperationID: "listInvites",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/invites",
		Summary:     "List invites",
		Description: "Lists all invite codes",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListInvites)

	huma.Register(s.api, huma.Operation{
		OperationID: "deleteInvite",
		Method:      http.MethodDelete,
		Path:        "/api/v1/admin/invites/{id}",
		Summary:     "Delete invite",
		Description: "Deletes an invite code",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleDeleteInvite)

	// User management
	huma.Register(s.api, huma.Operation{
		OperationID: "listUsers",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/users",
		Summary:     "List users",
		Description: "Lists all users",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListUsers)

	// Pending users - must be registered BEFORE /users/{id} to avoid route conflict
	huma.Register(s.api, huma.Operation{
		OperationID: "listPendingUsers",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/users/pending",
		Summary:     "List pending users",
		Description: "Lists all users awaiting approval",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListPendingUsers)

	// Search users - must be registered BEFORE /users/{id} to avoid route conflict
	huma.Register(s.api, huma.Operation{
		OperationID: "searchUsers",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/users/search",
		Summary:     "Search users",
		Description: "Searches users by name or email (for mapping UIs)",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleSearchUsers)

	huma.Register(s.api, huma.Operation{
		OperationID: "getAdminUser",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/users/{id}",
		Summary:     "Get user",
		Description: "Gets a user by ID",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetAdminUser)

	huma.Register(s.api, huma.Operation{
		OperationID: "updateAdminUser",
		Method:      http.MethodPatch,
		Path:        "/api/v1/admin/users/{id}",
		Summary:     "Update user",
		Description: "Updates a user",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleUpdateAdminUser)

	huma.Register(s.api, huma.Operation{
		OperationID: "deleteAdminUser",
		Method:      http.MethodDelete,
		Path:        "/api/v1/admin/users/{id}",
		Summary:     "Delete user",
		Description: "Deletes a user",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleDeleteAdminUser)

	// Open registration settings

	huma.Register(s.api, huma.Operation{
		OperationID: "approveUser",
		Method:      http.MethodPost,
		Path:        "/api/v1/admin/users/{id}/approve",
		Summary:     "Approve user",
		Description: "Approves a pending user, allowing them to log in",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleApproveUser)

	huma.Register(s.api, huma.Operation{
		OperationID: "denyUser",
		Method:      http.MethodPost,
		Path:        "/api/v1/admin/users/{id}/deny",
		Summary:     "Deny user",
		Description: "Denies a pending user registration",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleDenyUser)

	huma.Register(s.api, huma.Operation{
		OperationID: "setOpenRegistration",
		Method:      http.MethodPut,
		Path:        "/api/v1/admin/settings/open-registration",
		Summary:     "Set open registration",
		Description: "Enables or disables public registration",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleSetOpenRegistration)
}

// === DTOs ===

// CreateInviteRequest is the request body for creating an invite.
type CreateInviteRequest struct {
	Name          string `json:"name" validate:"required,min=1,max=100" doc:"Display name for the invitee"`
	Email         string `json:"email" validate:"required,email,max=254" doc:"Email address for the invitee"`
	Role          string `json:"role" validate:"required,oneof=admin member" doc:"Role to grant"`
	ExpiresInDays int    `json:"expires_in_days,omitempty" validate:"omitempty,gte=1,lte=365" doc:"Days until expiration (default 7)"`
}

// CreateInviteInput is the Huma input for creating an invite.
type CreateInviteInput struct {
	Authorization string `header:"Authorization"`
	Body          CreateInviteRequest
}

// InviteResponse is the API response for an invite.
type InviteResponse struct {
	ID        string    `json:"id" doc:"Invite ID"`
	Code      string    `json:"code" doc:"Invite code"`
	Name      string    `json:"name" doc:"Invitee name"`
	Email     string    `json:"email" doc:"Invitee email"`
	Role      string    `json:"role" doc:"Role granted"`
	ExpiresAt time.Time `json:"expires_at" doc:"Expiration time"`
	CreatedBy string    `json:"created_by" doc:"Creator user ID"`
	CreatedAt time.Time `json:"created_at" doc:"Creation time"`
	Status    string    `json:"status" doc:"Invite status"`
	URL       string    `json:"url,omitempty" doc:"Invite URL"`
}

// InviteOutput is the Huma output wrapper for an invite.
type InviteOutput struct {
	Body InviteResponse
}

// ListInvitesInput is the Huma input for listing invites.
type ListInvitesInput struct {
	Authorization string `header:"Authorization"`
}

// ListInvitesResponse is the API response for listing invites.
type ListInvitesResponse struct {
	Invites []InviteResponse `json:"invites" doc:"List of invites"`
	Total   int              `json:"total" doc:"Total count"`
}

// ListInvitesOutput is the Huma output wrapper for listing invites.
type ListInvitesOutput struct {
	Body ListInvitesResponse
}

// DeleteInviteInput is the Huma input for deleting an invite.
type DeleteInviteInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Invite ID"`
}

// ListUsersInput is the Huma input for listing users.
type ListUsersInput struct {
	Authorization string `header:"Authorization"`
}

// UserPermissionsResponse contains user permission flags in API responses.
type UserPermissionsResponse struct {
	CanDownload bool `json:"can_download" doc:"Whether user can download content"`
	CanShare    bool `json:"can_share" doc:"Whether user can share collections"`
}

// AdminUserResponse is the API response for a user in admin context.
type AdminUserResponse struct {
	ID          string                  `json:"id" doc:"User ID"`
	Email       string                  `json:"email" doc:"Email address"`
	DisplayName string                  `json:"display_name" doc:"Display name"`
	FirstName   string                  `json:"first_name" doc:"First name"`
	LastName    string                  `json:"last_name" doc:"Last name"`
	Role        string                  `json:"role" doc:"User role"`
	Status      string                  `json:"status" doc:"User status (active, pending)"`
	IsRoot      bool                    `json:"is_root" doc:"Is root user"`
	Permissions UserPermissionsResponse `json:"permissions" doc:"User permissions"`
	InvitedBy   string                  `json:"invited_by,omitempty" doc:"User ID who invited this user"`
	LastLoginAt time.Time               `json:"last_login_at,omitempty" doc:"Last login timestamp"`
	CreatedAt   time.Time               `json:"created_at" doc:"Creation time"`
	UpdatedAt   time.Time               `json:"updated_at" doc:"Last update time"`
}

// ListUsersResponse is the API response for listing users.
type ListUsersResponse struct {
	Users []AdminUserResponse `json:"users" doc:"List of users"`
	Total int                 `json:"total" doc:"Total count"`
}

// ListUsersOutput is the Huma output wrapper for listing users.
type ListUsersOutput struct {
	Body ListUsersResponse
}

// GetAdminUserInput is the Huma input for getting a user.
type GetAdminUserInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"User ID"`
}

// AdminUserOutput is the Huma output wrapper for a user.
type AdminUserOutput struct {
	Body AdminUserResponse
}

// UpdatePermissionsRequest contains optional permission updates in requests.
type UpdatePermissionsRequest struct {
	CanDownload *bool `json:"can_download,omitempty" doc:"Whether user can download content"`
	CanShare    *bool `json:"can_share,omitempty" doc:"Whether user can share collections"`
}

// UpdateAdminUserRequest is the request body for updating a user.
type UpdateAdminUserRequest struct {
	FirstName   *string                   `json:"first_name,omitempty" validate:"omitempty,min=1,max=100" doc:"First name"`
	LastName    *string                   `json:"last_name,omitempty" validate:"omitempty,min=1,max=100" doc:"Last name"`
	Role        *string                   `json:"role,omitempty" validate:"omitempty,oneof=admin member" doc:"User role"`
	Permissions *UpdatePermissionsRequest `json:"permissions,omitempty" doc:"User permissions"`
}

// UpdateAdminUserInput is the Huma input for updating a user.
type UpdateAdminUserInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"User ID"`
	Body          UpdateAdminUserRequest
}

// DeleteAdminUserInput is the Huma input for deleting a user.
type DeleteAdminUserInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"User ID"`
}

// ListPendingUsersInput is the Huma input for listing pending users.
type ListPendingUsersInput struct {
	Authorization string `header:"Authorization"`
}

// ApproveUserInput is the Huma input for approving a user.
type ApproveUserInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"User ID"`
}

// DenyUserInput is the Huma input for denying a user.
type DenyUserInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"User ID"`
}

// SetOpenRegistrationRequest is the request body for setting open registration.
type SetOpenRegistrationRequest struct {
	Enabled bool `json:"enabled" doc:"Whether open registration is enabled"`
}

// SetOpenRegistrationInput is the Huma input for setting open registration.
type SetOpenRegistrationInput struct {
	Authorization string `header:"Authorization"`
	Body          SetOpenRegistrationRequest
}

// SearchUsersInput is the Huma input for searching users.
type SearchUsersInput struct {
	Authorization string `header:"Authorization"`
	Query         string `query:"q" doc:"Search query (matches email, display name, first/last name)"`
	Limit         int    `query:"limit" default:"10" doc:"Maximum number of results to return"`
}

// UserSearchResult is a single user search result.
type UserSearchResult struct {
	ID          string `json:"id" doc:"User ID"`
	Email       string `json:"email" doc:"Email address"`
	DisplayName string `json:"display_name" doc:"Display name"`
	FirstName   string `json:"first_name" doc:"First name"`
	LastName    string `json:"last_name" doc:"Last name"`
}

// SearchUsersResponse is the API response for searching users.
type SearchUsersResponse struct {
	Users []UserSearchResult `json:"users" doc:"Matching users"`
	Total int                `json:"total" doc:"Total matches returned"`
}

// SearchUsersOutput is the Huma output wrapper for searching users.
type SearchUsersOutput struct {
	Body SearchUsersResponse
}

// === Handlers ===

func (s *Server) handleCreateInvite(ctx context.Context, input *CreateInviteInput) (*InviteOutput, error) {
	userID, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	role := domain.RoleMember
	if input.Body.Role == "admin" {
		role = domain.RoleAdmin
	}

	req := service.CreateInviteRequest{
		Name:          input.Body.Name,
		Email:         input.Body.Email,
		Role:          role,
		ExpiresInDays: input.Body.ExpiresInDays,
	}

	invite, err := s.services.Invite.CreateInvite(ctx, userID, req)
	if err != nil {
		return nil, err
	}

	return &InviteOutput{
		Body: InviteResponse{
			ID:        invite.ID,
			Code:      invite.Code,
			Name:      invite.Name,
			Email:     invite.Email,
			Role:      string(invite.Role),
			ExpiresAt: invite.ExpiresAt,
			CreatedBy: invite.CreatedBy,
			CreatedAt: invite.CreatedAt,
			Status:    invite.Status(),
			URL:       invite.URL,
		},
	}, nil
}

func (s *Server) handleListInvites(ctx context.Context, _ *ListInvitesInput) (*ListInvitesOutput, error) {
	if _, err := s.RequireAdmin(ctx); err != nil {
		return nil, err
	}

	invites, err := s.services.Invite.ListInvites(ctx)
	if err != nil {
		return nil, err
	}

	resp := make([]InviteResponse, len(invites))
	for i, inv := range invites {
		resp[i] = InviteResponse{
			ID:        inv.ID,
			Code:      inv.Code,
			Name:      inv.Name,
			Email:     inv.Email,
			Role:      string(inv.Role),
			ExpiresAt: inv.ExpiresAt,
			CreatedBy: inv.CreatedBy,
			CreatedAt: inv.CreatedAt,
			Status:    inv.Status(),
		}
	}

	return &ListInvitesOutput{
		Body: ListInvitesResponse{Invites: resp, Total: len(resp)},
	}, nil
}

func (s *Server) handleDeleteInvite(ctx context.Context, input *DeleteInviteInput) (*MessageOutput, error) {
	if _, err := s.RequireAdmin(ctx); err != nil {
		return nil, err
	}

	if err := s.services.Invite.DeleteInvite(ctx, input.ID); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "Invite deleted"}}, nil
}

func (s *Server) handleListUsers(ctx context.Context, _ *ListUsersInput) (*ListUsersOutput, error) {
	if _, err := s.RequireAdmin(ctx); err != nil {
		return nil, err
	}

	users, err := s.services.Admin.ListUsers(ctx)
	if err != nil {
		return nil, err
	}

	resp := MapSlice(users, mapAdminUserResponse)

	return &ListUsersOutput{
		Body: ListUsersResponse{Users: resp, Total: len(resp)},
	}, nil
}

func (s *Server) handleGetAdminUser(ctx context.Context, input *GetAdminUserInput) (*AdminUserOutput, error) {
	if _, err := s.RequireAdmin(ctx); err != nil {
		return nil, err
	}

	user, err := s.store.GetUser(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	return &AdminUserOutput{Body: mapAdminUserResponse(user)}, nil
}

func (s *Server) handleUpdateAdminUser(ctx context.Context, input *UpdateAdminUserInput) (*AdminUserOutput, error) {
	adminUserID, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Convert string role to domain.Role
	var role *domain.Role
	if input.Body.Role != nil {
		r := domain.Role(*input.Body.Role)
		role = &r
	}

	// Convert permissions update
	var perms *service.PermissionsUpdate
	if input.Body.Permissions != nil {
		perms = &service.PermissionsUpdate{
			CanDownload: input.Body.Permissions.CanDownload,
			CanShare:    input.Body.Permissions.CanShare,
		}
	}

	req := service.UpdateUserRequest{
		FirstName:   input.Body.FirstName,
		LastName:    input.Body.LastName,
		Role:        role,
		Permissions: perms,
	}

	user, err := s.services.Admin.UpdateUser(ctx, adminUserID, input.ID, req)
	if err != nil {
		return nil, err
	}

	return &AdminUserOutput{Body: mapAdminUserResponse(user)}, nil
}

func (s *Server) handleDeleteAdminUser(ctx context.Context, input *DeleteAdminUserInput) (*MessageOutput, error) {
	adminUserID, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.services.Admin.DeleteUser(ctx, adminUserID, input.ID); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "User deleted"}}, nil
}

func (s *Server) handleListPendingUsers(ctx context.Context, _ *ListPendingUsersInput) (*ListUsersOutput, error) {
	if _, err := s.RequireAdmin(ctx); err != nil {
		return nil, err
	}

	users, err := s.services.Admin.ListPendingUsers(ctx)
	if err != nil {
		return nil, err
	}

	resp := MapSlice(users, mapAdminUserResponse)

	return &ListUsersOutput{
		Body: ListUsersResponse{Users: resp, Total: len(resp)},
	}, nil
}

func (s *Server) handleApproveUser(ctx context.Context, input *ApproveUserInput) (*AdminUserOutput, error) {
	adminUserID, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	user, err := s.services.Admin.ApproveUser(ctx, adminUserID, input.ID)
	if err != nil {
		return nil, err
	}

	// Record user joined activity
	if s.services.Activity != nil {
		if err := s.services.Activity.RecordUserJoined(ctx, input.ID); err != nil {
			s.logger.Error("failed to record user joined activity", "user_id", input.ID, "error", err)
		}
	}

	return &AdminUserOutput{Body: mapAdminUserResponse(user)}, nil
}

func (s *Server) handleDenyUser(ctx context.Context, input *DenyUserInput) (*MessageOutput, error) {
	adminUserID, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.services.Admin.DenyUser(ctx, adminUserID, input.ID); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "User registration denied"}}, nil
}

func (s *Server) handleSetOpenRegistration(ctx context.Context, input *SetOpenRegistrationInput) (*MessageOutput, error) {
	if _, err := s.RequireAdmin(ctx); err != nil {
		return nil, err
	}

	if err := s.services.Instance.SetOpenRegistration(ctx, input.Body.Enabled); err != nil {
		return nil, err
	}

	message := "Open registration disabled"
	if input.Body.Enabled {
		message = "Open registration enabled"
	}

	return &MessageOutput{Body: MessageResponse{Message: message}}, nil
}

func (s *Server) handleSearchUsers(ctx context.Context, input *SearchUsersInput) (*SearchUsersOutput, error) {
	if _, err := s.RequireAdmin(ctx); err != nil {
		return nil, err
	}

	// Fetch all users and filter in memory (simple approach for now)
	// For larger user bases, consider adding a dedicated store method with prefix scanning
	users, err := s.services.Admin.ListUsers(ctx)
	if err != nil {
		return nil, err
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	query := strings.ToLower(strings.TrimSpace(input.Query))
	var results []UserSearchResult

	for _, u := range users {
		// Skip pending users - only active users can be mapped
		if u.Status == domain.UserStatusPending {
			continue
		}

		// Match against email, display name, first name, last name
		if query == "" ||
			strings.Contains(strings.ToLower(u.Email), query) ||
			strings.Contains(strings.ToLower(u.DisplayName), query) ||
			strings.Contains(strings.ToLower(u.FirstName), query) ||
			strings.Contains(strings.ToLower(u.LastName), query) {
			results = append(results, UserSearchResult{
				ID:          u.ID,
				Email:       u.Email,
				DisplayName: u.DisplayName,
				FirstName:   u.FirstName,
				LastName:    u.LastName,
			})
			if len(results) >= limit {
				break
			}
		}
	}

	return &SearchUsersOutput{
		Body: SearchUsersResponse{
			Users: results,
			Total: len(results),
		},
	}, nil
}

// mapAdminUserResponse converts a domain.User to AdminUserResponse.
func mapAdminUserResponse(u *domain.User) AdminUserResponse {
	status := string(u.Status)
	if status == "" {
		status = string(domain.UserStatusActive) // Backward compatibility
	}
	return AdminUserResponse{
		ID:          u.ID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		FirstName:   u.FirstName,
		LastName:    u.LastName,
		Role:        string(u.Role),
		Status:      status,
		IsRoot:      u.IsRoot,
		Permissions: UserPermissionsResponse{
			CanDownload: u.CanDownload(),
			CanShare:    u.CanShare(),
		},
		InvitedBy:   u.InvitedBy,
		LastLoginAt: u.LastLoginAt,
		CreatedAt:   u.CreatedAt,
		UpdatedAt:   u.UpdatedAt,
	}
}
