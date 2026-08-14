package api

import (
	"errors"
	"net/http"

	"aaa2ppp/teams-tasks/internal/model"
)

type httpError struct {
	Msg  string
	Code int
}

func (e *httpError) Error() string {
	return e.Msg
}

func mapError(err error) *httpError {
	if httpErr, ok := err.(*httpError); ok {
		return httpErr
	}
	var httpErr *httpError
	switch {
	case errors.As(err, &httpErr):
		return &httpError{err.Error(), httpErr.Code}
	case errors.Is(err, model.ErrValidation):
		return &httpError{err.Error(), http.StatusBadRequest}
	case errors.Is(err, model.ErrNotFound):
		return &httpError{err.Error(), http.StatusNotFound}
	case errors.Is(err, model.ErrConflict):
		return &httpError{err.Error(), http.StatusConflict}
	case errors.Is(err, model.ErrInternal):
		return &httpError{err.Error(), http.StatusInternalServerError}
	case errors.Is(err, model.ErrNotImplemented):
		return &httpError{err.Error(), http.StatusNotImplemented}
	}
	return nil
}
