package tasks

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"aaa2ppp/teams-tasks/internal/db"
	"aaa2ppp/teams-tasks/internal/lib/logging"
	"aaa2ppp/teams-tasks/internal/model"
)

type storage struct {
	db db.DBTX
}

var _ Storage = &storage{}

func NewStorage(db db.DBTX) *storage {
	return &storage{db: db}
}

func (s *storage) WithTx(tx db.DBTX) Storage {
	return &storage{tx}
}

func (s *storage) Create(ctx context.Context, req DBCreateReq) (model.TaskID, error) {
	var qb strings.Builder
	var args []any

	qb.WriteString(`
		INSERT INTO tasks (team_id, title, description, status, created_by, assignee_id)
		SELECT ?, ?, ?, ?, ?, ?
		FROM (SELECT 1) AS q
		WHERE 1=1
	`)
	args = append(args, req.TeamID, req.Title, req.Description, req.Status, req.CreatedBy, req.AssigneeID)

	if req.CreatedBy != 0 {
		qb.WriteString(`
			AND EXISTS (
				SELECT 1
				FROM team_members
				WHERE team_id = ? AND user_id = ?
			)
		`)
		args = append(args, req.TeamID, req.CreatedBy)
	}

	if req.AssigneeID.Valid {
		qb.WriteString(`
			AND EXISTS (
				SELECT 1
				FROM team_members
				WHERE team_id = ? AND user_id = ?
			)
		`)
		args = append(args, req.TeamID, req.AssigneeID.V)
	}

	query := qb.String()
	logging.GetLogger(ctx).Debug("CreateTask", "query", query, "args", args)

	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, model.ErrNoRowsAffected
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	return model.TaskID(id), nil
}

func (s *storage) GetByID(ctx context.Context, req DBGetByIDReq) (model.Task, error) {
	var qb strings.Builder
	var args []any

	qb.WriteString(`
		SELECT id, team_id, title, description, status, created_by, assignee_id, 
			created_at, updated_at, closed_at, version
		FROM tasks t
		WHERE id = ?
	`)
	args = append(args, req.TaskID)

	if req.CurUserID != 0 {
		qb.WriteString(`
			AND EXISTS (
				SELECT 1
				FROM team_members
				WHERE team_id=t.team_id AND user_id = ?
			)
		`)
		args = append(args, req.CurUserID)
	}

	query := qb.String()
	row := s.db.QueryRowContext(ctx, query, args...)

	var r model.Task
	if err := row.Scan(&r.ID, &r.TeamID, &r.Title, &r.Description, &r.Status, &r.CreatedBy, &r.AssigneeID,
		&r.CreatedAt, &r.UpdatedAt, &r.ClosedAt, &r.Version,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Task{}, fmt.Errorf("%w: tasks.task_id=%d", model.ErrNotFound, req.TaskID)
		}
		return model.Task{}, err
	}

	return r, nil
}

