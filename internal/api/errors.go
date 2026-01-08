package api

import (
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	domainerrors "github.com/listenupapp/listenup-server/internal/errors"
	"github.com/listenupapp/listenup-server/internal/store"
)

// APIError is a custom error type that implements huma.StatusError.
// It maps domain errors to HTTP responses with consistent structure.
type APIError struct { //nolint:revive // API prefix is intentional for clarity
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
func (e *APIError) ContentType(_ string) string {
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

			// Check for store "not found" errors and convert to 404
			if isNotFoundError(err) {
				return &APIError{
					status:  http.StatusNotFound,
					Code:    string(domainerrors.CodeNotFound),
					Message: err.Error(),
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

// isNotFoundError checks if the error is a "not found" type error from the store.
func isNotFoundError(err error) bool {
	// Check for store.Error type with 404 status code.
	// This catches ErrNotFound and all ErrNotFound.WithMessage() variants.
	var storeErr *store.Error
	if errors.As(err, &storeErr) && storeErr.HTTPCode() == 404 {
		return true
	}

	// Check for specific simple errors (errors.New style)
	return errors.Is(err, store.ErrBookNotFound) ||
		errors.Is(err, store.ErrContributorNotFound) ||
		errors.Is(err, store.ErrSeriesNotFound) ||
		errors.Is(err, store.ErrCollectionNotFound) ||
		errors.Is(err, store.ErrUserNotFound) ||
		errors.Is(err, store.ErrGenreNotFound) ||
		errors.Is(err, store.ErrTagNotFound) ||
		errors.Is(err, store.ErrInviteNotFound) ||
		errors.Is(err, store.ErrSessionNotFound) ||
		errors.Is(err, store.ErrLensNotFound) ||
		errors.Is(err, store.ErrLibraryNotFound) ||
		errors.Is(err, store.ErrShareNotFound) ||
		errors.Is(err, store.ErrServerNotFound)
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
