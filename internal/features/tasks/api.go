package tasks

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"aaa2ppp/teams-tasks/internal/lib/api"
	"aaa2ppp/teams-tasks/internal/model"
)

type Nullable[T any] = model.Nullable[T]

type SvcCreateReq struct {
	TeamID      model.TeamID
	Title       string
	Description string
	Status      model.Status
	AssigneeID  Nullable[model.UserID]
}

type SvcGetReq struct {
	TaskID       model.TaskID
	WithComments bool
	WithHistory  bool
}

type SvcListReq struct {
	TeamID       model.TeamID
	Status       model.Status
	AssigneeID   Nullable[model.UserID]
	WithComments bool
	WithHistory  bool
	Cursor       model.TaskID
	Limit        int
}

type SvcListResp struct {
	Tasks      []model.Task `json:"tasks,omitempty"`
	NextCursor model.TaskID `json:"next_cursor,omitempty"`
}

type SvcUpdateReq struct {
	TaskID      model.TaskID
	Title       string
	Description string
	Status      model.Status
	AssigneeID  Nullable[model.UserID]
	Version     int64
}

type SvcAddCommentReq struct {
	TaskID  model.TaskID
	Content string
}

type Service interface {
	Create(ctx context.Context, req SvcCreateReq) (model.Task, error)
	List(ctx context.Context, req SvcListReq) (SvcListResp, error)
	Update(ctx context.Context, req SvcUpdateReq) (model.Task, error)
	AddComment(ctx context.Context, req SvcAddCommentReq) (model.TaskComment, error)
	Get(ctx context.Context, req SvcGetReq) (model.Task, error)
}

func NewAPI(svc Service) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("POST /tasks/", create(svc))                       // создание задачи в команде.
	mux.Handle("GET  /tasks/", list(svc))                         // список задач команды с фильтрами и пагинацией.
	mux.Handle("PUT  /tasks/{id}", update(svc))                   // обновление задачи с проверкой прав и записью истории.
	mux.Handle("GET  /tasks/{id}/history", getWithHistory(svc))   // история изменений задачи.
	mux.Handle("POST /tasks/{id}/comments", addComment(svc))      // добавление комментария к задаче.
	mux.Handle("GET  /tasks/{id}/comments", getWithComments(svc)) // список комментариев задачи.
	return mux
}

// create godoc
//
//	@tags		tasks
//	@router		/tasks [post]
//	@summary	Создание задачи
//	@accept		json
//	@produce	json
//	@param		X-Authtoken	header		string			true	" "
//	@param		req			body		apiCreateReq	true	" "
//	@success	201			{object}	model.Task
//	@failure	400
func create(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := api.NewHelper(w, r, "tasks.create")

		var req apiCreateReq
		req.Status = model.StatusTodo

		if err := h.DecodeBody(&req); err != nil {
			h.WriteError(err)
			return
		}

		task, err := svc.Create(h.Ctx(), SvcCreateReq(req))
		if err != nil {
			h.WriteError(err)
			return
		}

		h.WriteResponse(201, task)
	}
}

type apiCreateReq struct {
	TeamID      model.TeamID           `json:"team_id,omitempty" validate:"required" minimum:"1"`
	Title       string                 `json:"title,omitempty" validate:"required"`
	Description string                 `json:"description,omitempty"`
	Status      model.Status           `json:"status,omitempty" validate:"required" swaggertype:"string" enums:"todo,in_progress,done,cancelled" default:"todo"`
	AssigneeID  Nullable[model.UserID] `json:"assignee_id,omitzero" swaggertype:"integer" minimum:"1"`
}

func (req *apiCreateReq) Validate() error {
	var errs []error

	if req.TeamID <= 0 {
		errs = append(errs, errors.New("team_id must be > 0"))
	}

	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		errs = append(errs, errors.New("title is required"))
	}

	req.Description = strings.TrimSpace(req.Description)

	if req.Status == model.StatusUnknown {
		errs = append(errs, errors.New("status is required"))
	}

	if req.AssigneeID.Valid && req.AssigneeID.V <= 0 {
		errs = append(errs, errors.New("assignee_id must be > 0"))
	}

	return errors.Join(errs...)
}

