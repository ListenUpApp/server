package store

import (
	"errors"
	"fmt"
	"net/http"
)

// Domain-specific sentinel errors.
var (
	ErrBookNotFound            = errors.New("book not found")
	ErrBookExists              = errors.New("book already exists")
	ErrLensNotFound            = errors.New("lens not found")
	ErrDuplicateLens           = errors.New("lens already exists")
	ErrLibraryNotFound         = errors.New("library not found")
	ErrCollectionNotFound      = errors.New("collection not found")
	ErrDuplicateLibrary        = errors.New("library already exists")
	ErrDuplicateCollection     = errors.New("collection already exists")
	ErrPermissionDenied        = errors.New("insufficient permissions")
	ErrServerNotFound          = errors.New("server not found")
	ErrServerAlreadyExists     = errors.New("server already exists")
	ErrUserNotFound            = errors.New("user not found")
	ErrEmailExists             = errors.New("email already exists")
	ErrContributorNotFound     = errors.New("contributor not found")
	ErrSeriesNotFound          = errors.New("series not found")
	ErrInviteNotFound          = errors.New("invite not found")
	ErrInviteCodeExists        = errors.New("invite code already exists")
	ErrProgressNotFound        = errors.New("playback progress not found")
	ErrBookPreferencesNotFound = errors.New("book preferences not found")
	ErrProfileNotFound         = errors.New("profile not found")
	ErrTagNotFound             = errors.New("tag not found")
	ErrGenreNotFound           = errors.New("genre not found")
	ErrSessionNotFound         = errors.New("session not found")
	ErrShareNotFound           = errors.New("share not found")
	ErrShelfNotFound           = ErrLensNotFound
)

// Error is a domain error with an HTTP status code.
type Error struct {
	Code    int    // HTTP status code
	Message string // User-facing message
	Err     error  // Underlying error (optional)
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Err }

// HTTPCode returns the HTTP status code associated with this error.
func (e *Error) HTTPCode() int { return e.Code }

// WithMessage returns a new error with a custom message.
func (e *Error) WithMessage(msg string) *Error {
	return &Error{
		Code:    e.Code,
		Message: msg,
		Err:     e.Err,
	}
}

// WithCause wraps an underlying error.
func (e *Error) WithCause(err error) *Error {
	return &Error{
		Code:    e.Code,
		Message: e.Message,
		Err:     err,
	}
}

// Sentinel errors.
var (
	ErrNotFound = &Error{
		Code:    http.StatusNotFound,
		Message: "resource not found",
	}

	ErrAlreadyExists = &Error{
		Code:    http.StatusConflict,
		Message: "resource already exists",
	}

	ErrInvalidInput = &Error{
		Code:    http.StatusBadRequest,
		Message: "invalid input",
	}

	ErrUnauthorized = &Error{
		Code:    http.StatusUnauthorized,
		Message: "unauthorized",
	}

	ErrForbidden = &Error{
		Code:    http.StatusForbidden,
		Message: "forbidden",
	}
)
