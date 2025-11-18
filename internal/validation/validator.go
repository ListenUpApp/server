package validation

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"

	"github.com/go-playground/validator/v10"
	"github.com/listenupapp/listenup-server/internal/store"
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

// formatError converts validator errors to store.Error.
func (v *Validator) formatError(err error) error {
	var validationErrs validator.ValidationErrors
	if !errors.As(err, &validationErrs) {
		return err
	}

	// Return first validation error
	for _, e := range validationErrs {
		msg := fmt.Sprintf("%s: %s", e.Field(), v.friendlyMessage(e))
		return &store.Error{
			Code:    http.StatusBadRequest,
			Message: msg,
			Err:     err,
		}
	}

	return err
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
	default:
		return "is invalid"
	}
}
