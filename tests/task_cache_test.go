package tests

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"aaa2ppp/teams-tasks/internal/features/tasks"
	"aaa2ppp/teams-tasks/internal/lib/auth"
	"aaa2ppp/teams-tasks/internal/model"

	"github.com/aaa2ppp/be"
	"github.com/aaa2ppp/be/tb"
	_ "github.com/go-sql-driver/mysql"
)

// CountingStorage — обёртка над Storage, считающая вызовы List.
type CountingStorage struct {
	tasks.Storage
	mu        sync.Mutex
	listCount int
}

func (c *CountingStorage) List(ctx context.Context, req tasks.DBListReq) ([]model.Task, error) {
	c.mu.Lock()
	c.listCount++
	c.mu.Unlock()
	return c.Storage.List(ctx, req)
}

// TestCache проверяет работу кеша для метода List: кеширование, инвалидация при Create/Update.
func TestCache(t *testing.T) {
	ctx := context.Background()

	// Поднимаем БД и Redis
	db, dbCleanup := StartTestDatabase(t)
	defer dbCleanup()
	redisClient, redisCleanup := StartTestRedis(t)
	defer redisCleanup()

	// Создаём хранилище с обёрткой-счётчиком
	realStorage := tasks.NewStorage(db)
	countingStorage := &CountingStorage{Storage: realStorage}

	// Кеш с TTL 5 минут (для теста достаточно)
	cache := tasks.NewCache(redisClient, 5*time.Minute)

	// Сервис задач
	svc := tasks.NewService(countingStorage, db, cache)

	// Подготовка пользователей и команды
	userOwner := InsertUser(t, db, "owner@cache.test", "Owner", "hash")

	teamID := InsertTeam(t, db, "Cache Team", userOwner)
	AddMember(t, db, teamID, userOwner, model.RoleOwner)

	var members = make([]model.UserID, 5)
	for i := range members {
		member := InsertUser(t, db, fmt.Sprintf("member%d@cache.test", i), fmt.Sprintf("Member%d", i), "hash")
		AddMember(t, db, teamID, member, model.RoleOwner)
	}

	// Создаём несколько задач для теста
	now := time.Now()
	for i := 0; i < 5; i++ {
		status := model.StatusTodo
		if i%2 == 0 {
			status = model.StatusDone
		}
		CreateTask(t, db, teamID,
			"Task "+string(rune('A'+i)),
			"desc",
			status,
			userOwner,
			nil,
			now, nil)
	}

	// Будем вносить изменения в контексте создателя команды (доступно все)
	ownerCtx := auth.ContextWithUserForTest(ctx, model.User{
		ID: userOwner,
		TeamRoles: map[string]model.Role{
			teamID.String(): model.RoleOwner,
		},
	})

	// ------------------------------------------------------------
	// Тест 1: повторный List должен брать данные из кеша (счётчик не увеличивается)
	t.Run("cache hit", func(t *testing.T) {
		req := tasks.SvcListReq{
			TeamID: teamID,
			Limit:  10,
		}
		countingStorage.listCount = 0

		// Первый вызов - поход в базу (увеличивает счетчик)
		resp1, err := svc.List(ownerCtx, req)
		be.Err(t, err, nil)
		be.Equal(t, countingStorage.listCount, 1)

		// Второй вызов - берет данные из кеша (не изменяет счетчик)
		resp2, err := svc.List(ownerCtx, req)
		be.Err(t, err, nil)
		be.Equal(t, countingStorage.listCount, 1)

		// Данные должны совпадать
		be.Equal(tb.Diff(t), resp1, resp2)

		// Проверяем, что принудительная инвалидация работает
		err = tasks.InvalidateTeamCacheForTest(svc, ctx, teamID)
		be.Err(t, err, nil)

		_, err = svc.List(ownerCtx, req)
		be.Err(t, err, nil)
		be.Equal(t, countingStorage.listCount, 2)
	})

	// ------------------------------------------------------------
	// Тест 2: разные параметры запроса — разные ключи кеша, счётчик увеличивается для каждого нового
	t.Run("cache key per params", func(t *testing.T) {
		reqTodo := tasks.SvcListReq{
			TeamID: teamID,
			Status: model.StatusTodo,
			Limit:  10,
		}
		reqDone := tasks.SvcListReq{
			TeamID: teamID,
			Status: model.StatusDone,
			Limit:  10,
		}
		countingStorage.listCount = 0

		err := tasks.InvalidateTeamCacheForTest(svc, ctx, teamID)
		be.Err(t, err, nil)

		// Первый запрос с status=todo - поход в базу (увеличивает счетчик)
		_, err = svc.List(ownerCtx, reqTodo)
		be.Err(t, err, nil)
		be.Equal(t, countingStorage.listCount, 1)

		// Второй запрос с status=todo - из кеша (счетчик не изменяется)
		_, err = svc.List(ownerCtx, reqTodo)
		be.Err(t, err, nil)
		be.Equal(t, countingStorage.listCount, 1)

		// Первый запрос с status=done - поход в базу
		_, err = svc.List(ownerCtx, reqDone)
		be.Err(t, err, nil)
		be.Equal(t, countingStorage.listCount, 2)

		// Второй запрос с status=done - из кеша
		_, err = svc.List(ownerCtx, reqDone)
		be.Err(t, err, nil)
		be.Equal(t, countingStorage.listCount, 2)
	})

	// ------------------------------------------------------------
	// Тест 3: создание задачи инвалидирует кеш команды
	t.Run("invalidate on create", func(t *testing.T) {
		req := tasks.SvcListReq{
			TeamID: teamID,
			Limit:  10,
		}
		countingStorage.listCount = 0

		err := tasks.InvalidateTeamCacheForTest(svc, ctx, teamID)
		be.Err(t, err, nil)

		// Первый List - поход в базу
		_, err = svc.List(ownerCtx, req)
		be.Err(t, err, nil)
		be.Equal(t, countingStorage.listCount, 1)

		// Создаём новую задачу
		createReq := tasks.SvcCreateReq{
			TeamID:      teamID,
			Title:       "New Task",
			Description: "for cache invalidation",
			Status:      model.StatusTodo,
		}
		_, err = svc.Create(ownerCtx, createReq)
		be.Err(t, err, nil)

		// После Create кеш команды должен быть удалён, следующий List пойдёт в БД
		_, err = svc.List(ownerCtx, req)
		be.Err(t, err, nil)
		be.Equal(t, countingStorage.listCount, 2) // второй вызов
	})

	// ------------------------------------------------------------
	// Тест 4: обновление задачи инвалидирует кеш команды
	t.Run("invalidate on update", func(t *testing.T) {
		// Берём первую задачу (она точно есть)
		req := tasks.SvcListReq{
			TeamID: teamID,
			Limit:  1,
		}
		countingStorage.listCount = 0

		err := tasks.InvalidateTeamCacheForTest(svc, ctx, teamID)
		be.Err(t, err, nil)

		resp, err := svc.List(ownerCtx, req)
		be.Err(t, err, nil)
		be.Equal(t, countingStorage.listCount, 1)
		if len(resp.Tasks) == 0 {
			t.Fatal("no tasks found")
		}
		task := resp.Tasks[0]

		// Обновляем задачу (меняем заголовок)
		updateReq := tasks.SvcUpdateReq{
			TaskID:  task.ID,
			Title:   "Updated Title",
			Version: task.Version,
		}
		_, err = svc.Update(ownerCtx, updateReq)
		be.Err(t, err, nil)

		// После Update кеш должен быть удалён
		_, err = svc.List(ownerCtx, req)
		be.Err(t, err, nil)
		be.Equal(t, countingStorage.listCount, 2) // второй вызов
	})

	// ------------------------------------------------------------
	// Тест 5: конкурентные запросы с одним ключом — singleflight + кеш
	// (проверяем, что storage вызывается только один раз даже при параллельных вызовах)
	t.Run("concurrent requests with same key", func(t *testing.T) {
		req := tasks.SvcListReq{
			TeamID: teamID,
			Limit:  10,
		}
		countingStorage.listCount = 0

		err := tasks.InvalidateTeamCacheForTest(svc, ctx, teamID)
		be.Err(t, err, nil)

		// При первом обращении, должен быть только один запрос к базе.
		// При повторных - ни одного (счетчик не должен изменятся).
		for i := 0; i < 2; i++ {
			const concurrency = 10
			var wg sync.WaitGroup
			wg.Add(concurrency)

			for i := 0; i < concurrency; i++ {
				go func() {
					defer wg.Done()
					userCtx := auth.ContextWithUserForTest(ctx, model.User{
						ID: members[i%len(members)],
						TeamRoles: map[string]model.Role{
							teamID.String(): model.RoleMember,
						},
					})
					_, _ = svc.List(userCtx, req)
				}()
			}
			wg.Wait()

			be.Equal(t, countingStorage.listCount, 1)
		}
	})

	t.Run("cache ttl", func(t *testing.T) {
		// Создаём кеш с TTL = 1 секунда
		shortCache := tasks.NewCache(redisClient, 1*time.Second)
		svc = tasks.NewService(countingStorage, db, shortCache)

		req := tasks.SvcListReq{TeamID: teamID, Limit: 10}
		countingStorage.listCount = 0

		err := tasks.InvalidateTeamCacheForTest(svc, ctx, teamID)
		be.Err(t, err, nil)

		// Первый запрос – идёт в БД, счётчик = 1
		_, err = svc.List(ownerCtx, req)
		be.Err(t, err, nil)
		be.Equal(t, countingStorage.listCount, 1)

		// Второй запрос сразу – попадание в кеш, счётчик не меняется
		_, err = svc.List(ownerCtx, req)
		be.Err(t, err, nil)
		be.Equal(t, countingStorage.listCount, 1)

		// Ждём >1 секунды, чтобы кеш протух
		time.Sleep(1100 * time.Millisecond)

		// Третий запрос – кеш пуст, снова идём в БД, счётчик = 2
		_, err = svc.List(ownerCtx, req)
		be.Err(t, err, nil)
		be.Equal(t, countingStorage.listCount, 2)
	})

}
