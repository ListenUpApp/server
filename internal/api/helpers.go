package api

import (
	"context"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	domainerrors "github.com/listenupapp/listenup-server/internal/errors"
)

// authenticateRequest validates the Authorization header and returns the user ID.
func (s *Server) authenticateRequest(ctx context.Context, authHeader string) (string, error) {
	if authHeader == "" {
		return "", huma.Error401Unauthorized("Missing authorization header")
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return "", huma.Error401Unauthorized("Invalid authorization header format")
	}

	user, _, err := s.services.Auth.VerifyAccessToken(ctx, parts[1])
	if err != nil {
		return "", huma.Error401Unauthorized("Invalid or expired token")
	}

	return user.ID, nil
}

// authenticateAndRequireAdmin validates the token and requires admin role.
func (s *Server) authenticateAndRequireAdmin(ctx context.Context, authHeader string) (string, error) {
	userID, err := s.authenticateRequest(ctx, authHeader)
	if err != nil {
		return "", err
	}

	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return "", huma.Error401Unauthorized("User not found")
	}

	if !user.IsAdmin() {
		return "", domainerrors.Forbidden("Admin access required")
	}

	return userID, nil
}
