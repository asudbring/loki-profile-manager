package app

import "fmt"

// UserError marks an error safe to show to users. Secret-bearing lower-level
// errors should be redacted before becoming UserError values.
type UserError struct {
	Message string
}

func (e UserError) Error() string { return e.Message }

func NewUserError(format string, args ...any) error {
	return UserError{Message: fmt.Sprintf(format, args...)}
}
