package model

import "errors"

var (
	ErrValidation     = errors.New("validation error")
	ErrNotFound       = errors.New("not found")
	ErrConflict       = errors.New("conflict")
	ErrInternal       = errors.New("internal error")
	ErrNotImplemented = errors.New("not implemented")
)
