// Package errors provides standardized domain errors with codes for the ListenUp API.
//
// Usage:
//
//	// In services - return typed errors
//	if userExists {
//	    return errors.AlreadyExists("email already in use")
//	}
//
//	// In handlers - check with errors.Is
//	if errors.Is(err, errors.ErrAlreadyExists) {
//	    response.Conflict(w, err.Error(), logger)
//	    return
//	}
//
//	// Or use the Code directly for switch statements
//	var domainErr *errors.Error
//	if errors.As(err, &domainErr) {
//	    switch domainErr.Code {
//	    case errors.CodeAlreadyExists:
//	        response.Conflict(w, domainErr.Message, logger)
//	    case errors.CodeUnauthorized:
//	        response.Unauthorized(w, domainErr.Message, logger)
//	    }
//	}
package errors

import (
	"errors"
	"fmt"
	"net/http"
)

// Re-export standard library functions for convenience.
var (
	Is     = errors.Is
	As     = errors.As
	Unwrap = errors.Unwrap
	Join   = errors.Join
)

// Code represents a machine-readable error code.
type Code string

// Error codes used throughout the application.
const (
	CodeNotFound           Code = "NOT_FOUND"
	CodeAlreadyExists      Code = "ALREADY_EXISTS"
	CodeUnauthorized       Code = "UNAUTHORIZED"
	CodeForbidden          Code = "FORBIDDEN"
	CodeValidation         Code = "VALIDATION"
	CodeConflict           Code = "CONFLICT"
	CodeInternal           Code = "INTERNAL"
	CodeAlreadyConfigured  Code = "ALREADY_CONFIGURED"
	CodeInvalidCredentials Code = "INVALID_CREDENTIALS"
	CodeTokenExpired       Code = "TOKEN_EXPIRED"
)

// HTTPStatus returns the appropriate HTTP status code for an error code.
func (c Code) HTTPStatus() int {
	switch c {
	case CodeNotFound:
		return http.StatusNotFound
	case CodeAlreadyExists, CodeConflict, CodeAlreadyConfigured:
		return http.StatusConflict
	case CodeUnauthorized, CodeInvalidCredentials, CodeTokenExpired:
		return http.StatusUnauthorized
	case CodeForbidden:
		return http.StatusForbidden
	case CodeValidation:
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// Error is a domain error with a code, message, and optional details.
type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
	cause   error  // unexported, for wrapping
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.cause)
	}
	return e.Message
}

// Unwrap returns the underlying error.
func (e *Error) Unwrap() error {
	return e.cause
}

// Is reports whether target matches this error.
// Matches if target is an *Error with the same Code.
func (e *Error) Is(target error) bool {
	var t *Error
	if errors.As(target, &t) {
		return e.Code == t.Code
	}
	return false
}

// HTTPStatus returns the HTTP status code for this error.
func (e *Error) HTTPStatus() int {
	return e.Code.HTTPStatus()
}

// WithDetails returns a new error with additional details.
func (e *Error) WithDetails(details any) *Error {
	return &Error{
		Code:    e.Code,
		Message: e.Message,
		Details: details,
		cause:   e.cause,
	}
}

// WithCause wraps an underlying error.
func (e *Error) WithCause(err error) *Error {
	return &Error{
		Code:    e.Code,
		Message: e.Message,
		Details: e.Details,
		cause:   err,
	}
}

// Sentinel errors for use with errors.Is().
var (
	ErrNotFound           = &Error{Code: CodeNotFound, Message: "not found"}
	ErrAlreadyExists      = &Error{Code: CodeAlreadyExists, Message: "already exists"}
	ErrUnauthorized       = &Error{Code: CodeUnauthorized, Message: "unauthorized"}
	ErrForbidden          = &Error{Code: CodeForbidden, Message: "forbidden"}
	ErrValidation         = &Error{Code: CodeValidation, Message: "validation error"}
	ErrConflict           = &Error{Code: CodeConflict, Message: "conflict"}
	ErrInternal           = &Error{Code: CodeInternal, Message: "internal error"}
	ErrAlreadyConfigured  = &Error{Code: CodeAlreadyConfigured, Message: "already configured"}
	ErrInvalidCredentials = &Error{Code: CodeInvalidCredentials, Message: "invalid credentials"}
	ErrTokenExpired       = &Error{Code: CodeTokenExpired, Message: "token expired"}
)

