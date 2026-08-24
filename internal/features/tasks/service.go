package tasks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"aaa2ppp/teams-tasks/internal/db"
	"aaa2ppp/teams-tasks/internal/lib/auth"
	"aaa2ppp/teams-tasks/internal/lib/logging"
	"aaa2ppp/teams-tasks/internal/model"
)

type DBCreateReq struct {
	TeamID      model.TeamID
	Title       string
	Description string
	Status      model.Status
	CreatedBy   model.UserID
	AssigneeID  Nullable[model.UserID]
}

type DBGetByIDReq struct {
	model.TaskID
	CurUserID model.UserID
}

type DBListReq struct {
	TeamID     model.TeamID
	Status     model.Status
	AssigneeID Nullable[model.UserID]
	Limit      int64
	Offset     int64
	CurUserID  model.UserID
}

type DBUpdateReq struct {
	TaskID      model.TaskID
	Title       string
	Description string
	Status      model.Status
	AssigneeID  Nullable[model.UserID]
	Version     int64
	CurUserID   model.UserID
	Allow       UpdateRole
	ClosedAt    Nullable[time.Time]
}

type UpdateRole uint8

const (
	_ UpdateRole = 1 << iota
	TeamOwner
	TeamAdmin
	TaskCreator
	TaskAssignee
)

type Change = model.Change

type DBAddHistoryEventReq struct {
	TaskID    model.TaskID
	ChangedBy model.UserID
	Changes   map[string]*Change
}

type DBAddCommentReq struct {
	TaskID  model.TaskID
	UserID  model.UserID
	Content string
	Allow   UpdateRole
}

type DBGetCommentByIDReq struct {
	CommentID model.TaskCommentID
	CurUserID model.UserID
}

type DBGetMemberRoleReq struct {
	TeamID model.TeamID
	UserID model.UserID
}

type Storage interface {
	WithTx(tx db.DBTX) Storage

	Create(ctx context.Context, req DBCreateReq) (model.TaskID, error)
	GetByID(ctx context.Context, req DBGetByIDReq) (model.Task, error)

	// List ДОЛЖЕН возвращать список отсортированный по ID
	List(ctx context.Context, req DBListReq) ([]model.Task, error)
	// GetComments ДОЛЖЕН возвращать список отсортированный по TaskID
	GetComments(ctx context.Context, taskIDs []model.TaskID) ([]model.TaskComment, error)
	// GetHistory ДОЛЖЕН возвращать список отсортированный по TaskID
	GetHistory(ctx context.Context, taskIDs []model.TaskID) ([]model.TaskHistoryEvent, error)

	Update(ctx context.Context, req DBUpdateReq) error
	AddHistoryEvent(ctx context.Context, req DBAddHistoryEventReq) (model.TaskHistoryEventID, error)

	AddComment(ctx context.Context, req DBAddCommentReq) (model.TaskCommentID, error)
	GetCommentByID(ctx context.Context, req DBGetCommentByIDReq) (model.TaskComment, error)

	GetMemberRole(ctx context.Context, req DBGetMemberRoleReq) (model.Role, error)
}

type Transactor interface {
	InTx(context.Context, func(context.Context, db.DBTX) error) error
}

type Cache interface {
	Get(ctx context.Context, key string, val any) error
	Put(ctx context.Context, key string, val any) error
	Invalidate(ctx context.Context, key string) error
}

func invalidatePattern(teamID model.TeamID) string {
	return fmt.Sprintf("tasks:team:%d:*", teamID)
}

func buildTasksKey(req SvcListReq) string {
	var kb strings.Builder
	fmt.Fprintf(&kb, "tasks:team:%d", req.TeamID)
	if req.Status != 0 {
		fmt.Fprintf(&kb, ":status:%v", req.Status)
	}
	if req.AssigneeID.Defined {
		fmt.Fprintf(&kb, ":assignee:%v", req.AssigneeID)
	}
	fmt.Fprintf(&kb, ":limit:%d:offset:%d", req.Limit, req.Offset)
	return kb.String()
}

func getOrLoadTasks(ctx context.Context, req SvcListReq, cache Cache, fn func() ([]model.Task, error)) ([]model.Task, error) {
	var tasks []model.Task

	key := buildTasksKey(req)

	if err := cache.Get(ctx, key, &tasks); err == nil {
		logging.GetLogger(ctx).Debug("match cache", "key", key)

	} else {
		if !errors.Is(err, model.ErrNotFound) {
			logging.GetLogger(ctx).Warn("get from cache", "error", err)
		}

		tasks, err = fn()
		if err != nil {
			return nil, err
		}

		if err := cache.Put(ctx, key, tasks); err != nil {
			logging.GetLogger(ctx).Warn("put to cache", "error", err)
		}
	}

	return tasks, nil
}

