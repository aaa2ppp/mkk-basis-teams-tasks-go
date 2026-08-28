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

// TestTaskPagination проверяет пагинацию списка задач: limit, cursor, next_cursor.
func TestTaskPagination(t *testing.T) {
	ctx := context.Background()
	db, cleanup := StartTestDatabase(t)
	defer cleanup()

	// Подготовка хранилища и сервиса с заглушкой кеша
	taskStorage := tasks.NewStorage(db)
	cache := &NoopCache{}
	taskService := tasks.NewService(taskStorage, db, cache)

	// Создаем пользователя, команду и добавляем польхователя в команду
	user := InsertUser(t, db, "user@test.com", "User", "hash")
	teamID := InsertTeam(t, db, "Test Team", user)
	AddMember(t, db, teamID, user, model.RoleMember)

	// Контекст пользователя
	userCtx := auth.ContextWithUserForTest(ctx, model.User{
		ID: user,
		TeamRoles: map[string]model.Role{
			teamID.String(): model.RoleMember,
		},
	})

	// Создаем 10 задач с разными статусами, чтобы проверить фильтры
	taskIDs := make([]model.TaskID, 10)
	for i := 0; i < 10; i++ {
		status := model.StatusTodo
		if i%2 == 0 {
			status = model.StatusDone
		}
		taskIDs[i] = CreateTask(t, db, teamID,
			"Task "+string(byte('A'+i)),
			"desc",
			status,
			user,
			nil,
			time.Time{}, nil)
	}

	// ---- Тест 1: пагинация без фильтров, limit=3 ----
	t.Run("basic pagination limit=3", func(t *testing.T) {
		req := tasks.SvcListReq{
			TeamID: teamID,
			Limit:  3,
		}
		resp, err := taskService.List(userCtx, req)
		be.Err(t, err, nil)

		// Должно вернуть 3 задачи, next_cursor = ID 4-й задачи (сортировка по ID)
		be.Equal(t, len(resp.Tasks), 3)
		be.Equal(t, resp.NextCursor, taskIDs[3]) // 4-я задача (индекс 3)

		// Проверяем, что задачи идут по возрастанию ID
		be.Equal(t, resp.Tasks[0].ID, taskIDs[0])
		be.Equal(t, resp.Tasks[1].ID, taskIDs[1])
		be.Equal(t, resp.Tasks[2].ID, taskIDs[2])
	})

	// ---- Тест 2: следующая страница с cursor ----
	t.Run("pagination with cursor", func(t *testing.T) {
		req := tasks.SvcListReq{
			TeamID: teamID,
			Cursor: taskIDs[3], // начиная с 4-й задачи
			Limit:  3,
		}
		resp, err := taskService.List(userCtx, req)
		be.Err(t, err, nil)

		// Должно вернуть задачи 4,5,6 (индексы 3,4,5) и next_cursor = ID 7-й (индекс 6)
		be.Equal(t, len(resp.Tasks), 3)
		be.Equal(t, resp.NextCursor, taskIDs[6])

		// Проверяем ID: первая должна быть taskIDs[3]
		be.Equal(t, resp.Tasks[0].ID, taskIDs[3])
		be.Equal(t, resp.Tasks[1].ID, taskIDs[4])
		be.Equal(t, resp.Tasks[2].ID, taskIDs[5])
	})

	// ---- Тест 3: последняя страница (меньше чем limit) ----
	t.Run("last page", func(t *testing.T) {
		req := tasks.SvcListReq{
			TeamID: teamID,
			Cursor: taskIDs[8], // 9-я задача (индекс 8)
			Limit:  3,
		}
		resp, err := taskService.List(userCtx, req)
		be.Err(t, err, nil)

		// Должна быть 2 задачи (индекс 8, 9) и next_cursor = 0 (пусто)
		be.Equal(t, len(resp.Tasks), 2)
		be.Equal(t, resp.NextCursor, model.TaskID(0))

		be.Equal(t, resp.Tasks[0].ID, taskIDs[8])
		be.Equal(t, resp.Tasks[1].ID, taskIDs[9])
	})

	// ---- Тест 4: limit больше общего количества ----
	t.Run("limit greater than total", func(t *testing.T) {
		req := tasks.SvcListReq{
			TeamID: teamID,
			Limit:  20,
		}
		resp, err := taskService.List(userCtx, req)
		be.Err(t, err, nil)

		// Должны вернуть все 10 задач, next_cursor = 0
		be.Equal(t, len(resp.Tasks), 10)
		be.Equal(t, resp.NextCursor, model.TaskID(0))

		for i := 0; i < 10; i++ {
			be.Equal(t, resp.Tasks[i].ID, taskIDs[i])
		}
	})

	// ---- Тест 5: пагинация с фильтром по статусу ----
	t.Run("pagination with status filter", func(t *testing.T) {
		// Все задачи с четным индексом имеют статус done (0,2,4,6,8) -> 5 задач
		req := tasks.SvcListReq{
			TeamID: teamID,
			Status: model.StatusDone,
			Limit:  2,
		}
		resp, err := taskService.List(userCtx, req)
		be.Err(t, err, nil)

		// Должны вернуть первые 2 задачи с done (индексы 0 и 2)
		be.Equal(t, len(resp.Tasks), 2)
		// Следующий курсор должен быть ID задачи с индексом 4 (третья done)
		be.Equal(t, resp.NextCursor, taskIDs[4])

		// Проверяем, что статус у всех задач done
		for _, task := range resp.Tasks {
			be.Equal(t, task.Status, model.StatusDone)
		}
	})

	// ---- Тест 6: пагинация с cursor и фильтром ----
	t.Run("pagination with cursor and filter", func(t *testing.T) {
		// Берем cursor = ID задачи с индексом 4 (это done), лимит 2
		req := tasks.SvcListReq{
			TeamID: teamID,
			Status: model.StatusDone,
			Cursor: taskIDs[4],
			Limit:  2,
		}
		resp, err := taskService.List(userCtx, req)
		be.Err(t, err, nil)

		// Должны получить задачи с индексами 4 и 6 (обе done), next_cursor = ID задачи 8
		be.Equal(t, len(resp.Tasks), 2)
		be.Equal(t, resp.NextCursor, taskIDs[8])
		be.Equal(t, resp.Tasks[0].ID, taskIDs[4])
		be.Equal(t, resp.Tasks[1].ID, taskIDs[6])
		for _, task := range resp.Tasks {
			be.Equal(t, task.Status, model.StatusDone)
		}
	})

	// ---- Тест 7: пустой список (нет задач по фильтру) ----
	t.Run("empty list", func(t *testing.T) {
		req := tasks.SvcListReq{
			TeamID: teamID,
			Status: model.StatusCancelled, // нет таких задач
			Limit:  5,
		}
		resp, err := taskService.List(userCtx, req)
		be.Err(t, err, nil)

		be.Equal(t, len(resp.Tasks), 0)
		be.Equal(t, resp.NextCursor, model.TaskID(0))
	})
}
