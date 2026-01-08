// Package validation provides HTTP request validation utilities using the validator/v10 library.
package validation

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/go-playground/validator/v10"
	domainerrors "github.com/listenupapp/listenup-server/internal/errors"
)

// Validator wraps go-playground/validator with domain error conversion.
type Validator struct {
	v *validator.Validate
}

// New creates a validator configured for our domain.
func New() *Validator {
	v := validator.New()

	// Use JSON tag names in error messages
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := fld.Tag.Get("json")
		if name == "" {
			return fld.Name
		}
		// Remove options like omitempty, -
		for i := range len(name) {
			if name[i] == ',' {
				return name[:i]
			}
		}
		return name
	})

	return &Validator{v: v}
}

// Validate validates a struct and returns a domain error.
func (v *Validator) Validate(s any) error {
	if err := v.v.Struct(s); err != nil {
		return v.formatError(err)
	}
	return nil
}

// formatError converts validator errors to domain errors.
func (v *Validator) formatError(err error) error {
	var validationErrs validator.ValidationErrors
	if !errors.As(err, &validationErrs) {
		return err
	}

	// Collect all field errors
	fieldErrors := make(map[string]string)
	for _, e := range validationErrs {
		fieldErrors[e.Field()] = v.friendlyMessage(e)
	}

	// Return domain validation error with details
	return domainerrors.ValidationWithDetails("validation failed", fieldErrors)
}

//nolint:gocyclo // Switch statement covering validation tags is intentionally exhaustive.
func (v *Validator) friendlyMessage(e validator.FieldError) string {
	switch e.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be a valid email address"
	case "min":
		return fmt.Sprintf("must be at least %s characters", e.Param())
	case "max":
		return fmt.Sprintf("must not exceed %s characters", e.Param())
	case "len":
		return fmt.Sprintf("must be exactly %s characters", e.Param())
	case "url":
		return "must be a valid URL"
	case "uuid":
		return "must be a valid UUID"
	case "oneof":
		return "must be one of: " + e.Param()
	case "gte":
		return "must be greater than or equal to " + e.Param()
	case "lte":
		return "must be less than or equal to " + e.Param()
	case "gt":
		return "must be greater than " + e.Param()
	case "lt":
		return "must be less than " + e.Param()
	case "gtefield":
		return "must be greater than or equal to " + e.Param()
	case "gtfield":
		return "must be greater than " + e.Param()
	case "ltefield":
		return "must be less than or equal to " + e.Param()
	case "ltfield":
		return "must be less than " + e.Param()
	default:
		return "is invalid"
	}
}
