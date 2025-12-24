package api

import (
	"context"
	"net/http"
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
}

// === DTOs ===

type CreateInviteRequest struct {
	Name          string `json:"name" validate:"required,max=100" doc:"Display name for the invitee"`
	Email         string `json:"email" validate:"required,email" doc:"Email address for the invitee"`
	Role          string `json:"role" validate:"required,oneof=admin member" doc:"Role to grant"`
	ExpiresInDays int    `json:"expires_in_days,omitempty" doc:"Days until expiration (default 7)"`
}

type CreateInviteInput struct {
	Authorization string `header:"Authorization"`
	Body          CreateInviteRequest
}

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

type InviteOutput struct {
	Body InviteResponse
}

type ListInvitesInput struct {
	Authorization string `header:"Authorization"`
}

type ListInvitesResponse struct {
	Invites []InviteResponse `json:"invites" doc:"List of invites"`
	Total   int              `json:"total" doc:"Total count"`
}

type ListInvitesOutput struct {
	Body ListInvitesResponse
}

type DeleteInviteInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Invite ID"`
}

type ListUsersInput struct {
	Authorization string `header:"Authorization"`
}

type AdminUserResponse struct {
	ID        string    `json:"id" doc:"User ID"`
	Email     string    `json:"email" doc:"Email address"`
	FirstName string    `json:"first_name" doc:"First name"`
	LastName  string    `json:"last_name" doc:"Last name"`
	Role      string    `json:"role" doc:"User role"`
	IsRoot    bool      `json:"is_root" doc:"Is root user"`
	CreatedAt time.Time `json:"created_at" doc:"Creation time"`
	UpdatedAt time.Time `json:"updated_at" doc:"Last update time"`
}

type ListUsersResponse struct {
	Users []AdminUserResponse `json:"users" doc:"List of users"`
	Total int                 `json:"total" doc:"Total count"`
}

type ListUsersOutput struct {
	Body ListUsersResponse
}

type GetAdminUserInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"User ID"`
}

type AdminUserOutput struct {
	Body AdminUserResponse
}

type UpdateAdminUserRequest struct {
	FirstName *string `json:"first_name,omitempty" doc:"First name"`
	LastName  *string `json:"last_name,omitempty" doc:"Last name"`
	Role      *string `json:"role,omitempty" validate:"omitempty,oneof=admin member" doc:"User role"`
}

type UpdateAdminUserInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"User ID"`
	Body          UpdateAdminUserRequest
}

type DeleteAdminUserInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"User ID"`
}

// === Handlers ===

func (s *Server) handleCreateInvite(ctx context.Context, input *CreateInviteInput) (*InviteOutput, error) {
	userID, err := s.authenticateAndRequireAdmin(ctx, input.Authorization)
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

func (s *Server) handleListInvites(ctx context.Context, input *ListInvitesInput) (*ListInvitesOutput, error) {
	if _, err := s.authenticateAndRequireAdmin(ctx, input.Authorization); err != nil {
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
	if _, err := s.authenticateAndRequireAdmin(ctx, input.Authorization); err != nil {
		return nil, err
	}

	if err := s.services.Invite.DeleteInvite(ctx, input.ID); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "Invite deleted"}}, nil
}

func (s *Server) handleListUsers(ctx context.Context, input *ListUsersInput) (*ListUsersOutput, error) {
	if _, err := s.authenticateAndRequireAdmin(ctx, input.Authorization); err != nil {
		return nil, err
	}

	users, err := s.services.Admin.ListUsers(ctx)
	if err != nil {
		return nil, err
	}

	resp := make([]AdminUserResponse, len(users))
	for i, u := range users {
		resp[i] = AdminUserResponse{
			ID:        u.ID,
			Email:     u.Email,
			FirstName: u.FirstName,
			LastName:  u.LastName,
			Role:      string(u.Role),
			IsRoot:    u.IsRoot,
			CreatedAt: u.CreatedAt,
			UpdatedAt: u.UpdatedAt,
		}
	}

	return &ListUsersOutput{
		Body: ListUsersResponse{Users: resp, Total: len(resp)},
	}, nil
}

func (s *Server) handleGetAdminUser(ctx context.Context, input *GetAdminUserInput) (*AdminUserOutput, error) {
	if _, err := s.authenticateAndRequireAdmin(ctx, input.Authorization); err != nil {
		return nil, err
	}

	user, err := s.store.GetUser(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	return &AdminUserOutput{
		Body: AdminUserResponse{
			ID:        user.ID,
			Email:     user.Email,
			FirstName: user.FirstName,
			LastName:  user.LastName,
			Role:      string(user.Role),
			IsRoot:    user.IsRoot,
			CreatedAt: user.CreatedAt,
			UpdatedAt: user.UpdatedAt,
		},
	}, nil
}

func (s *Server) handleUpdateAdminUser(ctx context.Context, input *UpdateAdminUserInput) (*AdminUserOutput, error) {
	adminUserID, err := s.authenticateAndRequireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	// Convert string role to domain.Role
	var role *domain.Role
	if input.Body.Role != nil {
		r := domain.Role(*input.Body.Role)
		role = &r
	}

	req := service.UpdateUserRequest{
		FirstName: input.Body.FirstName,
		LastName:  input.Body.LastName,
		Role:      role,
	}

	user, err := s.services.Admin.UpdateUser(ctx, adminUserID, input.ID, req)
	if err != nil {
		return nil, err
	}

	return &AdminUserOutput{
		Body: AdminUserResponse{
			ID:        user.ID,
			Email:     user.Email,
			FirstName: user.FirstName,
			LastName:  user.LastName,
			Role:      string(user.Role),
			IsRoot:    user.IsRoot,
			CreatedAt: user.CreatedAt,
			UpdatedAt: user.UpdatedAt,
		},
	}, nil
}

func (s *Server) handleDeleteAdminUser(ctx context.Context, input *DeleteAdminUserInput) (*MessageOutput, error) {
	adminUserID, err := s.authenticateAndRequireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	if err := s.services.Admin.DeleteUser(ctx, adminUserID, input.ID); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "User deleted"}}, nil
}
