package teams

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"aaa2ppp/teams-tasks/internal/lib/api"
	"aaa2ppp/teams-tasks/internal/model"
)

type SvcCreateReq struct {
	Name string
}

type SvcListReq struct {
	WithMembers bool
	WithTasks   bool
}

type SvcAddMemberReq struct {
	TeamID     model.TeamID
	UserID     model.UserID
	UserEmail  string
	MemberRole model.Role
}

type Service interface {
	Create(ctx context.Context, req SvcCreateReq) (model.Team, error)
	List(ctx context.Context, req SvcListReq) ([]model.Team, error)
	AddMember(ctx context.Context, req SvcAddMemberReq) error
	GenReport(ctx context.Context, teamID model.TeamID) ([]Metric, error)
}

func NewAPI(svc Service) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("POST /teams/", create(svc))                  // создание команды, текущий пользователь становится создателем.
	mux.Handle("GET  /teams/", list(svc))                    // список команд текущего пользователя.
	mux.Handle("POST /teams/{id}/invite", invite(svc))       // добавление пользователя в команду, только создатель или администратор.
	mux.Handle("GET  /teams/{id}/stats", getStatistics(svc)) // SQL-отчет по конкретной команде.
	return mux
}

// create godoc
//
//	@tags		teams
//	@router		/teams [post]
//	@summary	Cоздание команды
//	@accept		json
//	@produce	json
//	@param		X-Authtoken	header		string			true	" "
//	@param		req			body		apiCreateReq	true	" "
//	@success	201			{object}	model.Team
//	@failure	400
func create(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := api.NewHelper(w, r, "teams.create")

		var req apiCreateReq
		if err := h.DecodeBody(&req); err != nil {
			h.WriteError(err)
			return
		}

		team, err := svc.Create(h.Ctx(), SvcCreateReq(req))
		if err != nil {
			h.WriteError(err)
			return
		}

		h.WriteResponse(201, team)
	}
}

type apiCreateReq struct {
	Name string `json:"name" validate:"required"`
}

func (req *apiCreateReq) Validate() error {
	var errs []error

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		errs = append(errs, errors.New("name is required"))
	}

	return errors.Join(errs...)
}

// list godoc
//
//	@tags		teams
//	@router		/teams [get]
//	@summary	Список команд
//	@produce	json
//	@param		X-Authtoken		header	string	true	" "
//	@param		with_members	query	bool	fasle	" "	default(false)
//	@param		with_tasks		query	bool	fasle	" "	default(false)
//	@success	200				{array}	model.Team
//	@failure	400
func list(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := api.NewHelper(w, r, "teams.list")

		req, err := parseListRequest(r.URL.Query())
		if err != nil {
			h.WriteError(&api.HttpError{Code: 400, Msg: err.Error()})
			return
		}

		teams, err := svc.List(h.Ctx(), req)
		if err != nil {
			h.WriteError(err)
			return
		}

		h.WriteResponse(200, teams)
	}
}

func parseListRequest(q url.Values) (SvcListReq, error) {
	var req SvcListReq
	var errs []error

	if q.Has("with_members") {
		s := q.Get("with_members")
		switch s {
		case "true":
			req.WithMembers = true
		case "false":
			req.WithMembers = false
		default:
			errs = append(errs, errors.New("with_members must be true or false"))
		}
	}

	if q.Has("with_tasks") {
		s := q.Get("with_tasks")
		switch s {
		case "true":
			req.WithTasks = true
		case "false":
			req.WithTasks = false
		default:
			errs = append(errs, errors.New("with_tasks must be true or false"))
		}
	}

	return req, errors.Join(errs...)
}

// invite godoc
//
//	@tags			teams
//	@router			/teams/{id}/invite [post]
//	@summary		Добавление пользователя в команду
//	@description	One of the parameters must be specified: `user_id` or `user_email`.
//	@accept			json
//	@produce		json
//	@param			X-Authtoken	header	string			true	" "
//	@param			id			path	int				true	" "	minimum(1)	extensions(x-example=42)
//	@param			req			body	apiInviteReq	true	" "
//	@success		200
//	@failure		400
func invite(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := api.NewHelper(w, r, "teams.invite")

		teamID, err := h.GetIDFromPath()
		if err != nil {
			h.WriteError(err)
			return
		}

		var req apiInviteReq
		req.MemberRole = model.RoleMember

		if err := h.DecodeBody(&req); err != nil {
			h.WriteError(err)
			return
		}

		if err := svc.AddMember(h.Ctx(), SvcAddMemberReq{
			TeamID:     model.TeamID(teamID),
			UserID:     req.UserID,
			UserEmail:  req.UserEmail,
			MemberRole: req.MemberRole,
		}); err != nil {
			h.WriteError(err)
			return
		}
	}
}

type apiInviteReq struct {
	UserID     model.UserID `json:"user_id" minimum:"1"`
	UserEmail  string       `json:"user_email" format:"email"`
	MemberRole model.Role   `json:"member_role" swaggertype:"string" enums:"admin,member" default:"member"`
}

func (req *apiInviteReq) Validate() error {
	var errs []error

	req.UserEmail = strings.TrimSpace(req.UserEmail)

	if req.UserID == 0 && req.UserEmail == "" {
		errs = append(errs, errors.New("user_id or user_email is required"))
	}

	if req.UserID < 0 {
		errs = append(errs, errors.New("invalid user_id"))
	}

	possible := []model.Role{model.RoleAdmin, model.RoleMember}
	if !slices.Contains(possible, req.MemberRole) {
		errs = append(errs, errors.New("invalid member_role"))
	}

	return errors.Join(errs...)
}

// ------ SQL отчет ------

// list godoc
//
//	@tags		teams
//	@router		/teams/{id}/stats [get]
//	@summary	Отчет по конкретной команде
//	@produce	json
//	@param		X-Authtoken	header	string	true	" "
//	@param		id			path	int		true	" "	minimum(1)	extensions(x-example=42)
//	@success	200			{array}	Metric
//	@failure	400
func getStatistics(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := api.NewHelper(w, r, "teams.getStatistics")

		teamID, err := h.GetIDFromPath()
		if err != nil {
			h.WriteError(err)
			return
		}

		metrics, err := svc.GenReport(h.Ctx(), model.TeamID(teamID))
		if err != nil {
			h.WriteError(err)
			return
		}

		h.WriteResponse(200, metrics)
	}
}