// list godoc
//
//	@tags		tasks
//	@router		/tasks [get]
//	@summary	Список задач
//	@produce	json
//	@param		X-Authtoken		header		string	true	" "
//	@param		team_id			query		int		true	" "																	minimum(1)	extensions(x-example=1)
//	@param		status			query		string	false	" "																	enums(todo,in_progress,done,cancelled)
//	@param		assignee_id		query		string	false	"If assignee_id is specified, it must be positive integer or null."	extensions(x-example=5)
//	@param		with_comments	query		bool	false	" "																	default(false)
//	@param		with_history	query		bool	false	" "																	default(false)
//	@param		limit			query		int		false	" "																	minimum(1)	default(20)
//	@param		cursor			query		int		false	" "																	minimum(1)
//	@success	200				{object}	SvcListResp
//	@failure	400
//	@failure	401
func list(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := api.NewHelper(w, r, "tasks.list")

		req, err := parseListQuery(r.URL.Query())
		if err != nil {
			h.WriteError(&api.HttpError{Code: 400, Msg: err.Error()})
			return
		}

		resp, err := svc.List(h.Ctx(), req)
		if err != nil {
			h.WriteError(err)
			return
		}

		h.WriteResponse(200, resp)
	}
}

// parseListQuery ?team_id=1&status=todo&assignee_id=5&limit=20&cursor=11
func parseListQuery(q url.Values) (SvcListReq, error) {
	var req SvcListReq
	var errs []error

	if !q.Has("team_id") {
		errs = append(errs, errors.New("team_id is required"))
	} else {
		s := q.Get("team_id")
		v, err := model.ParseID(s)
		if err != nil || v <= 0 {
			errs = append(errs, fmt.Errorf("team_id: %v", err))
		} else {
			req.TeamID = model.TeamID(v)
		}
	}

	if q.Has("status") {
		s := q.Get("status")
		v, err := model.StatusString(s)
		if err != nil {
			errs = append(errs, errors.New("invalid status"))
		} else {
			req.Status = v
		}
	}

	if q.Has("assignee_id") {
		req.AssigneeID.Defined = true
		s := q.Get("assignee_id")
		if !strings.EqualFold(s, "null") {
			v, err := model.ParseID(s)
			if err != nil || v <= 0 {
				errs = append(errs, fmt.Errorf("assignee_id: %v", err))
			} else {
				req.AssigneeID.Valid = true
				req.AssigneeID.V = model.UserID(v)
			}
		}
	}

	if q.Has("with_comments") {
		s := q.Get("with_comments")
		switch s {
		case "true":
			req.WithComments = true
		case "false":
			req.WithComments = false
		default:
			errs = append(errs, errors.New("with_comments must be true or false"))
		}
	}

	if q.Has("with_history") {
		s := q.Get("with_history")
		switch s {
		case "true":
			req.WithHistory = true
		case "false":
			req.WithHistory = false
		default:
			errs = append(errs, errors.New("with_history must be true or false"))
		}
	}

	req.Limit = 20
	if q.Has("limit") {
		s := q.Get("limit")
		v, err := strconv.Atoi(s)
		if err != nil || v <= 0 {
			errs = append(errs, errors.New("limit must be int > 0"))
		} else {
			req.Limit = v
		}
	}

	if q.Has("cursor") {
		s := q.Get("cursor")
		v, err := model.ParseID(s)
		if err != nil || v <= 0 {
			errs = append(errs, errors.New("cursor must be int > 0"))
		} else {
			req.Cursor = model.TaskID(v)
		}
	}

	return req, errors.Join(errs...)
}

// update godoc
//
//	@tags		tasks
//	@router		/tasks/{id} [put]
//	@summary	Обновление задачи
//	@accept		json
//	@produce	json
//	@param		X-Authtoken	header		string			true	" "
//	@param		id			path		int				true	" "	minimum(1)	extensions(x-example=5)
//	@param		req			body		apiUpdateReq	true	" "
//	@success	200			{object}	model.Task
//	@failure	400
//	@failure	401
//	@failure	409	{object}	model.Task
func update(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := api.NewHelper(w, r, "tasks.update")

		taskID, err := h.GetIDFromPath()
		if err != nil {
			h.WriteError(err)
			return
		}

		var req apiUpdateReq
		if err := h.DecodeBody(&req); err != nil {
			h.WriteError(err)
			return
		}

		task, err := svc.Update(h.Ctx(), SvcUpdateReq{
			TaskID:      model.TaskID(taskID),
			Title:       req.Title,
			Description: req.Description,
			Status:      req.Status,
			AssigneeID:  req.AssigneeID,
			Version:     req.Version,
		})
		if err != nil {
			if errors.Is(err, model.ErrConflict) {
				h.WriteResponse(409, task)
				return
			}
			h.WriteError(err)
			return
		}

		h.WriteResponse(200, task)
	}
}

