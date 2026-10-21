package tasks_test

import (
	"context"
	"errors"
	"testing"

	database "aaa2ppp/teams-tasks/internal/db"
	"aaa2ppp/teams-tasks/internal/features/tasks"
	"aaa2ppp/teams-tasks/internal/model"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/aaa2ppp/be"
)

func TestGetMemberRole_DBError(t *testing.T) {
	sqlDB, mock, _ := sqlmock.New()
	defer sqlDB.Close() //nolint:errcheck

	db := database.New(sqlDB)
	storage := tasks.NewStorage(db)

	dbErr := errors.New("db error")
	mock.ExpectQuery("SELECT role FROM team_members").
		WithArgs(model.TeamID(1), model.UserID(1)).
		WillReturnError(dbErr)

	ctx := context.Background()
	_, err := storage.GetMemberRole(ctx, tasks.DBGetMemberRoleReq{TeamID: 1, UserID: 1})

	// С багом: ErrNotFound
	// Без бага: dbErr
	be.Err(t, err, dbErr)
}
