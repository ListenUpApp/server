package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

func (s *Server) registerUserRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "getCurrentUser",
		Method:      http.MethodGet,
		Path:        "/api/v1/users/me",
		Summary:     "Get current user",
		Description: "Returns the authenticated user's information",
		Tags:        []string{"Users"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetCurrentUser)
}

// AuthenticatedInput contains the authorization header for authenticated requests.
type AuthenticatedInput struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
}

// UserOutput wraps the user response for Huma.
type UserOutput struct {
	Body UserResponse
}

func (s *Server) handleGetCurrentUser(ctx context.Context, input *AuthenticatedInput) (*UserOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &UserOutput{
		Body: UserResponse{
			ID:          user.ID,
			Email:       user.Email,
			DisplayName: user.DisplayName,
			FirstName:   user.FirstName,
			LastName:    user.LastName,
			IsRoot:      user.IsRoot,
			CreatedAt:   user.CreatedAt,
			UpdatedAt:   user.UpdatedAt,
			LastLoginAt: user.LastLoginAt,
		},
	}, nil
}
