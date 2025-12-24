package validation_test

import (
	"errors"
	"net/http"
	"testing"

	domainerrors "github.com/listenupapp/listenup-server/internal/errors"
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

			var domainErr *domainerrors.Error
			if assert.True(t, errors.As(err, &domainErr)) {
				assert.Equal(t, tt.wantErrCode, domainErr.Code.HTTPStatus())
				// Field errors are in the Details map
				details, ok := domainErr.Details.(map[string]string)
				assert.True(t, ok, "Details should be map[string]string")
				_, hasField := details[tt.wantErrMsg]
				assert.True(t, hasField, "Details should contain field %q", tt.wantErrMsg)
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
	var domainErr *domainerrors.Error
	if assert.True(t, errors.As(err, &domainErr)) {
		details, ok := domainErr.Details.(map[string]string)
		assert.True(t, ok, "Details should be map[string]string")
		_, hasEmail := details["email"]
		assert.True(t, hasEmail, "Details should contain 'email' field (lowercase)")
		_, hasEmailUpper := details["Email"]
		assert.False(t, hasEmailUpper, "Details should not contain 'Email' field (uppercase)")
	}
}
