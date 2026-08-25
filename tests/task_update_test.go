// tests/task_update_test.go
package tests

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"testing"

	"aaa2ppp/teams-tasks/internal/db"
	"aaa2ppp/teams-tasks/internal/features/tasks"
	"aaa2ppp/teams-tasks/internal/lib/auth"
	"aaa2ppp/teams-tasks/internal/model"

	"github.com/aaa2ppp/be"
	_ "github.com/go-sql-driver/mysql"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// helpers для создания Nullable
func Val[T any](v T) model.Nullable[T] {
	return model.Nullable[T]{
		Null:    sql.Null[T]{Valid: true, V: v},
		Defined: true,
	}
}

func Null[T any]() model.Nullable[T] {
	return model.Nullable[T]{
		Defined: true,
	}
}

func Undef[T any]() model.Nullable[T] {
	return model.Nullable[T]{}
}

// TestTaskUpdate проверяет права доступа, версионность и историю при обновлении задачи.
func TestTaskUpdate(t *testing.T) {
	ctx := context.Background()
	containerLogger := log.New(io.Discard, "", 0)

	// 1. Запуск MariaDB
	req := testcontainers.ContainerRequest{
		Image:        "mariadb:12.3.2-noble",
		ExposedPorts: []string{"3306/tcp"},
		Env: map[string]string{
			"MYSQL_ROOT_PASSWORD": "testroot",
			"MYSQL_DATABASE":      "testdb",
			"MYSQL_USER":          "testuser",
			"MYSQL_PASSWORD":      "testpass",
		},
		WaitingFor: wait.ForLog("ready for connections").WithOccurrence(2),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
		Logger:           containerLogger,
	})
	be.Err(t, err, nil)
	defer container.Terminate(ctx)

	host, err := container.Host(ctx)
	be.Err(t, err, nil)
	port, err := container.MappedPort(ctx, "3306/tcp")
	be.Err(t, err, nil)

	dsn := fmt.Sprintf("testuser:testpass@tcp(%s:%s)/testdb?parseTime=true", host, port.Port())
	sqlDB, err := sql.Open("mysql", dsn)
	be.Err(t, err, nil)
	defer sqlDB.Close()

	// 2. Миграции
	be.Err(t, goose.SetDialect("mysql"), nil)
	be.Err(t, goose.Up(sqlDB, "../migrations"), nil)

	// 3. Хранилище, транзактор, сервис (кеш — заглушка)
	taskStorage := tasks.NewStorage(sqlDB)
	transactor := db.NewTransactor(sqlDB)
	cache := &noopCache{}
	taskService := tasks.NewService(taskStorage, transactor, cache)

	// 4. Тестовые данные
	insertUser := func(email, name, pass string) (model.UserID, error) {
		res, err := sqlDB.ExecContext(ctx,
			"INSERT INTO users (email, name, password_hash) VALUES (?, ?, ?)",
			email, name, pass)
		if err != nil {
			return 0, err
		}
		id, _ := res.LastInsertId()
		return model.UserID(id), nil
	}

	userOwner, err := insertUser("owner@test.com", "Owner", "hash")
	be.Err(t, err, nil)
	userAdmin, err := insertUser("admin@test.com", "Admin", "hash")
	be.Err(t, err, nil)
	userCreator, err := insertUser("creator@test.com", "Creator", "hash")
	be.Err(t, err, nil)
	userOther1, err := insertUser("other1@test.com", "Other1", "hash")
	be.Err(t, err, nil)
	userOther2, err := insertUser("other2@test.com", "Other2", "hash")
	be.Err(t, err, nil)
	userOutside, err := insertUser("outside@test.com", "Outside", "hash")
	be.Err(t, err, nil)

	teamRes, err := sqlDB.ExecContext(ctx,
		"INSERT INTO teams (name, created_by) VALUES (?, ?)",
		"Test Team", userOwner)
	be.Err(t, err, nil)
	teamID64, _ := teamRes.LastInsertId()
	teamID := model.TeamID(teamID64)

	addMember := func(teamID model.TeamID, userID model.UserID, role model.Role) error {
		_, err := sqlDB.ExecContext(ctx,
			"INSERT INTO team_members (team_id, user_id, role) VALUES (?, ?, ?)",
			teamID, userID, role.String())
		return err
	}
	be.Err(t, addMember(teamID, userOwner, model.RoleOwner), nil)
	be.Err(t, addMember(teamID, userAdmin, model.RoleAdmin), nil)
	be.Err(t, addMember(teamID, userCreator, model.RoleMember), nil)
	be.Err(t, addMember(teamID, userOther1, model.RoleMember), nil)
	be.Err(t, addMember(teamID, userOther2, model.RoleMember), nil)

	// Создаем задачу (creator = userMember, assignee = userMember)
	createTask := func(teamID model.TeamID, title, desc string, status model.Status,
		createdBy, assignee *model.UserID) (model.TaskID, error) {
		res, err := sqlDB.ExecContext(ctx,
			`INSERT INTO tasks (team_id, title, description, status, created_by, assignee_id)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			teamID, title, desc, status, createdBy, assignee)
		if err != nil {
			return 0, err
		}
		id, _ := res.LastInsertId()
		return model.TaskID(id), nil
	}
	taskID, err := createTask(teamID, "Test Task", "Initial desc", model.StatusTodo, &userCreator, nil)
	be.Err(t, err, nil)

	// Текущая версия
	task, err := taskStorage.GetByID(ctx, tasks.DBGetByIDReq{TaskID: taskID})
	be.Err(t, err, nil)
	currentVersion := task.Version

	// 5. Сценарии обновления
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
				contextWithUser(ctx, user),
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

// --- Заглушки и хелперы ---

type noopCache struct{}

func (c *noopCache) Get(ctx context.Context, key string, val any) error {
	return model.ErrNotFound
}
func (c *noopCache) Put(ctx context.Context, key string, val any) error {
	return nil
}
func (c *noopCache) Invalidate(ctx context.Context, key string) error {
	return nil
}

var contextWithUser = auth.ContextWithUserForTest