type service struct {
	storage    Storage
	transactor Transactor
	cache      Cache
}

var _ Service = &service{}

func NewService(storage Storage, transactor Transactor, cache Cache) *service {
	return &service{
		storage:    storage,
		transactor: transactor,
		cache:      cache,
	}
}

func (s *service) Create(ctx context.Context, req SvcCreateReq) (model.Task, error) {
	var zero model.Task

	curUser, err := auth.GetCurrentUser(ctx)
	if err != nil {
		return zero, fmt.Errorf("%w: %v", model.ErrUnauthorized, err)
	}

	var task model.Task
	if err := s.transactor.InTx(ctx, func(ctx context.Context, tx db.DBTX) (err error) {
		storage := s.storage.WithTx(tx)
		taskID, err := storage.Create(ctx, DBCreateReq{
			TeamID:      req.TeamID,
			Title:       req.Title,
			Description: req.Description,
			Status:      req.Status,
			CreatedBy:   curUser.ID,
			AssigneeID:  req.AssigneeID,
		})
		if err != nil {
			return err
		}
		task, err = storage.GetByID(ctx, DBGetByIDReq{
			TaskID: taskID,
		})
		return
	}); err != nil {
		return zero, err
	}

	if err := s.cache.Invalidate(ctx, invalidatePattern(task.TeamID)); err != nil {
		logging.GetLogger(ctx).Warn("invalidate cache", "error", err, "team_id", task.TeamID)
	}

	return task, err
}

func (s *service) Get(ctx context.Context, req SvcGetReq) (model.Task, error) {
	var zero model.Task

	curUser, err := auth.GetCurrentUser(ctx)
	if err != nil {
		return zero, err
	}

	task, err := s.storage.GetByID(ctx, DBGetByIDReq{
		TaskID:    req.TaskID,
		CurUserID: curUser.ID,
	})
	if err != nil {
		return zero, err
	}

	if req.WithComments {
		comments, err := s.storage.GetComments(ctx, []model.TaskID{req.TaskID})
		if err != nil {
			return zero, err
		}
		task.Comments = comments
	}

	if req.WithHistory {
		history, err := s.storage.GetHistory(ctx, []model.TaskID{req.TaskID})
		if err != nil {
			return zero, err
		}
		task.History = history
	}

	return task, nil
}

func (s *service) List(ctx context.Context, req SvcListReq) ([]model.Task, error) {
	curUser, err := auth.GetCurrentUser(ctx)
	if err != nil {
		return nil, err
	}

	if _, ok := curUser.TeamRoles[req.TeamID.String()]; !ok {
		return nil, model.ErrForbidden
	}

	tasks, err := getOrLoadTasks(ctx, req, s.cache, func() ([]model.Task, error) {
		return s.storage.List(ctx, DBListReq{
			TeamID:     req.TeamID,
			Status:     req.Status,
			AssigneeID: req.AssigneeID,
			Limit:      req.Limit,
			Offset:     req.Offset,
			CurUserID:  curUser.ID,
		})
	})
	if err != nil {
		return nil, err
	}

	if len(tasks) == 0 || (!req.WithComments && !req.WithHistory) {
		return tasks, nil
	}

	tasksIDs := make([]model.TaskID, 0, len(tasks))
	for i := range tasks {
		tasksIDs = append(tasksIDs, tasks[i].ID)
	}

	if req.WithComments {
		comments, err := s.storage.GetComments(ctx, tasksIDs)
		if err != nil {
			return nil, err
		}
		collectComments(tasks, comments)
	}

	if req.WithHistory {
		history, err := s.storage.GetHistory(ctx, tasksIDs)
		if err != nil {
			return nil, err
		}
		collectHistory(tasks, history)
	}

	return tasks, nil
}

func collectComments(tasks []model.Task, comments []model.TaskComment) {
	i, l := 0, 0
	for l < len(comments) {
		id := comments[l].TaskID
		for i < len(tasks) && tasks[i].ID < id {
			i++
		}
		if i == len(tasks) {
			break
		}
		r := l + 1
		for r < len(comments) && comments[r].TaskID == id {
			r++
		}
		if tasks[i].ID == id {
			tasks[i].Comments = comments[l : r : r-l]
			i++
		}
		l = r
	}
}

