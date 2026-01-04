package api

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5/middleware"
)

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
				// For 4xx errors, also log the response body to help debug validation issues
				if len(ww.body) > 0 && len(ww.body) < 2000 {
					attrs = append(attrs, slog.String("response_body", string(ww.body)))
				}
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

// EnvelopeVersion is the current API envelope version.
// Clients validate this to detect response structure mismatches early.
const EnvelopeVersion = 1

// APIEnvelope wraps all API responses in a consistent format.
// This maintains compatibility with clients expecting {v, success, data, error} structure.
// The "v" field is a canary - if it's missing or wrong, clients know the structure is broken.
type APIEnvelope struct { //nolint:revive // API prefix is intentional for clarity
	Version int    `json:"v"`
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

// APIErrorEnvelope is used for error responses that include code and details.
// Some errors (like 409 Conflict for disambiguation) need to return structured data.
// The "v" field is a canary - if it's missing or wrong, clients know the structure is broken.
type APIErrorEnvelope struct { //nolint:revive // API prefix is intentional for clarity
	Version int    `json:"v"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// EnvelopeTransformer wraps Huma responses in the standard API envelope format.
// Clients expect responses wrapped as: {"v": 1, "success": bool, "data": ..., "error": ...}
// For errors with details (like disambiguation), returns full error structure.
// The version field (v) acts as a canary - if parsing succeeds but v is missing,
// clients know the response structure doesn't match expectations.
func EnvelopeTransformer(_ huma.Context, status string, v any) (any, error) {
	// Parse status code to determine success
	statusCode, err := strconv.Atoi(status)
	if err != nil {
		statusCode = 200 // Default to success if parsing fails
	}

	success := statusCode < 400

	if success {
		return APIEnvelope{
			Version: EnvelopeVersion,
			Success: true,
			Data:    v,
		}, nil
	}

	// For APIErrors with details, return full error structure (code, message, details)
	// This is needed for 409 Conflict responses that include disambiguation candidates
	if apiErr, ok := v.(*APIError); ok && apiErr.Details != nil {
		return APIErrorEnvelope{
			Version: EnvelopeVersion,
			Code:    apiErr.Code,
			Message: apiErr.Message,
			Details: apiErr.Details,
		}, nil
	}

	// For simple errors, use the standard envelope
	errorMsg := extractErrorMessage(v)

	return APIEnvelope{
		Version: EnvelopeVersion,
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
