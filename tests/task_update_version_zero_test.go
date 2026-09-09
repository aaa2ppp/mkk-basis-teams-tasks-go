// tests/task_update_version_zero_test.go
package tests

import (
	"context"
	"testing"
	"time"

	"aaa2ppp/teams-tasks/internal/db"
	"aaa2ppp/teams-tasks/internal/features/tasks"
	"aaa2ppp/teams-tasks/internal/lib/auth"
	"aaa2ppp/teams-tasks/internal/model"

	"github.com/aaa2ppp/be"
)

// testTaskUpdateVersionZero доказывает, что сервис отклоняет обновление
// с Version=0, даже если вызов идёт напрямую (минуя HTTP/API-валидатор).
// Это страхует от регрессии: если кто-то уберёт проверку из apiUpdateReq.Validate(),
// сервис всё равно останется защищённым.
func testTaskUpdateVersionZero(t *testing.T, db *db.DB) {
	ctx := context.Background()

	storage := tasks.NewStorage(db)
	svc := tasks.NewService(storage, db, &NoopCache{})

	owner := InsertUser(t, db, "owner@test.com", "Owner", "hash")
	teamID := InsertTeam(t, db, "Team", owner)
	AddMember(t, db, teamID, owner, model.RoleOwner)

	taskID := CreateTask(t, db, teamID, "Task", "desc",
		model.StatusTodo, owner, nil, time.Now(), nil)

	// Проверяем, что начальная версия в БД == 1 (инвариант схемы)
	task, err := storage.GetByID(ctx, tasks.DBGetByIDReq{TaskID: taskID})
	be.Err(t, err, nil)
	be.Equal(t, task.Version, int64(1))

	ownerCtx := auth.ContextWithUserForTest(ctx, model.User{
		ID:    owner,
		Roles: model.UserRoles{teamID.String(): model.RoleOwner},
	})

	// Попытка обновить задачу с Version=0 (минуя API-валидатор)
	req := tasks.SvcUpdateReq{
		TaskID:  taskID,
		Title:   "Hacked title",
		Status:  model.StatusDone,
		Version: 0, // ← ключевое: ноль
	}

	_, err = svc.Update(ownerCtx, req)

	// Ожидаем ErrConflict, а не успешное обновление
	be.Err(t, err, model.ErrConflict)

	// Убеждаемся, что задача НЕ изменилась
	taskAfter, err := storage.GetByID(ctx, tasks.DBGetByIDReq{TaskID: taskID})
	be.Err(t, err, nil)
	be.Equal(t, taskAfter.Title, "Task")            // не "Hacked title"
	be.Equal(t, taskAfter.Status, model.StatusTodo) // не Done
	be.Equal(t, taskAfter.Version, int64(1))        // версия не инкрементировалась
}
