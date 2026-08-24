package sign

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"aaa2ppp/teams-tasks/internal/lib/api"
	"aaa2ppp/teams-tasks/internal/model"
)

type LoginReq struct {
	Email    string `json:"email" validate:"required" format:"email"`
	Password string `json:"password,omitempty" validate:"required"`
}

func (req *LoginReq) Validate() error {
	var errs []error

	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" {
		errs = append(errs, errors.New("email is required"))
	}

	req.Password = strings.TrimSpace(req.Password)
	if req.Password == "" {
		errs = append(errs, errors.New("password is required"))
	}

	return errors.Join(errs...)
}

type LoginResp struct {
	User  model.User `json:"user"`
	Token string     `json:"token,omitempty"`
}

type RegisterReq struct {
	Email    string `json:"email" validate:"required" format:"email"`
	Name     string `json:"name"`
	Password string `json:"password" validate:"required" example:"paS$w0rd"`
}

func (req *RegisterReq) Validate() error {
	var errs []error

	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" {
		errs = append(errs, errors.New("email is required"))
	} else if !isValidEmail(req.Email) {
		errs = append(errs, errors.New("invalid email"))
	}

	req.Password = strings.TrimSpace(req.Password)
	if req.Password == "" {
		errs = append(errs, errors.New("password is required"))
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		errs = append(errs, errors.New("name is required"))
	}

	return errors.Join(errs...)
}

func isValidEmail(email string) bool {
	parsed, err := mail.ParseAddress(email)
	if err != nil {
		return false
	}
	return parsed.Address == email
}

type Service interface {
	Login(ctx context.Context, req LoginReq) (LoginResp, error)
	Register(ctx context.Context, req RegisterReq) (LoginResp, error)
}

func NewAPI(svc Service) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("POST /login", login(svc))       // аутентификация, выдача JWT.
	mux.Handle("POST /register", register(svc)) // регистрация пользователя
	return mux
}

// login godoc
//
//	@tags		sign
//	@router		/login [post]
//	@summary	Аутентификация пользователя
//	@accept		json
//	@param		req	body		LoginReq	true	"LoginReq"
//	@success	200	{object}	LoginResp
//	@failure	401
func login(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := api.NewHelper(w, r, "auth.login")

		var req LoginReq
		if err := h.DecodeBody(&req); err != nil {
			h.Log().Debug("invalid login request", "error", err)
			h.WriteError(&api.HttpError{Code: 401, Msg: "unauthorized"})
			return
		}

		resp, err := svc.Login(h.Ctx(), req)
		if err != nil {
			h.Log().Debug("authorization failed", "error", err)
			h.WriteError(&api.HttpError{Code: 401, Msg: "unauthorized"})
			return
		}

		h.WriteResponse(200, resp)
	}
}

// register godoc
//
//	@tags		sign
//	@router		/register [post]
//	@summary	Регистрация пользователя
//	@accept		json
//	@produce	json
//	@param		req	body		RegisterReq	true	"RegisterReq"
//	@success	201	{object}	LoginResp
//	@failure	400
func register(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := api.NewHelper(w, r, "auth.register")

		var req RegisterReq
		if err := h.DecodeBody(&req); err != nil {
			h.WriteError(err)
			return
		}

		resp, err := svc.Register(h.Ctx(), req)
		if err != nil {
			h.WriteError(err)
			return
		}

		h.WriteResponse(201, resp)
	}
}
