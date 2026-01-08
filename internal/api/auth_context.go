package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	domainerrors "github.com/listenupapp/listenup-server/internal/errors"
	"github.com/listenupapp/listenup-server/internal/service"
)

// ctxKey is the type for context keys to avoid collisions.
type ctxKey string

// userIDKey is the context key for the authenticated user ID.
const userIDKey ctxKey = "userID"

// GetUserID returns the authenticated user ID from context.
// Returns 401 error if user is not authenticated.
func GetUserID(ctx context.Context) (string, error) {
	userID, ok := ctx.Value(userIDKey).(string)
	if !ok || userID == "" {
		return "", huma.Error401Unauthorized("Authentication required")
	}
	return userID, nil
}

// setUserID stores the user ID in context.
func setUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// authMiddleware returns a middleware that validates Bearer tokens and stores user ID in context.
// If no token is present or invalid, continues without user in context.
// Handlers use GetUserID to check authentication.
func authMiddleware(auth *service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				next.ServeHTTP(w, r)
				return
			}

			token := authHeader[7:]
			user, _, err := auth.VerifyAccessToken(r.Context(), token)
			if err != nil {
				// Invalid token - continue without user (handler will reject if auth required)
				next.ServeHTTP(w, r)
				return
			}

			ctx := setUserID(r.Context(), user.ID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdmin validates the user is authenticated and has admin role.
// Returns the user ID if successful, error otherwise.
func (s *Server) RequireAdmin(ctx context.Context) (string, error) {
	userID, err := GetUserID(ctx)
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
