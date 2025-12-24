package api

import (
	"errors"

	"github.com/danielgtaylor/huma/v2"
	domainerrors "github.com/listenupapp/listenup-server/internal/errors"
)

// APIError is a custom error type that implements huma.StatusError.
// It maps domain errors to HTTP responses with consistent structure.
type APIError struct {
	status  int
	Code    string `json:"code" doc:"Machine-readable error code"`
	Message string `json:"message" doc:"Human-readable error message"`
	Details any    `json:"details,omitempty" doc:"Additional error details"`
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return e.Message
}

// GetStatus implements huma.StatusError.
func (e *APIError) GetStatus() int {
	return e.status
}

// ContentType returns the content type for the error response.
func (e *APIError) ContentType(ct string) string {
	return "application/json"
}

// RegisterErrorHandler configures huma to use domain errors.
// Call this after creating the huma.API but before registering routes.
func RegisterErrorHandler() {
	huma.NewError = func(status int, message string, errs ...error) huma.StatusError {
		// Check if any of the errors are domain errors
		for _, err := range errs {
			var domainErr *domainerrors.Error
			if errors.As(err, &domainErr) {
				return &APIError{
					status:  domainErr.HTTPStatus(),
					Code:    string(domainErr.Code),
					Message: domainErr.Message,
					Details: domainErr.Details,
				}
			}
		}

		// Map standard HTTP status codes to our error codes
		code := statusToCode(status)

		return &APIError{
			status:  status,
			Code:    code,
			Message: message,
		}
	}
}

// statusToCode maps HTTP status codes to our domain error codes.
func statusToCode(status int) string {
	switch status {
	case 400:
		return string(domainerrors.CodeValidation)
	case 401:
		return string(domainerrors.CodeUnauthorized)
	case 403:
		return string(domainerrors.CodeForbidden)
	case 404:
		return string(domainerrors.CodeNotFound)
	case 409:
		return string(domainerrors.CodeConflict)
	default:
		return string(domainerrors.CodeInternal)
	}
}
