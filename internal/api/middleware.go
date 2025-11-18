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
		user, claims, err := s.authService.VerifyAccessToken(r.Context(), tokenString)
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

// requireRoot is middleware that ensures the authenticated user is a root user.
// Must be used after requireAuth.
func (s *Server) requireRoot(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isRoot, ok := r.Context().Value(contextKeyIsRoot).(bool)
		if !ok || !isRoot {
			response.Forbidden(w, "Root access required", s.logger)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// getUserID extracts the authenticated user ID from request context.
// Returns empty string if not authenticated.
func getUserID(ctx context.Context) string {
	if userID, ok := ctx.Value(contextKeyUserID).(string); ok {
		return userID
	}
	return ""
}

// getEmail extracts the authenticated user email from request context.
// Returns empty string if not authenticated.
func getEmail(ctx context.Context) string {
	if email, ok := ctx.Value(contextKeyEmail).(string); ok {
		return email
	}
	return ""
}

// isRoot checks if the authenticated user is a root user.
// Returns false if not authenticated.
func isRoot(ctx context.Context) bool {
	if isRoot, ok := ctx.Value(contextKeyIsRoot).(bool); ok {
		return isRoot
	}
	return false
}

// getSessionID extracts the session ID from request context.
// Returns empty string if not available.
func getSessionID(ctx context.Context) string {
	if sessionID, ok := ctx.Value(contextKeySessionID).(string); ok {
		return sessionID
	}
	return ""
}