func collectHistory(tasks []model.Task, history []model.TaskHistoryEvent) {
	i, l := 0, 0
	for l < len(history) {
		id := history[l].TaskID
		for i < len(tasks) && tasks[i].ID < id {
			i++
		}
		if i == len(tasks) {
			break
		}
		r := l + 1
		for r < len(history) && history[r].TaskID == id {
			r++
		}
		if tasks[i].ID == id {
			tasks[i].History = history[l : r : r-l]
			i++
		}
		l = r
	}
}

func (s *service) Update(ctx context.Context, req SvcUpdateReq) (model.Task, error) {
	var zero model.Task

	curUser, err := auth.GetCurrentUser(ctx)
	if err != nil {
		return model.Task{}, err
	}

	oldTask, err := s.storage.GetByID(ctx, DBGetByIDReq{
		TaskID:    req.TaskID,
		CurUserID: curUser.ID,
	})
	if err != nil {
		return zero, err
	}

	if req.Version != oldTask.Version {
		return oldTask, model.ErrConflict
	}

	allow := ^UpdateRole(0)
	changes := map[string]*Change{}

	if req.Title != oldTask.Title {
		changes["title"] = &Change{
			Old: oldTask.Title,
			New: req.Title,
		}
		allow &= TeamOwner | TeamAdmin | TaskCreator
	}
	if req.Description != oldTask.Description {
		changes["description"] = &Change{
			Old: oldTask.Description,
			New: req.Description,
		}
		allow &= TeamOwner | TeamAdmin | TaskCreator
	}

	closedAt := oldTask.ClosedAt
	if req.Status != oldTask.Status {
		if req.Status == model.StatusDone || req.Status == model.StatusCancelled {
			closedAt.Valid = true
			closedAt.V = time.Now()
		} else {
			closedAt.Valid = false
		}
		changes["status"] = &Change{
			Old: oldTask.Status,
			New: req.Status,
		}
		allow &= TeamOwner | TeamAdmin | TaskCreator | TaskAssignee
	}

	if req.AssigneeID != oldTask.AssigneeID {
		changes["assignee_id"] = &Change{
			Old: oldTask.AssigneeID,
			New: req.AssigneeID,
		}
		allow &= TeamOwner | TeamAdmin | TaskCreator
	}

	if allow == 0 {
		return zero, model.ErrForbidden
	}

	if len(changes) == 0 {
		return oldTask, nil
	}

	var task model.Task
	if err := s.transactor.InTx(ctx, func(ctx context.Context, tx db.DBTX) (err error) {
		storage := s.storage.WithTx(tx)
		if err := storage.Update(ctx, DBUpdateReq{
			TaskID:      req.TaskID,
			Title:       req.Title,
			Description: req.Description,
			Status:      req.Status,
			AssigneeID:  req.AssigneeID,
			Version:     req.Version,
			CurUserID:   curUser.ID,
			Allow:       allow,
			ClosedAt:    closedAt,
		}); err != nil {
			return err
		}

		if _, err := storage.AddHistoryEvent(ctx, DBAddHistoryEventReq{
			TaskID:    req.TaskID,
			ChangedBy: curUser.ID,
			Changes:   changes,
		}); err != nil {
			return err
		}

		task, err = storage.GetByID(ctx, DBGetByIDReq{
			TaskID: req.TaskID,
		})
		return
	}); err != nil {
		return zero, err
	}

	if err := s.cache.Invalidate(ctx, invalidatePattern(task.TeamID)); err != nil {
		logging.GetLogger(ctx).Warn("cache invalidate", "error", err)
	}

	return task, nil
}

func (s *service) AddComment(ctx context.Context, req SvcAddCommentReq) (model.TaskComment, error) {
	var zero model.TaskComment

	curUser, err := auth.GetCurrentUser(ctx)
	if err != nil {
		return zero, err
	}

	var comment model.TaskComment
	if err := s.transactor.InTx(ctx, func(ctx context.Context, tx db.DBTX) (err error) {
		storage := s.storage.WithTx(tx)
		commentID, err := storage.AddComment(ctx, DBAddCommentReq{
			TaskID:  req.TaskID,
			UserID:  curUser.ID,
			Content: req.Content,
			Allow:   TeamOwner | TeamAdmin | TaskCreator | TaskAssignee,
		})
		if err != nil {
			return err
		}
		comment, err = storage.GetCommentByID(ctx, DBGetCommentByIDReq{
			CommentID: commentID,
		})
		return
	}); err != nil {
		return zero, err
	}
	return comment, nil
}
