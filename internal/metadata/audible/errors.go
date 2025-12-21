package audible

import (
	"errors"
	"fmt"
)

// Sentinel errors for Audible API operations.
var (
	ErrNotFound    = errors.New("audible: not found")
	ErrRateLimited = errors.New("audible: rate limited by server")
	ErrBadRequest  = errors.New("audible: bad request")
	ErrServer      = errors.New("audible: server error")
	ErrInvalidASIN = errors.New("audible: invalid ASIN format")
)

// Error wraps an underlying error with operation context.
type Error struct {
	Op     string // Operation: "search", "getBook", "getChapters"
	Region Region
	ASIN   string // If applicable
	Err    error
}

func (e *Error) Error() string {
	if e.ASIN != "" {
		return fmt.Sprintf("audible %s [%s/%s]: %v", e.Op, e.Region, e.ASIN, e.Err)
	}
	return fmt.Sprintf("audible %s [%s]: %v", e.Op, e.Region, e.Err)
}

func (e *Error) Unwrap() error {
	return e.Err
}

// wrapError creates an Error with context.
func wrapError(op string, region Region, asin string, err error) error {
	return &Error{
		Op:     op,
		Region: region,
		ASIN:   asin,
		Err:    err,
	}
}
