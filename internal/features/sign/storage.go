package sign

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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
	return &storage{tx}
}

func (s *storage) Create(ctx context.Context, user model.User) (model.UserID, error) {
	const query = "INSERT INTO users (name, email, password_hash) VALUES (?, ?, ?)"

	res, err := s.db.ExecContext(ctx, query, user.Name, user.Email, user.PasswordHash)
	if err != nil {
		return 0, db.MapMySqlError(err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	return model.UserID(id), nil
}

func (s *storage) GetByID(ctx context.Context, id model.UserID) (model.User, error) {
	const query = "SELECT id, name, email, password_hash, created_at FROM users WHERE id=?"

	row := s.db.QueryRowContext(ctx, query, id)

	var u model.User
	if err := row.Scan(&u.ID, &u.Name, &u.Email, &u.PasswordHash, &u.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = fmt.Errorf("%w: users.id=%d", model.ErrNotFound, id)
		}
		return model.User{}, err
	}

	return u, nil
}

func (s *storage) GetByEmail(ctx context.Context, email string) (model.User, error) {
	const query = "SELECT id, name, email, password_hash, created_at FROM users WHERE email=?"

	row := s.db.QueryRowContext(ctx, query, email)

	var u model.User
	if err := row.Scan(&u.ID, &u.Name, &u.Email, &u.PasswordHash, &u.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = fmt.Errorf("%w: users.email=%q", model.ErrNotFound, email)
		}
		return model.User{}, err
	}

	return u, nil
}

func (s *storage) GetRoles(ctx context.Context, userID model.UserID) (map[string]model.Role, error) {
	const query = "SELECT team_id, role FROM team_members WHERE user_id = ?"

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}

	roles := map[string]model.Role{}
	var r model.TeamMember
	for rows.Next() {
		r = model.TeamMember{}
		if err := rows.Scan(&r.TeamID, &r.Role); err != nil {
			return nil, err
		}
		roles[r.TeamID.String()] = r.Role
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return roles, err
}
