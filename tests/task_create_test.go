package tests

import (
	"context"
	"testing"

	"aaa2ppp/teams-tasks/internal/features/tasks"
	"aaa2ppp/teams-tasks/internal/lib/auth"
	"aaa2ppp/teams-tasks/internal/model"

	"github.com/aaa2ppp/be"
	_ "github.com/go-sql-driver/mysql"
)

// TestTaskCreate проверяет, что создавать задачи могут только участники команды,
// и что назначенный исполнитель должен быть участником.
func TestTaskCreate(t *testing.T) {
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

	t.Run("member can create task", func(t *testing.T) {
		req := tasks.SvcCreateReq{
			TeamID:      teamID,
			Title:       "Task from member",
			Description: "desc",
			Status:      model.StatusTodo,
			AssigneeID:  Undef[model.UserID](),
		}
		ctxMember := auth.ContextWithUserForTest(ctx, model.User{
			ID: member,
			Roles: model.UserRoles{
				teamID.String(): model.RoleMember,
			},
		})
		task, err := taskSvc.Create(ctxMember, req)
		be.Err(t, err, nil)
		be.Equal(t, task.Title, "Task from member")
		be.Equal(t, task.CreatedBy, member)
		be.Equal(t, task.TeamID, teamID)
	})

	t.Run("owner can create task with assignee from team", func(t *testing.T) {
		req := tasks.SvcCreateReq{
			TeamID:      teamID,
			Title:       "Task with assignee",
			Description: "desc",
			Status:      model.StatusTodo,
			AssigneeID:  Val(member),
		}
		ctxOwner := auth.ContextWithUserForTest(ctx, model.User{
			ID: owner,
			Roles: model.UserRoles{
				teamID.String(): model.RoleOwner,
			},
		})
		task, err := taskSvc.Create(ctxOwner, req)
		be.Err(t, err, nil)
		be.Equal(t, task.AssigneeID.V, member)
	})

	t.Run("assignee must be team member", func(t *testing.T) {
		req := tasks.SvcCreateReq{
			TeamID:      teamID,
			Title:       "Task with outsider assignee",
			Description: "desc",
			Status:      model.StatusTodo,
			AssigneeID:  Val(outsider),
		}
		ctxOwner := auth.ContextWithUserForTest(ctx, model.User{
			ID: owner,
			Roles: model.UserRoles{
				teamID.String(): model.RoleOwner,
			},
		})
		_, err := taskSvc.Create(ctxOwner, req)
		be.Err(t, err, model.ErrForbidden)
	})

	t.Run("outsider cannot create task", func(t *testing.T) {
		req := tasks.SvcCreateReq{
			TeamID:      teamID,
			Title:       "Outsider task",
			Description: "desc",
			Status:      model.StatusTodo,
		}
		ctxOut := auth.ContextWithUserForTest(ctx, model.User{ID: outsider})
		_, err := taskSvc.Create(ctxOut, req)
		be.Err(t, err, model.ErrForbidden)
	})
}
