// Package response provides standardized HTTP response formatting and error handling utilities.
package response

import (
	"encoding/json/v2"
	"errors"
	"log/slog"
	"net/http"

	"github.com/listenupapp/listenup-server/internal/store"
)

// Envelope provides a consistent JSON response structure.
type Envelope struct {
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
	Success bool   `json:"success"`
}

// JSON writes a JSON response with the given status code using json/v2.
func JSON(w http.ResponseWriter, status int, data any, logger *slog.Logger) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	envelope := Envelope{
		Success: status < 400,
		Data:    data,
	}

	// json/v2 MarshalWrite doesn't add a newline, but that's fine for HTTP responses.
	if err := json.MarshalWrite(w, envelope); err != nil {
		if logger != nil {
			logger.Error("Failed to encode JSON response", "error", err)
		}
	}
}

// Success writes a successful JSON response (200 OK).
func Success(w http.ResponseWriter, data any, logger *slog.Logger) {
	JSON(w, http.StatusOK, data, logger)
}

// Created writes a created response (201 Created).
func Created(w http.ResponseWriter, data any, logger *slog.Logger) {
	JSON(w, http.StatusCreated, data, logger)
}

// NoContent writes a no content response (204 No Content).
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// Error writes an error response with the given status code.
func Error(w http.ResponseWriter, status int, message string, logger *slog.Logger) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	envelope := Envelope{
		Success: false,
		Error:   message,
	}

	if err := json.MarshalWrite(w, envelope); err != nil {
		if logger != nil {
			logger.Error("Failed to encode error response", "error", err)
		}
	}
}

// BadRequest writes a 400 Bad Request response.
func BadRequest(w http.ResponseWriter, message string, logger *slog.Logger) {
	Error(w, http.StatusBadRequest, message, logger)
}

// Unauthorized writes a 401 Unauthorized response.
func Unauthorized(w http.ResponseWriter, message string, logger *slog.Logger) {
	Error(w, http.StatusUnauthorized, message, logger)
}

// Forbidden writes a 403 Forbidden response.
func Forbidden(w http.ResponseWriter, message string, logger *slog.Logger) {
	Error(w, http.StatusForbidden, message, logger)
}

// NotFound writes a 404 Not Found response.
func NotFound(w http.ResponseWriter, message string, logger *slog.Logger) {
	Error(w, http.StatusNotFound, message, logger)
}

// InternalError writes a 500 Internal Server Error response.
func InternalError(w http.ResponseWriter, message string, logger *slog.Logger) {
	Error(w, http.StatusInternalServerError, message, logger)
}

// HandleError writes an appropriate HTTP response based on the error type.
// Store errors are mapped to their HTTP codes, unknown errors become 500.
func HandleError(w http.ResponseWriter, err error, logger *slog.Logger) {
	var storeErr *store.Error
	if errors.As(err, &storeErr) {
		Error(w, storeErr.HTTPCode(), storeErr.Message, logger)
		return
	}

	// Unknown error = 500
	if logger != nil {
		logger.Error("Unhandled error", "error", err)
	}
	InternalError(w, "internal server error", logger)
}
