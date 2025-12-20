package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/listenupapp/listenup-server/internal/http/response"
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const (
	contextKeyUserID    contextKey = "user_id"
	contextKeyEmail     contextKey = "email"
	contextKeyIsRoot    contextKey = "is_root"
	contextKeySessionID contextKey = "session_id"
)

// requireAuth is middleware that validates access tokens and attaches user context.
// "Life before death" - authentication comes before authorization.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract token from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			response.Unauthorized(w, "Missing authorization header", s.logger)
			return
		}

		// Parse Bearer token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			response.Unauthorized(w, "Invalid authorization header format", s.logger)
			return
		}

		tokenString := parts[1]

		// Verify token
		user, claims, err := s.services.Auth.VerifyAccessToken(r.Context(), tokenString)
		if err != nil {
			response.Unauthorized(w, "Invalid or expired token", s.logger)
			return
		}

		// Attach user context
		ctx := context.WithValue(r.Context(), contextKeyUserID, user.ID)
		ctx = context.WithValue(ctx, contextKeyEmail, user.Email)
		ctx = context.WithValue(ctx, contextKeyIsRoot, user.IsRoot)
		ctx = context.WithValue(ctx, contextKeySessionID, claims.TokenID)

		// Continue to next handler
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireAdmin is middleware that checks for administrative privileges.
// Must be used after requireAuth. Checks if the user is root or has admin role.
func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := mustGetUserID(ctx) // Safe: requireAdmin is always used after requireAuth

		// Get user to check admin status
		user, err := s.store.GetUser(ctx, userID)
		if err != nil {
			response.Unauthorized(w, "User not found", s.logger)
			return
		}

		if !user.IsAdmin() {
			response.Forbidden(w, "Admin access required", s.logger)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// getUserID extracts the authenticated user ID from request context.
// Returns empty string if not authenticated.
// Use mustGetUserID for routes protected by requireAuth middleware.
func getUserID(ctx context.Context) string {
	if userID, ok := ctx.Value(contextKeyUserID).(string); ok {
		return userID
	}
	return ""
}

// mustGetUserID extracts the authenticated user ID from request context.
// Panics if called on an unauthenticated route - this catches configuration
// bugs during development (forgetting to add requireAuth middleware).
// Use only on routes protected by requireAuth middleware.
func mustGetUserID(ctx context.Context) string {
	userID, ok := ctx.Value(contextKeyUserID).(string)
	if !ok || userID == "" {
		panic("mustGetUserID: called without authentication - missing requireAuth middleware on route")
	}
	return userID
}
