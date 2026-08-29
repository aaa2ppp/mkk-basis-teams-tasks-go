package tests

import (
	"context"
	"testing"
	"time"

	"aaa2ppp/teams-tasks/internal/features/tasks"
	"aaa2ppp/teams-tasks/internal/lib/auth"
	"aaa2ppp/teams-tasks/internal/model"

	"github.com/aaa2ppp/be"
	"github.com/aaa2ppp/be/tb"
	_ "github.com/go-sql-driver/mysql"
)

// TestTaskUpdateExtended проверяет дополнительные сценарии обновления.
func TestTaskUpdateExtended(t *testing.T) {
	ctx := context.Background()
	db, cleanup := StartTestDatabase(t)
	defer cleanup()

	taskStorage := tasks.NewStorage(db)
	cache := &NoopCache{}
	taskSvc := tasks.NewService(taskStorage, db, cache)

	owner := InsertUser(t, db, "owner@test.com", "Owner", "hash")
	admin := InsertUser(t, db, "admin@test.com", "Admin", "hash")
	creator := InsertUser(t, db, "creator@test.com", "Creator", "hash")
	assignee := InsertUser(t, db, "assignee@test.com", "Assignee", "hash")
	otherMember := InsertUser(t, db, "other@test.com", "Other", "hash")

	teamID := InsertTeam(t, db, "Test Team", owner)
	AddMember(t, db, teamID, owner, model.RoleOwner)
	AddMember(t, db, teamID, admin, model.RoleAdmin)
	AddMember(t, db, teamID, creator, model.RoleMember)
	AddMember(t, db, teamID, assignee, model.RoleMember)
	AddMember(t, db, teamID, otherMember, model.RoleMember)

	now := time.Now().Round(time.Millisecond)
	taskID := CreateTask(t, db, teamID, "Task", "desc", model.StatusTodo, creator, &assignee, now, nil)

	// проверяем, что GetByID возвращает ожидаемый результат
	task, _ := taskStorage.GetByID(ctx, tasks.DBGetByIDReq{TaskID: taskID})
	be.Equal(tb.Require(t), task.ID, taskID)
	be.Equal(tb.Require(t), task.Title, "Task")
	be.Equal(tb.Require(t), task.Description, "desc")
	be.Equal(tb.Require(t), task.Status, model.StatusTodo)
	be.Equal(tb.Require(t), task.CreatedBy, creator)
	be.Equal(tb.Require(t), task.AssigneeID, Val(assignee))
	be.Equal(tb.Require(t), task.CreatedAt, now)
	be.True(tb.Require(t), !task.ClosedAt.Valid)

	initUpdateReq := func(t *testing.T, taskID model.TaskID) tasks.SvcUpdateReq {
		t.Helper()
		task, err := taskStorage.GetByID(ctx, tasks.DBGetByIDReq{TaskID: taskID})
		be.Err(t, err, nil)
		return tasks.SvcUpdateReq{
			TaskID:      task.ID,
			Title:       task.Title,
			Description: task.Description,
			Status:      task.Status,
			AssigneeID:  task.AssigneeID,
			Version:     task.Version,
		}
	}

	// 2. При изменении статуса с Done на другое, closed_at должен стать null
	t.Run("closed_at becomes null when moving from done", func(t *testing.T) {
		ctxOwner := auth.ContextWithUserForTest(ctx, model.User{
			ID:    owner,
			Roles: model.UserRoles{teamID.String(): model.RoleOwner},
		})

		// Сначала вернём задачу в Done
		req := initUpdateReq(t, taskID)
		req.Status = model.StatusDone
		updated, err := taskSvc.Update(ctxOwner, req)
		be.Err(t, err, nil)
		be.Equal(t, updated.Status, model.StatusDone)
		be.True(t, updated.ClosedAt.Valid) // not null
		since := time.Since(updated.ClosedAt.V)
		be.True(t, 0 <= since && since < 100*time.Millisecond)

		// Возвращаем задачу в работу
		req = initUpdateReq(t, taskID)
		req.Status = model.StatusInProgress
		updated, err = taskSvc.Update(ctxOwner, req)
		be.Err(t, err, nil)
		be.Equal(t, updated.Status, model.StatusInProgress)
		be.True(t, !updated.ClosedAt.Valid) // null
	})

	// 3. Обычный участник (не создатель, не исполнитель) не может менять ничего
	t.Run("ordinary member cannot update", func(t *testing.T) {
		req := initUpdateReq(t, taskID)
		req.Status = model.StatusCancelled
		ctxOther := auth.ContextWithUserForTest(ctx, model.User{
			ID:    otherMember,
			Roles: model.UserRoles{teamID.String(): model.RoleMember},
		})
		_, err := taskSvc.Update(ctxOther, req)
		be.Err(t, err, model.ErrForbidden)
	})

	// 4. Создатель может менять всё
	t.Run("creator can update all fields", func(t *testing.T) {
		req := initUpdateReq(t, taskID)
		req.Title = "Creator edit"
		req.Description = "new desc"
		req.Status = model.StatusTodo
		req.AssigneeID = Val(otherMember)
		ctxCreator := auth.ContextWithUserForTest(ctx, model.User{
			ID:    creator,
			Roles: model.UserRoles{teamID.String(): model.RoleMember},
		})
		updated, err := taskSvc.Update(ctxCreator, req)
		be.Err(t, err, nil)
		be.Equal(t, updated.Title, "Creator edit")
		be.Equal(t, updated.Description, "new desc")
		be.Equal(t, updated.Status, model.StatusTodo)
		be.Equal(t, updated.AssigneeID.V, otherMember)
	})

	// 5. Администратор может менять всё
	t.Run("admin can update all fields", func(t *testing.T) {
		req := initUpdateReq(t, taskID)
		req.Title = "Admin edit"
		req.Description = "admin desc"
		req.Status = model.StatusDone
		req.AssigneeID = Val(assignee)
		ctxAdmin := auth.ContextWithUserForTest(ctx, model.User{
			ID:    admin,
			Roles: model.UserRoles{teamID.String(): model.RoleAdmin},
		})
		updated, err := taskSvc.Update(ctxAdmin, req)
		be.Err(t, err, nil)
		be.Equal(t, updated.Title, "Admin edit")
		be.Equal(t, updated.Description, "admin desc")
		be.Equal(t, updated.Status, model.StatusDone)
		be.Equal(t, updated.AssigneeID.V, assignee)
	})
}
