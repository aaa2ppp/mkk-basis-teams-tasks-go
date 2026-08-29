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

// TestTaskListAccess проверяет, что список задач доступен только для своей команды,
// а также корректность фильтрации по статусу и исполнителю.
func TestTaskListAccess(t *testing.T) {
	ctx := context.Background()
	db, cleanup := StartTestDatabase(t)
	defer cleanup()

	taskStorage := tasks.NewStorage(db)
	cache := &NoopCache{}
	taskSvc := tasks.NewService(taskStorage, db, cache)

	owner1 := InsertUser(t, db, "owner1@test.com", "Owner1", "hash")
	member1 := InsertUser(t, db, "member1@test.com", "Member1", "hash")
	owner2 := InsertUser(t, db, "owner2@test.com", "Owner2", "hash")
	member2 := InsertUser(t, db, "member2@test.com", "Member2", "hash")

	team1 := InsertTeam(t, db, "Team1", owner1)
	AddMember(t, db, team1, owner1, model.RoleOwner)
	AddMember(t, db, team1, member1, model.RoleMember)

	team2 := InsertTeam(t, db, "Team2", owner2)
	AddMember(t, db, team2, owner2, model.RoleOwner)
	AddMember(t, db, team2, member2, model.RoleMember)

	now := time.Now()
	// Задачи в team1
	task1 := CreateTask(t, db, team1, "T1", "desc", model.StatusTodo, owner1, nil, now, nil)
	task2 := CreateTask(t, db, team1, "T2", "desc", model.StatusDone, owner1, &member1, now, nil)
	// Задача в team2
	task3 := CreateTask(t, db, team2, "T3", "desc", model.StatusTodo, owner2, nil, now, nil)
	_ = task3

	t.Run("member of team1 can list team1 tasks", func(t *testing.T) {
		req := tasks.SvcListReq{TeamID: team1, Limit: 10}
		ctxMember := auth.ContextWithUserForTest(ctx, model.User{
			ID:    member1,
			Roles: model.UserRoles{team1.String(): model.RoleMember},
		})
		resp, err := taskSvc.List(ctxMember, req)
		be.Err(t, err, nil)
		be.Equal(t, len(resp.Tasks), 2)
		ids := map[model.TaskID]bool{task1: true, task2: true}
		for _, task := range resp.Tasks {
			be.True(t, ids[task.ID])
		}
	})

	t.Run("member of team1 cannot list team2 tasks", func(t *testing.T) {
		req := tasks.SvcListReq{TeamID: team2, Limit: 10}
		ctxMember := auth.ContextWithUserForTest(ctx, model.User{
			ID:    member1,
			Roles: model.UserRoles{team1.String(): model.RoleMember},
		})
		_, err := taskSvc.List(ctxMember, req)
		be.Err(t, err, model.ErrForbidden) // или ErrNotFound
	})

	t.Run("owner of team1 can list with filter by status", func(t *testing.T) {
		ctxOwner := auth.ContextWithUserForTest(ctx, model.User{
			ID:    owner1,
			Roles: model.UserRoles{team1.String(): model.RoleOwner},
		})
		req := tasks.SvcListReq{TeamID: team1, Status: model.StatusDone, Limit: 10}
		resp, err := taskSvc.List(ctxOwner, req)
		be.Err(t, err, nil)
		if be.Equal(t, len(resp.Tasks), 1) {
			be.Equal(t, resp.Tasks[0].ID, task2)
		}
	})

	t.Run("owner of team1 can list with filter by assignee", func(t *testing.T) {
		req := tasks.SvcListReq{
			TeamID:     team1,
			AssigneeID: Val(member1),
			Limit:      10,
		}
		ctxOwner := auth.ContextWithUserForTest(ctx, model.User{
			ID:    owner1,
			Roles: model.UserRoles{team1.String(): model.RoleOwner},
		})
		resp, err := taskSvc.List(ctxOwner, req)
		be.Err(t, err, nil)
		if be.Equal(t, len(resp.Tasks), 1) {
			be.Equal(t, resp.Tasks[0].ID, task2)
		}
	})

	t.Run("outsider cannot list any team tasks", func(t *testing.T) {
		outsider := InsertUser(t, db, "outsider@test.com", "Outsider", "hash")
		ctxOut := auth.ContextWithUserForTest(ctx, model.User{ID: outsider})

		req := tasks.SvcListReq{TeamID: team1, Limit: 10}
		_, err := taskSvc.List(ctxOut, req)
		be.Err(t, err, model.ErrForbidden)

		req = tasks.SvcListReq{TeamID: team2, Limit: 10}
		_, err = taskSvc.List(ctxOut, req)
		be.Err(t, err, model.ErrForbidden)
	})
}
