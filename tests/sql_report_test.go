package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"aaa2ppp/teams-tasks/internal/features/teams"
	"aaa2ppp/teams-tasks/internal/model"

	"github.com/aaa2ppp/be"
	_ "github.com/go-sql-driver/mysql"
)

// TestSQLReport - интеграционный тест для SQL-отчёта /teams/{id}/stats.
// Проверяет корректность метрик: статусы задач, топ-3 исполнителей по закрытым задачам за 30 дней,
// среднее время закрытия, общее количество комментариев.
func TestSQLReport(t *testing.T) {
	ctx := context.Background()
	db, cleanup := StartTestDatabase(t)
	defer cleanup()

	// Подготовка хранилища и транзактора.
	teamStorage := teams.NewStorage(db)

	userOwnerID := InsertUser(t, db, "owner@test.com", "Owner", "hash")
	userAdminID := InsertUser(t, db, "admin@test.com", "Admin", "hash")
	userMemberID := InsertUser(t, db, "member@test.com", "Member", "hash")
	userOutsideID := InsertUser(t, db, "outside@test.com", "Outside", "hash")

	// Создание команды.
	teamID := InsertTeam(t, db, "Test Team", userOwnerID)

	// Добавление участников.
	roles := map[model.UserID]model.Role{
		userOwnerID:  model.RoleOwner,
		userAdminID:  model.RoleAdmin,
		userMemberID: model.RoleMember,
	}
	AddMember(t, db, teamID, userOwnerID, model.RoleOwner)
	AddMember(t, db, teamID, userAdminID, model.RoleAdmin)
	AddMember(t, db, teamID, userMemberID, model.RoleMember)

	now := time.Now()
	tenDaysAgo := now.Add(-10 * 24 * time.Hour)
	twentyDaysAgo := now.Add(-20 * 24 * time.Hour)
	thirtyOneDaysAgo := now.Add(-31 * 24 * time.Hour)
	fiveDaysAgo := now.Add(-5 * 24 * time.Hour)
	twoDaysAgo := now.Add(-2 * 24 * time.Hour)

	// Задачи:
	// 1: done, assignee=admin, закрыта 10 дней назад
	CreateTask(t, db, teamID, "Task 1", "desc1", model.StatusDone, userOwnerID, &userAdminID, tenDaysAgo, &tenDaysAgo)
	// 2: done, assignee=member, закрыта 20 дней назад
	CreateTask(t, db, teamID, "Task 2", "desc2", model.StatusDone, userOwnerID, &userMemberID, twentyDaysAgo, &twentyDaysAgo)
	// 3: done, assignee=admin, закрыта 5 дней назад
	CreateTask(t, db, teamID, "Task 3", "desc3", model.StatusDone, userOwnerID, &userAdminID, fiveDaysAgo, &fiveDaysAgo)
	// 4: done, assignee=admin, закрыта 31 день назад (не входит в топ за 30 дней)
	CreateTask(t, db, teamID, "Task 4", "desc4", model.StatusDone, userOwnerID, &userAdminID, thirtyOneDaysAgo, &thirtyOneDaysAgo)
	// 5: todo
	CreateTask(t, db, teamID, "Task 5", "desc5", model.StatusTodo, userOwnerID, &userMemberID, now, nil)
	// 6: in_progress
	CreateTask(t, db, teamID, "Task 6", "desc6", model.StatusInProgress, userOwnerID, &userAdminID, now, nil)
	// 7: done, assignee=owner, закрыта 2 дня назад
	CreateTask(t, db, teamID, "Task 7", "desc7", model.StatusDone, userOwnerID, &userOwnerID, twoDaysAgo, &twoDaysAgo)

	// Комментарии: к задачам 1,2,3 (по одному), и дополнительный к задаче 1.
	for i := 1; i <= 3; i++ {
		AddComment(t, db, model.TaskID(i), userMemberID, fmt.Sprintf("comment %d", i), &now)
	}
	AddComment(t, db, 1, userAdminID, "admin comment", &now)

	// Вызов GenReport для пользователя с правами
	for _, userID := range []model.UserID{userOwnerID, userAdminID} {
		t.Run(roles[userID].String(), func(t *testing.T) {
			reportReq := teams.DBGenReportReq{
				TeamID:    teamID,
				CurUserID: userID,
			}
			metrics, err := teamStorage.GenReport(ctx, reportReq)
			be.Err(t, err, nil)

			// Проверка метрик.
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
		})
	}

	// Проверка доступа пользователь без прав
	for _, userID := range []model.UserID{userMemberID, userOutsideID} {
		t.Run(roles[userID].String(), func(t *testing.T) {
			reportReqOutside := teams.DBGenReportReq{
				TeamID:    teamID,
				CurUserID: userID,
			}
			_, err := teamStorage.GenReport(ctx, reportReqOutside)
			be.Err(t, err, model.ErrForbidden)
		})
	}
}
