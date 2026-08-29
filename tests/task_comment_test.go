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

// TODO: не все роли покрыты. В контексте задачи есть:
// создатель/админ команды,
// член команды создатель/испольнитель/"не учасстник и не исполнител",
// не член команды

// TestTaskAddComment проверяет, что комментарии могут добавлять только участники команды.
func TestTaskAddComment(t *testing.T) {
	ctx := context.Background()
	db, cleanup := StartTestDatabase(t)
	defer cleanup()

	taskStorage := tasks.NewStorage(db)
	cache := &NoopCache{}
	taskSvc := tasks.NewService(taskStorage, db, cache)

	owner := InsertUser(t, db, "owner@test.com", "Owner", "hash")
	assignee := InsertUser(t, db, "assignee@test.com", "Assignee", "hash")
	member := InsertUser(t, db, "member@test.com", "Member", "hash")
	outsider := InsertUser(t, db, "outsider@test.com", "Outsider", "hash")

	teamID := InsertTeam(t, db, "Test Team", owner)
	AddMember(t, db, teamID, owner, model.RoleOwner)
	AddMember(t, db, teamID, assignee, model.RoleMember)
	AddMember(t, db, teamID, member, model.RoleMember)

	now := time.Now()
	taskID := CreateTask(t, db, teamID, "Task", "desc", model.StatusTodo, owner, &assignee, now, nil)

	t.Run("assignee add comment", func(t *testing.T) {
		req := tasks.SvcAddCommentReq{
			TaskID:  taskID,
			Content: "Assignee comment",
		}
		ctxAssignee := auth.ContextWithUserForTest(ctx, model.User{
			ID: assignee,
			Roles: model.UserRoles{
				teamID.String(): model.RoleMember,
			},
		})
		comment, err := taskSvc.AddComment(ctxAssignee, req)
		be.Err(t, err, nil)
		be.Equal(t, comment.TaskID, taskID)
		be.Equal(t, comment.UserID, assignee)
		be.Equal(t, comment.Content, "Assignee comment")
		be.True(t, !comment.CreatedAt.IsZero())
	})

	t.Run("owner can add comment", func(t *testing.T) {
		req := tasks.SvcAddCommentReq{
			TaskID:  taskID,
			Content: "Owner comment",
		}
		ctxOwner := auth.ContextWithUserForTest(ctx, model.User{
			ID: owner,
			Roles: model.UserRoles{
				teamID.String(): model.RoleOwner,
			},
		})
		comment, err := taskSvc.AddComment(ctxOwner, req)
		be.Err(t, err, nil)
		be.Equal(t, comment.UserID, owner)
	})

	t.Run("member can canot add comment", func(t *testing.T) {
		req := tasks.SvcAddCommentReq{
			TaskID:  taskID,
			Content: "Member comment",
		}
		ctxMember := auth.ContextWithUserForTest(ctx, model.User{
			ID: member,
			Roles: model.UserRoles{
				teamID.String(): model.RoleMember,
			},
		})
		_, err := taskSvc.AddComment(ctxMember, req)
		be.Err(t, err, model.ErrForbidden)
	})

	t.Run("outsider cannot add comment", func(t *testing.T) {
		req := tasks.SvcAddCommentReq{
			TaskID:  taskID,
			Content: "Outsider comment",
		}
		ctxOut := auth.ContextWithUserForTest(ctx, model.User{ID: outsider})
		_, err := taskSvc.AddComment(ctxOut, req)
		be.Err(t, err, model.ErrForbidden)
	})

	// Проверяем, что комментарий сохранился и доступен через Get
	t.Run("comment is persisted", func(t *testing.T) {
		getReq := tasks.SvcGetReq{TaskID: taskID, WithComments: true}
		ctxOwner := auth.ContextWithUserForTest(ctx, model.User{ID: owner})
		task, err := taskSvc.Get(ctxOwner, getReq)
		be.Err(t, err, nil)
		be.Equal(t, len(task.Comments), 2) // два добавленных комментария
	})
}
