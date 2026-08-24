package tests

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"testing"
	"time"

	"aaa2ppp/teams-tasks/internal/features/teams"
	"aaa2ppp/teams-tasks/internal/model"

	"github.com/aaa2ppp/be"
	_ "github.com/go-sql-driver/mysql"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestSQLReport - интеграционный тест для SQL-отчёта /teams/{id}/stats.
// Проверяет корректность метрик: статусы задач, топ-3 исполнителей по закрытым задачам за 30 дней,
// среднее время закрытия, общее количество комментариев.
func TestSQLReport(t *testing.T) {
	ctx := context.Background()
	containerLogger := log.New(io.Discard, "", 0)

	// 1. Запуск MariaDB в контейнере через testcontainers.
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

	// 2. Применение миграций.
	be.Err(t, goose.SetDialect("mysql"), nil)
	be.Err(t, goose.Up(sqlDB, "../migrations"), nil)

	// 3. Подготовка хранилища и транзактора.
	//transactor := db.NewTransactor(sqlDB)
	teamStorage := teams.NewStorage(sqlDB)

	// 4. Вставка тестовых данных (прямые SQL-запросы для простоты).
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

	userOwnerID, err := insertUser("owner@test.com", "Owner", "hash")
	be.Err(t, err, nil)
	userAdminID, err := insertUser("admin@test.com", "Admin", "hash")
	be.Err(t, err, nil)
	userMemberID, err := insertUser("member@test.com", "Member", "hash")
	be.Err(t, err, nil)
	userOutsideID, err := insertUser("outside@test.com", "Outside", "hash")
	be.Err(t, err, nil)

	// Создание команды.
	teamRes, err := sqlDB.ExecContext(ctx,
		"INSERT INTO teams (name, created_by) VALUES (?, ?)",
		"Test Team", userOwnerID)
	be.Err(t, err, nil)
	teamID64, _ := teamRes.LastInsertId()
	teamID := model.TeamID(teamID64)

	// Добавление участников.
	addMember := func(teamID model.TeamID, userID model.UserID, role model.Role) error {
		_, err := sqlDB.ExecContext(ctx,
			"INSERT INTO team_members (team_id, user_id, role) VALUES (?, ?, ?)",
			teamID, userID, role.String())
		return err
	}
	be.Err(t, addMember(teamID, userOwnerID, model.RoleOwner), nil)
	be.Err(t, addMember(teamID, userAdminID, model.RoleAdmin), nil)
	be.Err(t, addMember(teamID, userMemberID, model.RoleMember), nil)

	now := time.Now()
	tenDaysAgo := now.Add(-10 * 24 * time.Hour)
	twentyDaysAgo := now.Add(-20 * 24 * time.Hour)
	thirtyOneDaysAgo := now.Add(-31 * 24 * time.Hour)
	fiveDaysAgo := now.Add(-5 * 24 * time.Hour)
	twoDaysAgo := now.Add(-2 * 24 * time.Hour)

	createTask := func(title, desc string, status model.Status, createdBy, assignee model.UserID, createdAt, closedAt *time.Time) error {
		_, err := sqlDB.ExecContext(ctx,
			"INSERT INTO tasks (team_id, title, description, status, created_by, assignee_id, created_at, closed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			teamID, title, desc, status, createdBy, assignee, createdAt, closedAt)
		return err
	}

	// Задачи:
	// 1: done, assignee=admin, закрыта 10 дней назад
	be.Err(t, createTask("Task 1", "desc1", model.StatusDone, userOwnerID, userAdminID, &tenDaysAgo, &tenDaysAgo), nil)
	// 2: done, assignee=member, закрыта 20 дней назад
	be.Err(t, createTask("Task 2", "desc2", model.StatusDone, userOwnerID, userMemberID, &twentyDaysAgo, &twentyDaysAgo), nil)
	// 3: done, assignee=admin, закрыта 5 дней назад
	be.Err(t, createTask("Task 3", "desc3", model.StatusDone, userOwnerID, userAdminID, &fiveDaysAgo, &fiveDaysAgo), nil)
	// 4: done, assignee=admin, закрыта 31 день назад (не входит в топ за 30 дней)
	be.Err(t, createTask("Task 4", "desc4", model.StatusDone, userOwnerID, userAdminID, &thirtyOneDaysAgo, &thirtyOneDaysAgo), nil)
	// 5: todo
	be.Err(t, createTask("Task 5", "desc5", model.StatusTodo, userOwnerID, userMemberID, &now, nil), nil)
	// 6: in_progress
	be.Err(t, createTask("Task 6", "desc6", model.StatusInProgress, userOwnerID, userAdminID, &now, nil), nil)
	// 7: done, assignee=owner, закрыта 2 дня назад
	be.Err(t, createTask("Task 7", "desc7", model.StatusDone, userOwnerID, userOwnerID, &twoDaysAgo, &twoDaysAgo), nil)

	// Комментарии: к задачам 1,2,3 (по одному), и дополнительный к задаче 1.
	addComment := func(taskID model.TaskID, userID model.UserID, content string, createdAt *time.Time) error {
		_, err := sqlDB.ExecContext(ctx,
			"INSERT INTO task_comments (task_id, user_id, content, created_at) VALUES (?, ?, ?, ?)",
			taskID, userID, content, createdAt)
		return err
	}
	for i := 1; i <= 3; i++ {
		be.Err(t, addComment(model.TaskID(i), userMemberID, fmt.Sprintf("comment %d", i), &now), nil)
	}
	be.Err(t, addComment(1, userAdminID, "admin comment", &now), nil)

	// 5. Вызов GenReport для пользователя с правами (owner).
	reportReq := teams.DBGenReportReq{
		TeamID:    teamID,
		CurUserID: userOwnerID,
	}
	metrics, err := teamStorage.GenReport(ctx, reportReq)
	be.Err(t, err, nil)

	// 6. Проверка метрик.
	metricsMap := make(map[string]map[string]float64) // type -> detail -> value
	for _, m := range metrics {
		if _, ok := metricsMap[m.Type]; !ok {
			metricsMap[m.Type] = make(map[string]float64)
		}
		metricsMap[m.Type][m.Detail] = m.Value.V
	}

	// status_stats: ожидаем 3 статуса: done (5 задач), todo (1), in_progress (1)
	statusStats := metricsMap["status_stats"]
	be.Equal(t, len(statusStats), 3)
	be.Equal(t, statusStats["done"], 5)
	be.Equal(t, statusStats["todo"], 1)
	be.Equal(t, statusStats["in_progress"], 1)

	// top3_assignees: ожидаем 3 записи с количествами 2,1,1 (admin=2, member=1, owner=1)
	top3 := metricsMap["top3_assignees"]
	if len(top3) != 3 {
		t.Errorf("expected 3 top3 assignees, got %d", len(top3))
	}
	counts := []float64{}
	for _, v := range top3 {
		counts = append(counts, v)
	}
	found1, found2 := 0, 0
	for _, v := range counts {
		switch v {
		case 1:
			found1++
		case 2:
			found2++
		}
	}
	be.Equal(t, found1, 2)
	be.Equal(t, found2, 1)

	// avg_close_days: одна запись со значением > 0
	avgClose := metricsMap["avg_close_days"]
	be.Equal(t, len(avgClose), 1)
	be.Equal(t, avgClose["all"], 1)

	// total_comments: всего 4 комментария
	totalComments := metricsMap["total_comments"]
	be.Equal(t, len(totalComments), 1)
	be.Equal(t, totalComments[""], 4)

	// 7. Проверка доступа: пользователь не участник команды должен получить ошибку.
	reportReqOutside := teams.DBGenReportReq{
		TeamID:    teamID,
		CurUserID: userOutsideID,
	}
	_, err = teamStorage.GenReport(ctx, reportReqOutside)
	be.Err(t, err, model.ErrForbidden)
}
