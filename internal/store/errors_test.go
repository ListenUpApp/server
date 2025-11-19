package store_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/listenupapp/listenup-server/internal/store"
	"github.com/stretchr/testify/assert"
)

func TestError_Error(t *testing.T) {
	err := &store.Error{
		Code:    http.StatusNotFound,
		Message: "not found",
	}

	assert.Equal(t, "not found", err.Error())
}

func TestError_ErrorWithCause(t *testing.T) {
	cause := errors.New("underlying error")
	err := &store.Error{
		Code:    http.StatusNotFound,
		Message: "not found",
		Err:     cause,
	}

	assert.Contains(t, err.Error(), "not found")
	assert.Contains(t, err.Error(), "underlying error")
}

func TestError_HTTPCode(t *testing.T) {
	err := &store.Error{
		Code:    http.StatusBadRequest,
		Message: "bad request",
	}

	assert.Equal(t, http.StatusBadRequest, err.HTTPCode())
}

func TestError_Unwrap(t *testing.T) {
	cause := errors.New("underlying")
	err := &store.Error{
		Code:    http.StatusInternalServerError,
		Message: "error",
		Err:     cause,
	}

	assert.Equal(t, cause, err.Unwrap())
}

func TestError_WithMessage(t *testing.T) {
	original := &store.Error{
		Code:    http.StatusNotFound,
		Message: "original",
	}

	modified := original.WithMessage("custom message")

	assert.Equal(t, http.StatusNotFound, modified.Code)
	assert.Equal(t, "custom message", modified.Message)
}

func TestError_WithCause(t *testing.T) {
	original := &store.Error{
		Code:    http.StatusNotFound,
		Message: "not found",
	}

	cause := errors.New("db error")
	modified := original.WithCause(cause)

	assert.Equal(t, http.StatusNotFound, modified.Code)
	assert.Equal(t, "not found", modified.Message)
	assert.Equal(t, cause, modified.Err)
}

func TestSentinelErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      *store.Error
		wantCode int
	}{
		{
			name:     "not found",
			err:      store.ErrNotFound,
			wantCode: http.StatusNotFound,
		},
		{
			name:     "already exists",
			err:      store.ErrAlreadyExists,
			wantCode: http.StatusConflict,
		},
		{
			name:     "invalid input",
			err:      store.ErrInvalidInput,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "unauthorized",
			err:      store.ErrUnauthorized,
			wantCode: http.StatusUnauthorized,
		},
		{
			name:     "forbidden",
			err:      store.ErrForbidden,
			wantCode: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantCode, tt.err.HTTPCode())
			assert.NotEmpty(t, tt.err.Message)
		})
	}
}
