package teams

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"aaa2ppp/teams-tasks/internal/db"
	"aaa2ppp/teams-tasks/internal/model"
)

type storage struct {
	db db.DBTX
}

func NewStorage(db db.DBTX) *storage {
	return &storage{db: db}
}

func (s *storage) WithTx(tx db.DBTX) Storage {
	return &storage{db: tx}
}

func (s *storage) Create(ctx context.Context, req DBCreateReq) (model.TeamID, error) {
	const query = "INSERT INTO teams (name, created_by) VALUES (?, ?)"

	res, err := s.db.ExecContext(ctx, query, req.Name, req.CreatedBy)
	if err != nil {
		return 0, db.MapMySqlError(err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	return model.TeamID(id), nil
}

func (s *storage) GetByID(ctx context.Context, req DBGetByIDReq) (model.Team, error) {
	var qb strings.Builder
	var args []any

	qb.WriteString(`
		SELECT id, name, created_by, created_at FROM teams WHERE id = ?
	`)
	args = append(args, req.TeamID)

	if req.CurUserID != 0 {
		qb.WriteString(`
			AND EXISTS (
				SELECT 1
				FROM team_members
				WHERE team_id = ? AND user_id = ?
			)
		`)
		args = append(args, req.TeamID, req.CurUserID)
	}

	query := qb.String()
	row := s.db.QueryRowContext(ctx, query, args...)

	var t model.Team
	if err := row.Scan(&t.ID, &t.Name, &t.CreatedBy, &t.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = fmt.Errorf("%w: teams.id=%d", model.ErrNotFound, req.TeamID)
		}
		return model.Team{}, err
	}

	return t, nil
}

func (s *storage) AddMember(ctx context.Context, req DBAddMemberReq) error {
	if req.UserID == 0 && req.UserEmail == "" {
		return errors.New("must defined member_id or member_email")
	}

	var qb strings.Builder
	var args []any

	qb.WriteString(`
		INSERT INTO team_members (team_id, user_id, role)
		SELECT ?, u.id, ?
		FROM users u
		WHERE 1=1
	`)
	args = append(args, req.TeamID, req.MemberRole)

	if req.UserID != 0 {
		qb.WriteString(" AND u.id = ?")
		args = append(args, req.UserID)
	}
	if req.UserEmail != "" {
		qb.WriteString(" AND u.email = ?")
		args = append(args, req.UserEmail)
	}

	if req.CurUserID != 0 {
		qb.WriteString(`
			AND EXISTS (
				SELECT 1 FROM team_members tm
				WHERE tm.team_id = ? AND tm.user_id = ?
				AND (
					(tm.role = 'owner' AND ? IN ('member', 'admin'))
					OR
					(tm.role = 'admin' AND ? = 'member')
				)
			)
		`)
		args = append(args, req.TeamID, req.CurUserID, req.MemberRole, req.MemberRole)
	}

	query := qb.String()
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return db.MapMySqlError(err)
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

func (s *storage) List(ctx context.Context, req DBListReq) ([]model.Team, error) {
	var qb strings.Builder
	var args []any

	qb.WriteString(`
		SELECT t.id, t.name, t.created_by, t.created_at
		FROM teams t
	`)
	if req.CurUserID != 0 {
		qb.WriteString(`
			JOIN team_members tm ON t.id = tm.team_id
			WHERE tm.user_id = ?
		`)
		args = append(args, req.CurUserID)
	}
	qb.WriteString(`
		ORDER BY t.id
	`)

	query := qb.String()
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	teams := []model.Team{}
	var r model.Team
	for rows.Next() {
		r = model.Team{}
		if err := rows.Scan(&r.ID, &r.Name, &r.CreatedBy, &r.CreatedAt); err != nil {
			return nil, err
		}
		teams = append(teams, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return teams, nil
}

func (s *storage) GetMembers(ctx context.Context, req DBGetByIDsReq) ([]model.TeamMember, error) {
	if len(req.TeamIDs) == 0 {
		return []model.TeamMember{}, nil
	}

	args := make([]any, len(req.TeamIDs))
	for i, id := range req.TeamIDs {
		args[i] = id
	}

	placeholders := strings.Repeat("?,", len(args))
	placeholders = placeholders[:len(placeholders)-1]

	var qb strings.Builder
	qb.WriteString(`
		SELECT tm.team_id, tm.user_id, tm.role, u.email, u.name, u.created_at 
		FROM team_members AS tm 
		JOIN users AS u ON u.id = tm.user_id
		WHERE tm.team_id IN (`)
	qb.WriteString(placeholders)
	qb.WriteString(`)
		ORDER BY tm.team_id
	`)

	query := qb.String()
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	users := []model.TeamMember{}
	var r model.TeamMember
	for rows.Next() {
		r = model.TeamMember{}
		if err := rows.Scan(&r.TeamID, &r.User.ID, &r.Role, &r.User.Email, &r.User.Name, &r.User.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}

func (s *storage) GetTasks(ctx context.Context, req DBGetByIDsReq) ([]model.Task, error) {
	if len(req.TeamIDs) == 0 {
		return []model.Task{}, nil
	}

	args := make([]any, len(req.TeamIDs))
	for i, id := range req.TeamIDs {
		args[i] = id
	}

	placeholders := strings.Repeat("?,", len(args))
	placeholders = placeholders[:len(placeholders)-1]

	var qb strings.Builder
	qb.WriteString(`
		SELECT id, team_id, title, description, status, created_by, assignee_id, 
			created_at, updated_at, closed_at 
		FROM tasks
		WHERE team_id IN (`)
	qb.WriteString(placeholders)
	qb.WriteString(`)
		ORDER BY team_id
	`)

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
			&r.CreatedAt, &r.UpdatedAt, &r.ClosedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tasks, nil
}
