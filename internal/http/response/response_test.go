package response

import (
	"encoding/json/v2"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvelope_Marshal(t *testing.T) {
	envelope := Envelope{
		Success: true,
		Data:    map[string]string{"key": "value"},
		Error:   "",
		Message: "test message",
	}

	data, err := json.Marshal(envelope)
	require.NoError(t, err)

	var decoded Envelope
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.True(t, decoded.Success)
	assert.NotNil(t, decoded.Data)
	assert.Equal(t, "test message", decoded.Message)
}

func TestJSON_Success(t *testing.T) {
	w := httptest.NewRecorder()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	data := map[string]string{"message": "test"}
	JSON(w, http.StatusOK, data, logger)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))

	var result Envelope
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.True(t, result.Success)
	assert.NotNil(t, result.Data)
	assert.Empty(t, result.Error)
}

func TestJSON_Error(t *testing.T) {
	w := httptest.NewRecorder()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	data := map[string]string{"message": "test"}
	JSON(w, http.StatusNotFound, data, logger)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))

	var result Envelope
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.False(t, result.Success, "Success should be false for status >= 400")
	assert.NotNil(t, result.Data)
}

func TestJSON_NilLogger(t *testing.T) {
	w := httptest.NewRecorder()

	data := map[string]string{"message": "test"}
	JSON(w, http.StatusOK, data, nil)

	assert.Equal(t, http.StatusOK, w.Code)

	var result Envelope
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestSuccess(t *testing.T) {
	w := httptest.NewRecorder()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	data := map[string]any{
		"id":   "123",
		"name": "test",
	}

	Success(w, data, logger)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))

	var result Envelope
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.True(t, result.Success)
	assert.NotNil(t, result.Data)

	dataMap, ok := result.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "123", dataMap["id"])
	assert.Equal(t, "test", dataMap["name"])
}

func TestCreated(t *testing.T) {
	w := httptest.NewRecorder()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	data := map[string]string{"id": "new-id"}
	Created(w, data, logger)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))

	var result Envelope
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.True(t, result.Success)
	assert.NotNil(t, result.Data)
}

func TestNoContent(t *testing.T) {
	w := httptest.NewRecorder()

	NoContent(w)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.String())
}

func TestError_Generic(t *testing.T) {
	w := httptest.NewRecorder()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	Error(w, http.StatusInternalServerError, "something went wrong", logger)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))

	var result Envelope
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.Nil(t, result.Data)
	assert.Equal(t, "something went wrong", result.Error)
}

func TestError_NilLogger(t *testing.T) {
	w := httptest.NewRecorder()

	Error(w, http.StatusBadRequest, "bad request", nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result Envelope
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.Equal(t, "bad request", result.Error)
}

func TestBadRequest(t *testing.T) {
	w := httptest.NewRecorder()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	BadRequest(w, "invalid input", logger)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result Envelope
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.Equal(t, "invalid input", result.Error)
}

func TestUnauthorized(t *testing.T) {
	w := httptest.NewRecorder()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	Unauthorized(w, "authentication required", logger)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var result Envelope
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.Equal(t, "authentication required", result.Error)
}

func TestForbidden(t *testing.T) {
	w := httptest.NewRecorder()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	Forbidden(w, "access denied", logger)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var result Envelope
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.Equal(t, "access denied", result.Error)
}

func TestNotFound(t *testing.T) {
	w := httptest.NewRecorder()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	NotFound(w, "resource not found", logger)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var result Envelope
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.Equal(t, "resource not found", result.Error)
}

func TestInternalError(t *testing.T) {
	w := httptest.NewRecorder()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	InternalError(w, "internal server error", logger)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var result Envelope
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.Equal(t, "internal server error", result.Error)
}

func TestStatusCodeBoundary(t *testing.T) {
	tests := []struct {
		name            string
		status          int
		expectedSuccess bool
	}{
		{"100 Continue", 100, true},
		{"200 OK", 200, true},
		{"201 Created", 201, true},
		{"204 No Content", 204, true},
		{"301 Moved Permanently", 301, true},
		{"399 Custom Success", 399, true},
		{"400 Bad Request", 400, false},
		{"401 Unauthorized", 401, false},
		{"404 Not Found", 404, false},
		{"500 Internal Server Error", 500, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))

			JSON(w, tt.status, nil, logger)

			var result Envelope
			err := json.Unmarshal(w.Body.Bytes(), &result)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedSuccess, result.Success, "Status %d should have Success=%v", tt.status, tt.expectedSuccess)
		})
	}
}

func TestEnvelope_OmitEmpty(t *testing.T) {
	tests := []struct {
		name        string
		envelope    Envelope
		contains    []string
		notContains []string
	}{
		{
			name: "success with data",
			envelope: Envelope{
				Success: true,
				Data:    "test",
			},
			contains:    []string{"\"success\":true", "\"data\":\"test\""},
			notContains: []string{"\"error\":", "\"message\":"},
		},
		{
			name: "error without data",
			envelope: Envelope{
				Success: false,
				Error:   "something failed",
			},
			contains:    []string{"\"success\":false", "\"error\":\"something failed\""},
			notContains: []string{"\"data\":"},
		},
		{
			name: "with message",
			envelope: Envelope{
				Success: true,
				Message: "operation completed",
			},
			contains: []string{"\"success\":true", "\"message\":\"operation completed\""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.envelope)
			require.NoError(t, err)

			jsonStr := string(data)
			for _, s := range tt.contains {
				assert.Contains(t, jsonStr, s)
			}
			for _, s := range tt.notContains {
				assert.NotContains(t, jsonStr, s)
			}
		})
	}
}
