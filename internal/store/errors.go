package store

import (
	"fmt"
	"net/http"
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