// Constructor functions for creating errors with custom messages.

// NotFound creates a not found error.
func NotFound(msg string) *Error {
	return &Error{Code: CodeNotFound, Message: msg}
}

// NotFoundf creates a not found error with formatted message.
func NotFoundf(format string, args ...any) *Error {
	return &Error{Code: CodeNotFound, Message: fmt.Sprintf(format, args...)}
}

// AlreadyExists creates an already exists error.
func AlreadyExists(msg string) *Error {
	return &Error{Code: CodeAlreadyExists, Message: msg}
}

// AlreadyExistsf creates an already exists error with formatted message.
func AlreadyExistsf(format string, args ...any) *Error {
	return &Error{Code: CodeAlreadyExists, Message: fmt.Sprintf(format, args...)}
}

// Unauthorized creates an unauthorized error.
func Unauthorized(msg string) *Error {
	return &Error{Code: CodeUnauthorized, Message: msg}
}

// Unauthorizedf creates an unauthorized error with formatted message.
func Unauthorizedf(format string, args ...any) *Error {
	return &Error{Code: CodeUnauthorized, Message: fmt.Sprintf(format, args...)}
}

// Forbidden creates a forbidden error.
func Forbidden(msg string) *Error {
	return &Error{Code: CodeForbidden, Message: msg}
}

// Forbiddenf creates a forbidden error with formatted message.
func Forbiddenf(format string, args ...any) *Error {
	return &Error{Code: CodeForbidden, Message: fmt.Sprintf(format, args...)}
}

// Validation creates a validation error.
func Validation(msg string) *Error {
	return &Error{Code: CodeValidation, Message: msg}
}

// Validationf creates a validation error with formatted message.
func Validationf(format string, args ...any) *Error {
	return &Error{Code: CodeValidation, Message: fmt.Sprintf(format, args...)}
}

// ValidationWithDetails creates a validation error with details.
func ValidationWithDetails(msg string, details any) *Error {
	return &Error{Code: CodeValidation, Message: msg, Details: details}
}

// Conflict creates a conflict error.
func Conflict(msg string) *Error {
	return &Error{Code: CodeConflict, Message: msg}
}

// Conflictf creates a conflict error with formatted message.
func Conflictf(format string, args ...any) *Error {
	return &Error{Code: CodeConflict, Message: fmt.Sprintf(format, args...)}
}

// Internal creates an internal error.
func Internal(msg string) *Error {
	return &Error{Code: CodeInternal, Message: msg}
}

// Internalf creates an internal error with formatted message.
func Internalf(format string, args ...any) *Error {
	return &Error{Code: CodeInternal, Message: fmt.Sprintf(format, args...)}
}

// Wrap wraps an error with a code and message.
func Wrap(err error, code Code, msg string) *Error {
	return &Error{Code: code, Message: msg, cause: err}
}

// Wrapf wraps an error with a code and formatted message.
func Wrapf(err error, code Code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), cause: err}
}

// AlreadyConfigured creates an already configured error.
func AlreadyConfigured(msg string) *Error {
	return &Error{Code: CodeAlreadyConfigured, Message: msg}
}

// InvalidCredentials creates an invalid credentials error.
func InvalidCredentials(msg string) *Error {
	return &Error{Code: CodeInvalidCredentials, Message: msg}
}

// TokenExpired creates a token expired error.
func TokenExpired(msg string) *Error {
	return &Error{Code: CodeTokenExpired, Message: msg}
}
