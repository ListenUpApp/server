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

// OptionalFieldRequest tests validation with omitempty
type OptionalFieldRequest struct {
	Title       *string `json:"title" validate:"omitempty,min=1,max=100"`
	Description *string `json:"description" validate:"omitempty,max=500"`
	URL         *string `json:"url" validate:"omitempty,url"`
}

func TestValidator_OptionalFields(t *testing.T) {
	v := validation.New()

	// Test 1: All nil should pass
	req := OptionalFieldRequest{}
	err := v.Validate(req)
	assert.NoError(t, err, "All nil fields should pass")

	// Test 2: Valid values should pass
	title := "Valid Title"
	url := "https://example.com"
	req = OptionalFieldRequest{
		Title: &title,
		URL:   &url,
	}
	err = v.Validate(req)
	assert.NoError(t, err, "Valid optional fields should pass")

	// Test 3: Invalid URL should fail
	invalidURL := "not-a-url"
	req = OptionalFieldRequest{
		URL: &invalidURL,
	}
	err = v.Validate(req)
	assert.Error(t, err, "Invalid URL should fail")

	var domainErr *domainerrors.Error
	if assert.True(t, errors.As(err, &domainErr)) {
		details, ok := domainErr.Details.(map[string]string)
		assert.True(t, ok)
		_, hasURL := details["url"]
		assert.True(t, hasURL, "Details should contain 'url' field")
	}

	// Test 4: Title too long should fail
	longTitle := string(make([]byte, 101))
	req = OptionalFieldRequest{
		Title: &longTitle,
	}
	err = v.Validate(req)
	assert.Error(t, err, "Title too long should fail")
}

// SliceRequest tests validation with slices and dive
type SliceRequest struct {
	Items []ItemInput `json:"items" validate:"required,min=1,max=10,dive"`
}

type ItemInput struct {
	Name string `json:"name" validate:"required,min=1,max=50"`
}

func TestValidator_SliceValidation(t *testing.T) {
	v := validation.New()

	// Test 1: Valid slice should pass
	req := SliceRequest{
		Items: []ItemInput{{Name: "Item 1"}, {Name: "Item 2"}},
	}
	err := v.Validate(req)
	assert.NoError(t, err, "Valid slice should pass")

	// Test 2: Empty slice should fail (min=1)
	req = SliceRequest{
		Items: []ItemInput{},
	}
	err = v.Validate(req)
	assert.Error(t, err, "Empty slice should fail")

	// Test 3: Too many items should fail (max=10)
	items := make([]ItemInput, 11)
	for i := range items {
		items[i] = ItemInput{Name: "Item"}
	}
	req = SliceRequest{
		Items: items,
	}
	err = v.Validate(req)
	assert.Error(t, err, "Too many items should fail")

	// Test 4: Invalid item in slice should fail (dive)
	req = SliceRequest{
		Items: []ItemInput{{Name: "Valid"}, {Name: ""}},
	}
	err = v.Validate(req)
	assert.Error(t, err, "Invalid item in slice should fail")
}

// LengthRequest tests exact length validation
type LengthRequest struct {
	ASIN string `json:"asin" validate:"omitempty,len=10"`
	Code string `json:"code" validate:"required,len=6"`
}

func TestValidator_LengthValidation(t *testing.T) {
	v := validation.New()

	// Test 1: Correct length should pass
	req := LengthRequest{
		ASIN: "B00ABC1234",
		Code: "ABC123",
	}
	err := v.Validate(req)
	assert.NoError(t, err, "Correct length should pass")

	// Test 2: Wrong length should fail
	req = LengthRequest{
		ASIN: "B00AB", // Too short
		Code: "ABC123",
	}
	err = v.Validate(req)
	assert.Error(t, err, "Wrong ASIN length should fail")

	// Test 3: Empty optional is OK
	req = LengthRequest{
		Code: "ABC123",
	}
	err = v.Validate(req)
	assert.NoError(t, err, "Empty optional with len should pass")
}

// NumericRangeRequest tests gte/lte validation
type NumericRangeRequest struct {
	Speed   float32 `json:"speed" validate:"gt=0,lte=4"`
	SkipSec int     `json:"skip_sec" validate:"gte=5,lte=300"`
}

func TestValidator_NumericRangeValidation(t *testing.T) {
	v := validation.New()

	// Test 1: Valid values should pass
	req := NumericRangeRequest{
		Speed:   1.5,
		SkipSec: 30,
	}
	err := v.Validate(req)
	assert.NoError(t, err, "Valid values should pass")

	// Test 2: Speed at boundary (lte=4) should pass
	req = NumericRangeRequest{
		Speed:   4.0,
		SkipSec: 30,
	}
	err = v.Validate(req)
	assert.NoError(t, err, "Speed at max boundary should pass")

	// Test 3: Speed above max should fail
	req = NumericRangeRequest{
		Speed:   4.1,
		SkipSec: 30,
	}
	err = v.Validate(req)
	assert.Error(t, err, "Speed above max should fail")

	// Test 4: Speed at zero should fail (gt=0)
	req = NumericRangeRequest{
		Speed:   0,
		SkipSec: 30,
	}
	err = v.Validate(req)
	assert.Error(t, err, "Speed at zero should fail")

	// Test 5: SkipSec below min should fail
	req = NumericRangeRequest{
		Speed:   1.0,
		SkipSec: 4,
	}
	err = v.Validate(req)
	assert.Error(t, err, "SkipSec below min should fail")
}

// OneOfRequest tests oneof validation
type OneOfRequest struct {
	Role string `json:"role" validate:"required,oneof=admin member"`
}

func TestValidator_OneOfValidation(t *testing.T) {
	v := validation.New()

	// Test 1: Valid value should pass
	req := OneOfRequest{Role: "admin"}
	err := v.Validate(req)
	assert.NoError(t, err, "Valid oneof value should pass")

	req = OneOfRequest{Role: "member"}
	err = v.Validate(req)
	assert.NoError(t, err, "Valid oneof value should pass")

	// Test 2: Invalid value should fail
	req = OneOfRequest{Role: "superadmin"}
	err = v.Validate(req)
	assert.Error(t, err, "Invalid oneof value should fail")

	var domainErr *domainerrors.Error
	if assert.True(t, errors.As(err, &domainErr)) {
		details, ok := domainErr.Details.(map[string]string)
		assert.True(t, ok)
		msg := details["role"]
		assert.Contains(t, msg, "must be one of", "Error message should mention valid options")
	}
}
