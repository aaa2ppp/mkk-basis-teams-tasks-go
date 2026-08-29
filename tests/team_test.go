package tests

import (
	"context"
	"testing"

	"aaa2ppp/teams-tasks/internal/features/teams"
	"aaa2ppp/teams-tasks/internal/lib/auth"
	"aaa2ppp/teams-tasks/internal/model"

	"github.com/aaa2ppp/be"
	_ "github.com/go-sql-driver/mysql"
)

// TestTeamCreate проверяет создание команды и автоматическое назначение создателя владельцем.
func TestTeamCreate(t *testing.T) {
	ctx := context.Background()
	db, cleanup := StartTestDatabase(t)
	defer cleanup()

	storage := teams.NewStorage(db)
	svc := teams.NewService(storage, db)

	user := InsertUser(t, db, "creator@test.com", "Creator", "hash")
	userCtx := auth.ContextWithUserForTest(ctx, model.User{ID: user})

	req := teams.SvcCreateReq{Name: "New Team"}
	team, err := svc.Create(userCtx, req)
	be.Err(t, err, nil)

	be.Equal(t, team.Name, "New Team")
	be.Equal(t, team.CreatedBy, user)

	// Проверяем, что создатель добавлен как участник с ролью owner
	members, err := storage.GetMembers(ctx, teams.DBGetByIDsReq{TeamIDs: []model.TeamID{team.ID}})
	be.Err(t, err, nil)
	be.Equal(t, len(members), 1)
	be.Equal(t, members[0].User.ID, user)
	be.Equal(t, members[0].Role, model.RoleOwner)
}

// TestTeamList проверяет, что пользователь видит только свои команды.
func TestTeamList(t *testing.T) {
	ctx := context.Background()
	db, cleanup := StartTestDatabase(t)
	defer cleanup()

	storage := teams.NewStorage(db)
	svc := teams.NewService(storage, db)

	user1 := InsertUser(t, db, "user1@test.com", "User1", "hash")
	user2 := InsertUser(t, db, "user2@test.com", "User2", "hash")

	// Команда 1 – создана user1, user2 – участник
	team1 := InsertTeam(t, db, "Team1", user1)
	AddMember(t, db, team1, user1, model.RoleOwner)
	AddMember(t, db, team1, user2, model.RoleMember)

	// Команда 2 – создана user2, только user2
	team2 := InsertTeam(t, db, "Team2", user2)
	AddMember(t, db, team2, user2, model.RoleOwner)

	// Команда 3 – создана user1, только user1
	team3 := InsertTeam(t, db, "Team3", user1)
	AddMember(t, db, team3, user1, model.RoleOwner)

	// user1 должен видеть team1 и team3
	ctx1 := auth.ContextWithUserForTest(ctx, model.User{ID: user1})
	list, err := svc.List(ctx1, teams.SvcListReq{})
	be.Err(t, err, nil)
	be.Equal(t, len(list), 2)
	ids := map[model.TeamID]bool{team1: true, team3: true}
	for _, team := range list {
		be.True(t, ids[team.ID])
	}

	// user2 должен видеть team1 и team2
	ctx2 := auth.ContextWithUserForTest(ctx, model.User{ID: user2})
	list, err = svc.List(ctx2, teams.SvcListReq{})
	be.Err(t, err, nil)
	be.Equal(t, len(list), 2)
	ids = map[model.TeamID]bool{team1: true, team2: true}
	for _, team := range list {
		be.True(t, ids[team.ID])
	}
}

// TestTeamAddMember проверяет права на добавление участников и запрет на выдачу роли owner.
func TestTeamAddMember(t *testing.T) {
	ctx := context.Background()
	db, cleanup := StartTestDatabase(t)
	defer cleanup()

	storage := teams.NewStorage(db)
	svc := teams.NewService(storage, db)

	owner := InsertUser(t, db, "owner@test.com", "Owner", "hash")
	admin := InsertUser(t, db, "admin@test.com", "Admin", "hash")
	member := InsertUser(t, db, "member@test.com", "Member", "hash")
	outsider := InsertUser(t, db, "outsider@test.com", "Outsider", "hash")
	newUser1 := InsertUser(t, db, "new1@test.com", "New1", "hash")
	newUser2 := InsertUser(t, db, "new2@test.com", "New1", "hash")

	teamID := InsertTeam(t, db, "Test Team", owner)
	AddMember(t, db, teamID, owner, model.RoleOwner)
	AddMember(t, db, teamID, admin, model.RoleAdmin)
	AddMember(t, db, teamID, member, model.RoleMember)

	tests := []struct {
		name      string
		actor     model.User
		target    model.UserID
		role      model.Role
		wantError error
	}{
		{
			name:   "owner can add member",
			actor:  model.User{ID: owner, Roles: model.UserRoles{teamID.String(): model.RoleOwner}},
			target: newUser1,
			role:   model.RoleMember,
		},
		{
			name:   "admin can add member",
			actor:  model.User{ID: admin, Roles: model.UserRoles{teamID.String(): model.RoleAdmin}},
			target: newUser2,
			role:   model.RoleMember,
		},
		{
			name:      "member cannot add",
			actor:     model.User{ID: member, Roles: model.UserRoles{teamID.String(): model.RoleMember}},
			target:    newUser1,
			role:      model.RoleMember,
			wantError: model.ErrForbidden,
		},
		{
			name:      "outsider cannot add",
			actor:     model.User{ID: outsider},
			target:    newUser1,
			role:      model.RoleMember,
			wantError: model.ErrForbidden,
		},
		{
			name:      "cannot add existing member",
			actor:     model.User{ID: owner, Roles: model.UserRoles{teamID.String(): model.RoleOwner}},
			target:    admin,
			role:      model.RoleMember,
			wantError: model.ErrConflict, // или другая ошибка
		},
		{
			name:      "cannot assign owner role via AddMember",
			actor:     model.User{ID: owner, Roles: model.UserRoles{teamID.String(): model.RoleOwner}},
			target:    newUser1,
			role:      model.RoleOwner,
			wantError: model.ErrForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := teams.SvcAddMemberReq{
				TeamID:     teamID,
				UserID:     tt.target,
				MemberRole: tt.role,
			}
			actorCtx := auth.ContextWithUserForTest(ctx, tt.actor)
			err := svc.AddMember(actorCtx, req)
			if tt.wantError != nil {
				be.Err(t, err, tt.wantError)
				return
			}
			be.Err(t, err, nil)
			// Проверяем, что пользователь действительно добавлен
			members, err := storage.GetMembers(ctx, teams.DBGetByIDsReq{TeamIDs: []model.TeamID{teamID}})
			be.Err(t, err, nil)
			found := false
			for _, m := range members {
				if m.User.ID == tt.target {
					found = true
					be.Equal(t, m.Role, tt.role)
					break
				}
			}
			be.True(t, found)
		})
	}
}
