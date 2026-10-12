// Package apperr defines domain-level sentinel errors shared by all modules.
// Wrap them with fmt.Errorf("...%w...") and classify with errors.Is.
package apperr

import "errors"

var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("already exists")
	ErrUnauthorized = errors.New("unauthorized")
	ErrValidation   = errors.New("invalid input")
)
