package model

import "errors"

var (
	ErrValidation     = errors.New("validation error")
	ErrUnauthorized   = errors.New("unauthorized")
	ErrForbidden      = errors.New("forbidden")
	ErrNotFound       = errors.New("not found")
	ErrConflict       = errors.New("conflict")
	ErrNoRowsAffected = errors.New("no rows affected")
	ErrInternal       = errors.New("internal error")
	ErrUnmplemented   = errors.New("unimplemented")
)
