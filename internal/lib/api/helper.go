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
	"aaa2ppp/teams-tasks/internal/model"
)

const applicationJSON = "application/json"

type Helper struct {
	w   http.ResponseWriter
	r   *http.Request
	op  string
	ctx context.Context
	log *slog.Logger
}

var _ Helper

func NewHelper(w http.ResponseWriter, r *http.Request, op string) *Helper {
	return &Helper{
		w:  w,
		r:  r,
		op: op,
	}
}

func (h *Helper) Ctx() context.Context {
	if h.ctx == nil {
		h.ctx = h.r.Context()
	}
	return h.ctx
}

func (h *Helper) Log() *slog.Logger {
	if h.log == nil {
		h.log = logging.GetLogger(h.Ctx()).With("op", h.op)
	}
	return h.log
}

func (h *Helper) WriteError(err error) {
	if err == nil { // хм?..
		h.Log().Error("writeError called with nil error")
		h.writeHTTPError(&HttpError{"internal error", http.StatusInternalServerError})
		return
	}
	if httpErr := mapError(err); httpErr != nil {
		h.writeHTTPError(httpErr)
		return
	}
	h.Log().Warn("unmapped error", "type", reflect.TypeOf(err).String(), "error", err)
	h.writeHTTPError(&HttpError{"internal error", http.StatusInternalServerError})
}

func (h *Helper) writeHTTPError(err *HttpError) {
	http.Error(h.w, strconv.Itoa(err.Code)+" "+err.Msg, err.Code)
}

func (h *Helper) CheckContentType(wantType string) error {
	ct, _, _ := mime.ParseMediaType(h.r.Header.Get("Content-Type"))
	if ct != wantType {
		return &HttpError{"want " + wantType, http.StatusBadRequest}
	}
	return nil
}

func (h *Helper) DecodeBody(req any) error {
	if err := h.CheckContentType(applicationJSON); err != nil {
		return err
	}

	d := json.NewDecoder(h.r.Body)
	d.UseNumber() // удобно, если декодируем в мапу

	if err := d.Decode(req); err != nil {
		return &HttpError{"bad json: " + err.Error(), http.StatusBadRequest}
	}

	if req, ok := req.(Validator); ok {
		if err := req.Validate(); err != nil {
			return &HttpError{err.Error(), http.StatusBadRequest}
		}
	}

	return nil
}

type Validator interface {
	Validate() error
}

func (h *Helper) WriteResponse(statusCode int, resp any) {
	h.w.Header().Set("Content-Type", applicationJSON)
	h.w.WriteHeader(statusCode)

	if err := json.NewEncoder(h.w).Encode(resp); err != nil {
		h.Log().Error("write response", "error", err)
	}
}

func (h *Helper) GetIDFromPath() (model.ID, error) {
	s := h.r.PathValue("id")
	if s == "" {
		return 0, &HttpError{"id cannot be empty", http.StatusBadRequest}
	}
	id, err := model.ParseID(s)
	if err != nil || id <= 0 {
		return 0, &HttpError{err.Error(), http.StatusBadRequest}
	}
	return id, nil
}
