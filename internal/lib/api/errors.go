package api

import (
	"errors"
	"net/http"

	"aaa2ppp/teams-tasks/internal/model"
)

type HttpError struct {
	Msg  string
	Code int
}

func (e *HttpError) Error() string {
	return e.Msg
}

func mapError(err error) *HttpError {
	if httpErr, ok := err.(*HttpError); ok {
		return httpErr
	}
	var httpErr *HttpError
	switch {
	case errors.As(err, &httpErr):
		return &HttpError{err.Error(), httpErr.Code}
	case errors.Is(err, model.ErrValidation):
		return &HttpError{err.Error(), http.StatusBadRequest}
	case errors.Is(err, model.ErrNotFound):
		return &HttpError{err.Error(), http.StatusNotFound}
	case errors.Is(err, model.ErrConflict):
		return &HttpError{err.Error(), http.StatusConflict}
	case errors.Is(err, model.ErrInternal):
		return &HttpError{err.Error(), http.StatusInternalServerError}
	case errors.Is(err, model.ErrUnmplemented):
		return &HttpError{err.Error(), http.StatusNotImplemented}
	case errors.Is(err, model.ErrForbidden):
		return &HttpError{err.Error(), http.StatusForbidden}
	}
	return nil
}