type apiUpdateReq struct {
	Title       string                 `json:"title,omitempty" validate:"required"`
	Description string                 `json:"description,omitempty"`
	Status      model.Status           `json:"status,omitempty" validate:"required" swaggertype:"string" enums:"todo,in_progress,done,cancelled"`
	AssigneeID  Nullable[model.UserID] `json:"assignee_id,omitzero" swaggertype:"integer" minimum:"1"`
	Version     int64                  `json:"version" validate:"required"`
}

func (req *apiUpdateReq) Validate() error {
	var errs []error

	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		errs = append(errs, errors.New("title is required"))
	}

	req.Description = strings.TrimSpace(req.Description)

	if req.Status == model.StatusUnknown {
		errs = append(errs, errors.New("status is required"))
	}

	if req.AssigneeID.Valid && req.AssigneeID.V <= 0 {
		errs = append(errs, errors.New("assignee_id must be > 0"))
	}

	if req.Version == 0 {
		errs = append(errs, errors.New("version is required"))
	}

	return errors.Join(errs...)
}

// getWithHistory godoc
//
//	@tags		tasks
//	@router		/tasks/{id}/history [get]
//	@summary	История изменений задачи
//	@produce	json
//	@param		X-Authtoken	header		string	true	" "
//	@param		id			path		int		true	" "	minimum(1)	extensions(x-example=5)
//	@success	200			{object}	model.Task
//	@failure	400
func getWithHistory(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := api.NewHelper(w, r, "tasks.getWithHistory")

		taskID, err := h.GetIDFromPath()
		if err != nil {
			h.WriteError(err)
			return
		}

		task, err := svc.Get(h.Ctx(), SvcGetReq{
			TaskID:      model.TaskID(taskID),
			WithHistory: true,
		})
		if err != nil {
			h.WriteError(err)
			return
		}

		h.WriteResponse(200, task)
	}
}

// addComment godoc
//
//	@tags		tasks
//	@router		/tasks/{id}/comments [post]
//	@summary	Добавление комментария к задаче
//	@accept		json
//	@produce	json
//	@param		X-Authtoken	header		string				true	" "
//	@param		id			path		int					true	" "	minimum(1)	extensions(x-example=5)
//	@param		req			body		apiAddCommentReq	true	" "
//	@success	201			{object}	model.TaskComment
//	@failure	400
func addComment(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := api.NewHelper(w, r, "tasks.addComment")

		taskID, err := h.GetIDFromPath()
		if err != nil {
			h.WriteError(err)
			return
		}

		var req apiAddCommentReq
		if err := h.DecodeBody(&req); err != nil {
			h.WriteError(err)
			return
		}

		comment, err := svc.AddComment(h.Ctx(), SvcAddCommentReq{
			TaskID:  model.TaskID(taskID),
			Content: req.Content,
		})
		if err != nil {
			h.WriteError(err)
			return
		}

		h.WriteResponse(201, comment)
	}
}

type apiAddCommentReq struct {
	Content string `json:"content,omitempty" validate:"required"`
}

func (req *apiAddCommentReq) Validate() error {
	var errs []error

	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" {
		errs = append(errs, errors.New("comment is required"))
	}

	return errors.Join(errs...)
}

// getWithComments godoc
//
//	@tags		tasks
//	@router		/tasks/{id}/comments [get]
//	@summary	Список комментариев задачи
//	@produce	json
//	@param		X-Authtoken	header		string	true	" "
//	@param		id			path		int		true	" "	minimum(1)	extensions(x-example=5)
//	@success	200			{object}	model.Task
//	@failure	400
func getWithComments(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := api.NewHelper(w, r, "tasks.getWithComments")

		taskID, err := h.GetIDFromPath()
		if err != nil {
			h.WriteError(err)
			return
		}

		task, err := svc.Get(h.Ctx(), SvcGetReq{
			TaskID:       model.TaskID(taskID),
			WithComments: true,
		})
		if err != nil {
			h.WriteError(err)
			return
		}

		h.WriteResponse(200, task)
	}
}
