package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"aaa2ppp/teams-tasks/internal/features/tasks"
	"aaa2ppp/teams-tasks/internal/lib/auth"
	"aaa2ppp/teams-tasks/internal/model"

	"github.com/aaa2ppp/be"
	_ "github.com/go-sql-driver/mysql"
)

// TestTaskUpdate проверяет права доступа, версионность и историю при обновлении задачи.
func TestTaskUpdate(t *testing.T) {
	ctx := context.Background()
	db, cleanup := StartTestDatabase(t)
	defer cleanup()

	// Хранилище, транзактор, сервис (кеш — заглушка)
	taskStorage := tasks.NewStorage(db)
	cache := &NoopCache{}
	taskService := tasks.NewService(taskStorage, db, cache)

	// Тестовые данные
	userOwner := InsertUser(t, db, "owner@test.com", "Owner", "hash")
	userAdmin := InsertUser(t, db, "admin@test.com", "Admin", "hash")
	userCreator := InsertUser(t, db, "creator@test.com", "Creator", "hash")
	userOther1 := InsertUser(t, db, "other1@test.com", "Other1", "hash")
	userOther2 := InsertUser(t, db, "other2@test.com", "Other2", "hash")
	userOutside := InsertUser(t, db, "outside@test.com", "Outside", "hash")

	teamID := InsertTeam(t, db, "Test Team", userOwner)
	AddMember(t, db, teamID, userOwner, model.RoleOwner)
	AddMember(t, db, teamID, userAdmin, model.RoleAdmin)
	AddMember(t, db, teamID, userCreator, model.RoleMember)
	AddMember(t, db, teamID, userOther1, model.RoleMember)
	AddMember(t, db, teamID, userOther2, model.RoleMember)

	now := time.Now()
	taskID := CreateTask(t, db, teamID, "Test Task", "Initial desc", model.StatusTodo, userCreator, nil, now, nil)

	// Текущая версия
	task, err := taskStorage.GetByID(ctx, tasks.DBGetByIDReq{TaskID: taskID})
	be.Err(t, err, nil)
	currentVersion := task.Version

	// Сценарии обновления
	tests := []struct {
		name    string
		userID  model.UserID
		req     tasks.SvcUpdateReq
		wantErr error
		checkFn func(t *testing.T, task model.Task)
	}{
		{
			name:   "owner может изменить все",
			userID: userOwner,
			req: tasks.SvcUpdateReq{
				TaskID:      taskID,
				Title:       "Updated by owner",
				Description: "New description",
				Status:      model.StatusInProgress,
				AssigneeID:  Val(userOther1),
				Version:     currentVersion,
			},
			wantErr: nil,
			checkFn: func(t *testing.T, task model.Task) {
				be.Equal(t, task.Title, "Updated by owner")
				be.Equal(t, task.Description, "New description")
				be.Equal(t, task.Status, model.StatusInProgress)
				be.Equal(t, task.AssigneeID.V, userOther1)
				be.Equal(t, task.Version, currentVersion+1)
			},
		},
		{
			name:   "owner НЕ может назначить пользователя из другой команды",
			userID: userOwner,
			req: tasks.SvcUpdateReq{
				TaskID:      taskID,
				Title:       "Updated by owner",
				Description: "New description",
				Status:      model.StatusInProgress,
				AssigneeID:  Val(userOutside),
				Version:     currentVersion + 1,
			},
			wantErr: model.ErrForbidden,
		},
		{
			name:   "admin может изменить все",
			userID: userAdmin,
			req: tasks.SvcUpdateReq{
				TaskID:      taskID,
				Title:       "Updated by admin",
				Description: "Admin desc",
				Status:      model.StatusDone,
				AssigneeID:  Val(userOther2),
				Version:     currentVersion + 1,
			},
			wantErr: nil,
			checkFn: func(t *testing.T, task model.Task) {
				be.Equal(t, task.Title, "Updated by admin")
				be.Equal(t, task.Description, "Admin desc")
				be.Equal(t, task.Status, model.StatusDone)
				be.Equal(t, task.AssigneeID.V, userOther2)
				be.Equal(t, task.Version, currentVersion+2)
			},
		},
		{
			name:   "admin НЕ может назначить пользователя из другой команды",
			userID: userAdmin,
			req: tasks.SvcUpdateReq{
				TaskID:      taskID,
				Title:       "Updated by admin",
				Description: "Admin desc",
				Status:      model.StatusDone,
				AssigneeID:  Val(userOutside),
				Version:     currentVersion + 2,
			},
			wantErr: model.ErrForbidden,
		},
		{
			name:   "creator может изменить все",
			userID: userCreator,
			req: tasks.SvcUpdateReq{
				TaskID:      taskID,
				Title:       "Creator update",
				Description: "Creator desc",
				Status:      model.StatusTodo,
				AssigneeID:  Val(userOther1),
				Version:     currentVersion + 2,
			},
			wantErr: nil,
			checkFn: func(t *testing.T, task model.Task) {
				be.Equal(t, task.Title, "Creator update")
				be.Equal(t, task.Description, "Creator desc")
				be.Equal(t, task.Status, model.StatusTodo)
				be.Equal(t, task.AssigneeID.V, userOther1)
				be.Equal(t, task.Version, currentVersion+3)
			},
		},
		{
			name:   "creator НЕ может назначить пользователя из другой команды",
			userID: userCreator,
			req: tasks.SvcUpdateReq{
				TaskID:      taskID,
				Title:       "Creator update",
				Description: "Creator desc",
				Status:      model.StatusTodo,
				AssigneeID:  Val(userOutside),
				Version:     currentVersion + 3,
			},
			wantErr: model.ErrForbidden,
		},
		{
			name:   "assignee может изменить статус",
			userID: userOther1, // текущий assignee
			req: tasks.SvcUpdateReq{
				TaskID:      taskID,
				Title:       "Creator update",
				Description: "Creator desc",
				Status:      model.StatusDone,
				AssigneeID:  Val(userOther1),
				Version:     currentVersion + 3,
			},
			wantErr: nil,
			checkFn: func(t *testing.T, task model.Task) {
				be.Equal(t, task.Title, "Creator update")
				be.Equal(t, task.Description, "Creator desc")
				be.Equal(t, task.Status, model.StatusDone)
				be.Equal(t, task.AssigneeID.V, userOther1)
				be.Equal(t, task.Version, currentVersion+4)
			},
		},
		{
			name:   "assignee НЕ может менять title",
			userID: userOther1, // текущий assignee = userOwner
			req: tasks.SvcUpdateReq{
				TaskID:      taskID,
				Title:       "Assignee update",
				Description: "Creator desc",
				Status:      model.StatusDone,
				AssigneeID:  Val(userOther1),
				Version:     currentVersion + 4,
			},
			wantErr: model.ErrForbidden,
		},
		{
			name:   "assignee НЕ может менять Description",
			userID: userOther1, // текущий assignee = userOwner
			req: tasks.SvcUpdateReq{
				TaskID:      taskID,
				Title:       "Creator update",
				Description: "Assignee desc",
				Status:      model.StatusDone,
				AssigneeID:  Val(userOther1),
				Version:     currentVersion + 4,
			},
			wantErr: model.ErrForbidden,
		},
		{
			name:   "assignee НЕ может переназначить задачу",
			userID: userOther1, // текущий assignee = userOwner
			req: tasks.SvcUpdateReq{
				TaskID:      taskID,
				Title:       "Creator update",
				Description: "Creator desc",
				Status:      model.StatusDone,
				AssigneeID:  Val(userOther2), // попытка сменить исполнителя
				Version:     currentVersion + 4,
			},
			wantErr: model.ErrForbidden,
		},
		{
			name:   "простой member НЕ может менять title",
			userID: userOther2,
			req: tasks.SvcUpdateReq{
				TaskID:      taskID,
				Title:       "Other update",
				Description: "Creator desc",
				Status:      model.StatusDone,
				AssigneeID:  Val(userOther1),
				Version:     currentVersion + 4,
			},
			wantErr: model.ErrForbidden,
		},
		{
			name:   "простой member НЕ может менять Description",
			userID: userOther2,
			req: tasks.SvcUpdateReq{
				TaskID:      taskID,
				Title:       "Creator update",
				Description: "Other desc",
				Status:      model.StatusDone,
				AssigneeID:  Val(userOther1),
				Version:     currentVersion + 4,
			},
			wantErr: model.ErrForbidden,
		},
		{
			name:   "простой member НЕ может менять Status",
			userID: userOther2,
			req: tasks.SvcUpdateReq{
				TaskID:      taskID,
				Title:       "Creator update",
				Description: "Creator desc",
				Status:      model.StatusCancelled,
				AssigneeID:  Val(userOther1),
				Version:     currentVersion + 4,
			},
			wantErr: model.ErrForbidden,
		},
		{
			name:   "простой member НЕ может переназначить задачу",
			userID: userOther2,
			req: tasks.SvcUpdateReq{
				TaskID:      taskID,
				Title:       "Creator update",
				Description: "Creator desc",
				Status:      model.StatusDone,
				AssigneeID:  Val(userOther2),
				Version:     currentVersion + 4,
			},
			wantErr: model.ErrForbidden,
		},
		{
			name:   "не member НЕ ВИДИТ задачу",
			userID: userOutside,
			req: tasks.SvcUpdateReq{
				TaskID:      taskID,
				Title:       "Outside update",
				Description: "Creator desc",
				Status:      model.StatusDone,
				AssigneeID:  Val(userOther1),
				Version:     currentVersion + 4,
			},
			wantErr: model.ErrNotFound,
		},
		{
			name:   "конкурентное обновление - 409 Conflict",
			userID: userOwner,
			req: tasks.SvcUpdateReq{
				TaskID:  taskID,
				Title:   "Conflict test",
				Status:  model.StatusDone,
				Version: currentVersion, // старая версия
			},
			wantErr: model.ErrConflict,
		},
	}

	// Выполняем тесты последовательно
	currentTask := task
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.req
			if req.Version == 0 {
				req.Version = currentTask.Version
			}

			user := model.User{ID: tt.userID}
			updated, err := taskService.Update(
				auth.ContextWithUserForTest(ctx, user),
				req,
			)

			if tt.wantErr != nil {
				be.Err(t, err, tt.wantErr)
				if errors.Is(err, model.ErrConflict) {
					// при конфликте возвращается актуальная запись
					be.Equal(t, updated.Version, currentTask.Version)
				}
				return
			}

			be.Err(t, err, nil)
			if tt.checkFn != nil {
				tt.checkFn(t, updated)
			}
			currentTask = updated

			// Проверяем историю
			history, err := taskStorage.GetHistory(ctx, []model.TaskID{taskID})
			be.Err(t, err, nil)
			be.True(t, len(history) > 0)
			last := history[len(history)-1]
			be.Equal(t, last.ChangedBy, tt.userID)
			be.True(t, len(last.Changes) > 0)
		})
	}
}
