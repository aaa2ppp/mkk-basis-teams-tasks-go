package tests

import (
	"context"
	"testing"
	"time"

	"aaa2ppp/teams-tasks/internal/features/tasks"
	"aaa2ppp/teams-tasks/internal/lib/auth"
	"aaa2ppp/teams-tasks/internal/model"

	"github.com/aaa2ppp/be"
	_ "github.com/go-sql-driver/mysql"
)

// TestTaskGet проверяет доступ к задаче по ID, включая опции withComments и withHistory.
func TestTaskGet(t *testing.T) {
	ctx := context.Background()
	db, cleanup := StartTestDatabase(t)
	defer cleanup()

	taskStorage := tasks.NewStorage(db)
	cache := &NoopCache{}
	taskSvc := tasks.NewService(taskStorage, db, cache)

	owner := InsertUser(t, db, "owner@test.com", "Owner", "hash")
	member := InsertUser(t, db, "member@test.com", "Member", "hash")
	outsider := InsertUser(t, db, "outsider@test.com", "Outsider", "hash")

	teamID := InsertTeam(t, db, "Test Team", owner)
	AddMember(t, db, teamID, owner, model.RoleOwner)
	AddMember(t, db, teamID, member, model.RoleMember)

	now := time.Now()
	taskID := CreateTask(t, db, teamID, "Test Task", "desc", model.StatusTodo, owner, nil, now, nil)
	AddComment(t, db, taskID, member, "comment 1", &now)

	// Добавим историю, обновив задачу
	updateReq := tasks.SvcUpdateReq{
		TaskID:  taskID,
		Title:   "Updated Title",
		Version: 1, // начальная версия
	}
	ctxOwner := auth.ContextWithUserForTest(ctx, model.User{
		ID: owner,
		Roles: model.UserRoles{
			teamID.String(): model.RoleOwner,
		},
	})
	_, err := taskSvc.Update(ctxOwner, updateReq)
	be.Err(t, err, nil)

	t.Run("member can get task with comments and history", func(t *testing.T) {
		req := tasks.SvcGetReq{
			TaskID:       taskID,
			WithComments: true,
			WithHistory:  true,
		}
		ctxMember := auth.ContextWithUserForTest(ctx, model.User{ID: member})
		task, err := taskSvc.Get(ctxMember, req)
		be.Err(t, err, nil)
		be.Equal(t, task.ID, taskID)
		be.Equal(t, len(task.Comments), 1)
		be.Equal(t, task.Comments[0].Content, "comment 1")
		be.Equal(t, len(task.History), 1) // как минимум одно изменение
	})

	t.Run("member can get task without extra data", func(t *testing.T) {
		req := tasks.SvcGetReq{
			TaskID:       taskID,
			WithComments: false,
			WithHistory:  false,
		}
		ctxMember := auth.ContextWithUserForTest(ctx, model.User{ID: member})
		task, err := taskSvc.Get(ctxMember, req)
		be.Err(t, err, nil)
		be.Equal(t, task.ID, taskID)
		be.Equal(t, task.Comments, nil)
		be.Equal(t, task.History, nil)
	})

	t.Run("outsider cannot get task", func(t *testing.T) {
		req := tasks.SvcGetReq{TaskID: taskID}
		ctxOut := auth.ContextWithUserForTest(ctx, model.User{ID: outsider})
		_, err := taskSvc.Get(ctxOut, req)
		be.Err(t, err, model.ErrNotFound)
	})
}
