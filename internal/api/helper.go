package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"mime"
	"net/http"
	"reflect"
	"strconv"

	"aaa2ppp/teams-tasks/internal/lib/logging"
)

const applicationJSON = "application/json"

type helper struct {
	w    http.ResponseWriter
	r    *http.Request
	op   string
	_ctx context.Context
	_log *slog.Logger
}

var _ helper

func newHelper(w http.ResponseWriter, r *http.Request, op string) *helper { //nolint:unused
	return &helper{
		w:  w,
		r:  r,
		op: op,
	}
}

func (h *helper) ctx() context.Context {
	if h._ctx == nil {
		h._ctx = h.r.Context()
	}
	return h._ctx
}

func (h *helper) log() *slog.Logger {
	if h._log == nil {
		h._log = logging.FromContext(h.ctx()).With("op", h.op)
	}
	return h._log
}

func (h *helper) writeError(err error) {
	if err == nil { // хм?..
		h.log().Error("writeError called with nil error")
		h.writeHTTPError(&httpError{"internal error", http.StatusInternalServerError})
		return
	}
	if httpErr := mapError(err); httpErr != nil {
		h.writeHTTPError(httpErr)
		return
	}
	h.log().Warn("unmapped error", "type", reflect.TypeOf(err).String(), "error", err)
	h.writeHTTPError(&httpError{"internal error", http.StatusInternalServerError})
}

func (h *helper) writeHTTPError(err *httpError) {
	http.Error(h.w, strconv.Itoa(err.Code)+" "+err.Msg, err.Code)
}

func (h *helper) checkContentType(wantType string) error {
	ct, _, _ := mime.ParseMediaType(h.r.Header.Get("Content-Type"))
	if ct != wantType {
		return &httpError{"want " + wantType, http.StatusBadRequest}
	}
	return nil
}

func (h *helper) decodeRequestBody(req any) error {
	if err := h.checkContentType(applicationJSON); err != nil {
		return err
	}

	d := json.NewDecoder(h.r.Body)
	d.UseNumber() // удобно, если декодируем в мапу

	if err := d.Decode(req); err != nil {
		return &httpError{"bad json: " + err.Error(), http.StatusBadRequest}
	}

	if req, ok := req.(validator); ok {
		if err := req.Validate(); err != nil {
			return &httpError{err.Error(), http.StatusBadRequest}
		}
	}

	return nil
}

type validator interface {
	Validate() error
}

func (h *helper) writeResponse(statusCode int, resp any) {
	h.w.Header().Set("Content-Type", applicationJSON)
	h.w.WriteHeader(statusCode)

	if err := json.NewEncoder(h.w).Encode(resp); err != nil {
		h.log().Error("write response", "error", err)
	}
}

func (h *helper) getIDFromPath() (int64, error) {
	s := h.r.PathValue("id")
	if s == "" {
		return 0, &httpError{"id cannot be empty", http.StatusBadRequest}
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return 0, &httpError{"id must be int >0", http.StatusBadRequest}
	}
	return id, nil
}