func (s *storage) List(ctx context.Context, req DBListReq) ([]model.Task, error) {
	var qb strings.Builder
	var args []any

	qb.WriteString(`
		SELECT t.id, t.team_id, t.title, t.description, t.status, t.created_by, t.assignee_id, 
			t.created_at, t.updated_at, t.closed_at, version
		FROM tasks t
	`)
	if req.CurUserID != 0 {
		qb.WriteString(`
			JOIN team_members tm ON tm.team_id = t.team_id
		`)
	}
	qb.WriteString(`
		WHERE 1=1	
	`)

	if req.CurUserID != 0 {
		qb.WriteString(" AND tm.user_id = ?")
		args = append(args, req.CurUserID)
	}

	if req.TeamID != 0 {
		qb.WriteString(" AND t.team_id = ?")
		args = append(args, req.TeamID)
	}

	if req.Status != 0 {
		qb.WriteString(" AND t.status = ?")
		args = append(args, req.Status)
	}

	if req.AssigneeID.Defined {
		if !req.AssigneeID.Valid {
			qb.WriteString(" AND t.assignee_id IS NULL")
		} else {
			qb.WriteString(" AND t.assignee_id = ?")
			args = append(args, req.AssigneeID.V)
		}
	}

	qb.WriteString(`
		ORDER BY t.id
		LIMIT ?	OFFSET ?
	`)
	args = append(args, req.Limit, req.Offset)

	query := qb.String()
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	tasks := []model.Task{}
	var r model.Task
	for rows.Next() {
		r = model.Task{}
		if err := rows.Scan(&r.ID, &r.TeamID, &r.Title, &r.Description, &r.Status, &r.CreatedBy, &r.AssigneeID,
			&r.CreatedAt, &r.UpdatedAt, &r.ClosedAt, &r.Version); err != nil {
			return nil, err
		}
		tasks = append(tasks, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tasks, nil
}

func (s *storage) GetComments(ctx context.Context, taskIDs []model.TaskID) ([]model.TaskComment, error) {
	if len(taskIDs) == 0 {
		return []model.TaskComment{}, nil
	}

	args := make([]any, 0, len(taskIDs))
	for _, id := range taskIDs {
		args = append(args, id)
	}
	placeholders := strings.Repeat("?,", len(args))
	placeholders = placeholders[:len(placeholders)-1]

	var qb strings.Builder
	qb.WriteString(`
		SELECT id, task_id, user_id, content, created_at
		FROM task_comments
		WHERE task_id IN (`)
	qb.WriteString(placeholders)
	qb.WriteString(`)
		ORDER BY task_id, created_at
	`)

	query := qb.String()
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	comments := []model.TaskComment{}
	var r model.TaskComment
	for rows.Next() {
		r = model.TaskComment{}
		if err := rows.Scan(&r.ID, &r.TaskID, &r.UserID, &r.Content, &r.CreatedAt); err != nil {
			return nil, err
		}
		comments = append(comments, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return comments, nil
}

func (s *storage) GetHistory(ctx context.Context, taskIDs []model.TaskID) ([]model.TaskHistoryEvent, error) {
	if len(taskIDs) == 0 {
		return []model.TaskHistoryEvent{}, nil
	}

	args := make([]any, 0, len(taskIDs))
	for _, id := range taskIDs {
		args = append(args, id)
	}
	placeholders := strings.Repeat("?,", len(args))
	placeholders = placeholders[:len(placeholders)-1]

	var qb strings.Builder
	qb.WriteString(`
		SELECT id, task_id, changed_by, changes, created_at
		FROM task_history th
		WHERE task_id IN (
	`)
	qb.WriteString(placeholders)
	qb.WriteString(`)
		ORDER BY task_id, created_at
	`)

	query := qb.String()
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	history := []model.TaskHistoryEvent{}
	var r model.TaskHistoryEvent
	var changes []byte
	for rows.Next() {
		r = model.TaskHistoryEvent{}
		changes = changes[:0]
		if err := rows.Scan(&r.ID, &r.TaskID, &r.ChangedBy, &changes, &r.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.NewDecoder(bytes.NewReader(changes)).Decode(&r.Changes); err != nil {
			return nil, err
		}
		history = append(history, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return history, nil
}

func (s *storage) Update(ctx context.Context, req DBUpdateReq) error {
	var qb strings.Builder
	var args []any

	qb.WriteString(`
		UPDATE tasks t
		SET version = version+1, title = ?, description = ?, status = ?, assignee_id = ?, closed_at = ?
		WHERE id = ? AND version = ?
	`)
	args = append(args, req.Title, req.Description, req.Status, req.AssigneeID, req.ClosedAt)
	args = append(args, req.TaskID, req.Version)

	if req.CurUserID != 0 {
		qb.WriteString(`
			AND (
				(created_by = ? AND ?)
				OR 
				(assignee_id = ? AND ?)
				OR 
				EXISTS (
					SELECT 1
					FROM team_members tm
					WHERE tm.team_id = t.team_id AND tm.user_id = ?
					AND (
						(tm.role = 'owner' AND ?)
						OR
						(tm.role = 'admin' AND ?)
					)
				)
			)
		`)
		args = append(args, req.CurUserID, req.Allow&TaskCreator != 0)
		args = append(args, req.CurUserID, req.Allow&TaskAssignee != 0)
		args = append(args, req.CurUserID, req.Allow&TeamOwner != 0, req.Allow&TeamAdmin != 0)
	}

	if req.AssigneeID.Valid {
		qb.WriteString(`
			AND (
				(assignee_id = ?)
				OR
				EXISTS (
					SELECT 1
					FROM team_members
					WHERE team_id = t.team_id AND user_id = ?
				)
			)
		`)
		args = append(args, req.AssigneeID.V, req.AssigneeID.V)
	}

	query := qb.String()
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return model.ErrNoRowsAffected
	}

	return nil
}

func (s *storage) AddComment(ctx context.Context, req DBAddCommentReq) (model.TaskCommentID, error) {
	var qb strings.Builder
	var args []any

	qb.WriteString(`
		INSERT INTO task_comments (task_id, user_id, content)
		SELECT ?, ?, ?
		FROM tasks t
		WHERE
			(created_by = ? AND ?)
			OR 
			(assignee_id = ? AND ?)
			OR 
			EXISTS (
				SELECT 1
				FROM team_members tm
				WHERE tm.team_id = t.team_id AND tm.user_id = ?
				AND (
					(tm.role = 'owner' AND ?)
					OR
					(tm.role = 'admin' AND ?)
				)
			)
	`)
	args = append(args, req.TaskID, req.UserID, req.Content)
	args = append(args, req.UserID, req.Allow&TaskCreator != 0)
	args = append(args, req.UserID, req.Allow&TaskAssignee != 0)
	args = append(args, req.UserID, req.Allow&TeamOwner != 0, req.Allow&TeamAdmin != 0)

	query := qb.String()
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, model.ErrNoRowsAffected
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	return model.TaskCommentID(id), nil
}

func (s *storage) GetCommentByID(ctx context.Context, req DBGetCommentByIDReq) (model.TaskComment, error) {
	var qb strings.Builder
	var args []any

	qb.WriteString(`
		SELECT id, task_id, user_id, content, created_at
		FROM task_comments
		WHERE id = ?
	`)
	args = append(args, req.CommentID)

	if req.CurUserID != 0 {
		qb.WriteString(`
			AND EXISTS (
				SELECT 1
				FROM tasks t
				JOIN team_members tm ON tm.team_id = t.team_id
				WHERE tm.user_id = ?
			)
		`)
		args = append(args, req.CurUserID)
	}

	query := qb.String()
	row := s.db.QueryRowContext(ctx, query, args...)

	var tc model.TaskComment
	if err := row.Scan(&tc.ID, &tc.TaskID, &tc.UserID, &tc.Content, &tc.CreatedAt); err != nil {
		return model.TaskComment{}, err
	}

	return tc, nil
}

func (s *storage) AddHistoryEvent(ctx context.Context, req DBAddHistoryEventReq) (model.TaskHistoryEventID, error) {
	var zero model.TaskHistoryEventID

	var jb bytes.Buffer
	if err := json.NewEncoder(&jb).Encode(req.Changes); err != nil {
		return zero, err
	}
	changes := jb.Bytes()

	res, err := s.db.ExecContext(ctx, "INSERT INTO task_history (task_id, changed_by, changes) VALUES (?, ?, ?)",
		req.TaskID, req.ChangedBy, changes)
	if err != nil {
		return zero, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return zero, err
	}

	return model.TaskHistoryEventID(id), nil
}

func (s *storage) GetMemberRole(ctx context.Context, req DBGetMemberRoleReq) (model.Role, error) {
	row := s.db.QueryRowContext(ctx, "SELECT role FROM team_members WHERE team_id = ? AND user_id = ?",
		req.TeamID, req.UserID)

	var role model.Role
	if err := row.Scan(&role); err != nil {
		if err := sql.ErrNoRows; err != nil {
			return 0, model.ErrNotFound
		}
		return 0, err
	}

	return role, nil
}
