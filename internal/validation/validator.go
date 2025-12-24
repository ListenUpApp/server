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
		for i := 0; i < len(name); i++ {
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
		return fmt.Sprintf("must be one of: %s", e.Param())
	case "gte":
		return fmt.Sprintf("must be greater than or equal to %s", e.Param())
	case "lte":
		return fmt.Sprintf("must be less than or equal to %s", e.Param())
	case "gt":
		return fmt.Sprintf("must be greater than %s", e.Param())
	case "lt":
		return fmt.Sprintf("must be less than %s", e.Param())
	case "gtefield":
		return fmt.Sprintf("must be greater than or equal to %s", e.Param())
	case "gtfield":
		return fmt.Sprintf("must be greater than %s", e.Param())
	case "ltefield":
		return fmt.Sprintf("must be less than or equal to %s", e.Param())
	case "ltfield":
		return fmt.Sprintf("must be less than %s", e.Param())
	default:
		return "is invalid"
	}
}
