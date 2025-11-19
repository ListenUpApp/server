package validation_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/listenupapp/listenup-server/internal/store"
	"github.com/listenupapp/listenup-server/internal/validation"
	"github.com/stretchr/testify/assert"
)

type TestRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8,max=1024"`
	Name     string `json:"name" validate:"required"`
}

func TestValidator_ValidateSuccess(t *testing.T) {
	v := validation.New()

	req := TestRequest{
		Email:    "test@example.com",
		Password: "password123",
		Name:     "Test User",
	}

	err := v.Validate(req)
	assert.NoError(t, err)
}

func TestValidator_ValidateErrors(t *testing.T) {
	v := validation.New()

	//nolint:govet // fieldalignment: Minor memory optimization not worth the complexity in test code
	tests := []struct {
		name        string
		req         TestRequest
		wantErrCode int
		wantErrMsg  string
	}{
		{
			name: "missing required field",
			req: TestRequest{
				Email:    "test@example.com",
				Password: "password123",
				Name:     "", // Missing
			},
			wantErrCode: http.StatusBadRequest,
			wantErrMsg:  "name",
		},
		{
			name: "invalid email",
			req: TestRequest{
				Email:    "not-an-email",
				Password: "password123",
				Name:     "Test",
			},
			wantErrCode: http.StatusBadRequest,
			wantErrMsg:  "email",
		},
		{
			name: "password too short",
			req: TestRequest{
				Email:    "test@example.com",
				Password: "short",
				Name:     "Test",
			},
			wantErrCode: http.StatusBadRequest,
			wantErrMsg:  "password",
		},
		{
			name: "password too long",
			req: TestRequest{
				Email:    "test@example.com",
				Password: string(make([]byte, 1025)),
				Name:     "Test",
			},
			wantErrCode: http.StatusBadRequest,
			wantErrMsg:  "password",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(tt.req)
			assert.Error(t, err)

			var storeErr *store.Error
			if assert.True(t, errors.As(err, &storeErr)) {
				assert.Equal(t, tt.wantErrCode, storeErr.HTTPCode())
				assert.Contains(t, storeErr.Message, tt.wantErrMsg)
			}
		})
	}
}

func TestValidator_JSONFieldNames(t *testing.T) {
	v := validation.New()

	req := TestRequest{
		Email:    "",
		Password: "password123",
		Name:     "Test",
	}

	err := v.Validate(req)
	assert.Error(t, err)

	// Should use JSON tag name "email", not struct field name "Email"
	assert.Contains(t, err.Error(), "email")
	assert.NotContains(t, err.Error(), "Email")
}
