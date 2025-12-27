package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5/middleware"
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

// StructuredLogger returns a middleware that logs requests using structured logging.
// It captures: method, path, status, duration, request_id, client_ip, bytes_written.
// Log level varies by status code: INFO for 2xx/3xx, WARN for 4xx, ERROR for 5xx.
func StructuredLogger(logger *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture status code and body for error logging
			ww := &responseCapture{
				ResponseWriter: w,
				ww:             middleware.NewWrapResponseWriter(w, r.ProtoMajor),
			}

			// Process request
			ww.ww.Tee(ww) // Capture response body for error logging
			next.ServeHTTP(ww.ww, r)

			// Calculate duration
			duration := time.Since(start)

			// Get request ID from chi middleware
			requestID := middleware.GetReqID(r.Context())

			// Build log attributes
			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", ww.ww.Status()),
				slog.Duration("duration", duration),
				slog.String("request_id", requestID),
				slog.String("client_ip", r.RemoteAddr),
				slog.Int("bytes", ww.ww.BytesWritten()),
			}

			// Add query string if present (but not for sensitive endpoints)
			if r.URL.RawQuery != "" && !isSensitivePath(r.URL.Path) {
				attrs = append(attrs, slog.String("query", r.URL.RawQuery))
			}

			// Choose log level based on status code
			status := ww.ww.Status()
			switch {
			case status >= 500:
				// For 5xx errors, also log the response body to help debug
				if len(ww.body) > 0 && len(ww.body) < 2000 {
					attrs = append(attrs, slog.String("response_body", string(ww.body)))
				}
				logger.LogAttrs(r.Context(), slog.LevelError, "request completed", attrs...)
			case status >= 400:
				logger.LogAttrs(r.Context(), slog.LevelWarn, "request completed", attrs...)
			default:
				logger.LogAttrs(r.Context(), slog.LevelInfo, "request completed", attrs...)
			}
		})
	}
}

// responseCapture captures the response body for error logging.
type responseCapture struct {
	http.ResponseWriter
	ww   middleware.WrapResponseWriter
	body []byte
}

func (rc *responseCapture) Write(b []byte) (int, error) {
	// Only capture first 2KB of response for logging
	if len(rc.body) < 2000 {
		remaining := 2000 - len(rc.body)
		if len(b) > remaining {
			rc.body = append(rc.body, b[:remaining]...)
		} else {
			rc.body = append(rc.body, b...)
		}
	}
	return len(b), nil
}

// SecurityHeaders returns a middleware that adds security headers to responses.
// These headers protect against common web vulnerabilities even in self-hosted contexts.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent clickjacking - deny all framing
		w.Header().Set("X-Frame-Options", "DENY")

		// Prevent MIME type sniffing attacks
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Control referrer information sent with requests
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Legacy XSS protection (still useful for older browsers)
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Restrict access to sensitive browser features
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		next.ServeHTTP(w, r)
	})
}

// isSensitivePath returns true if the path might contain sensitive data in query params.
func isSensitivePath(path string) bool {
	sensitivePaths := []string{
		"/api/v1/auth/",
		"/api/v1/invites/",
	}
	for _, sp := range sensitivePaths {
		if len(path) >= len(sp) && path[:len(sp)] == sp {
			return true
		}
	}
	return false
}

// APIEnvelope wraps all API responses in a consistent format.
// This maintains compatibility with clients expecting {success, data, error} structure.
type APIEnvelope struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

// APIErrorEnvelope is used for error responses that include code and details.
// Some errors (like 409 Conflict for disambiguation) need to return structured data.
type APIErrorEnvelope struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// EnvelopeTransformer wraps Huma responses in the standard API envelope format.
// Clients expect responses wrapped as: {"success": bool, "data": ..., "error": ...}
// For errors with details (like disambiguation), returns full error structure.
func EnvelopeTransformer(_ huma.Context, status string, v any) (any, error) {
	// Parse status code to determine success
	statusCode, err := strconv.Atoi(status)
	if err != nil {
		statusCode = 200 // Default to success if parsing fails
	}

	success := statusCode < 400

	if success {
		return APIEnvelope{
			Success: true,
			Data:    v,
		}, nil
	}

	// For APIErrors with details, return full error structure (code, message, details)
	// This is needed for 409 Conflict responses that include disambiguation candidates
	if apiErr, ok := v.(*APIError); ok && apiErr.Details != nil {
		return APIErrorEnvelope{
			Code:    apiErr.Code,
			Message: apiErr.Message,
			Details: apiErr.Details,
		}, nil
	}

	// For simple errors, use the standard envelope
	errorMsg := extractErrorMessage(v)

	return APIEnvelope{
		Success: false,
		Error:   errorMsg,
	}, nil
}

// extractErrorMessage extracts a human-readable error message from various error types.
func extractErrorMessage(v any) string {
	// Try our custom APIError
	if apiErr, ok := v.(*APIError); ok {
		return apiErr.Message
	}

	// Try huma's ErrorModel (struct with Title, Detail, etc.)
	if errModel, ok := v.(*huma.ErrorModel); ok {
		if errModel.Detail != "" {
			return errModel.Detail
		}
		if errModel.Title != "" {
			return errModel.Title
		}
	}

	// Try map[string]any (generic error response)
	if errMap, ok := v.(map[string]any); ok {
		if msg, ok := errMap["message"].(string); ok {
			return msg
		}
		if detail, ok := errMap["detail"].(string); ok {
			return detail
		}
		if title, ok := errMap["title"].(string); ok {
			return title
		}
	}

	// Try error interface
	if err, ok := v.(error); ok {
		return err.Error()
	}

	return "An error occurred"
}
