package teams

import (
	"context"
	"errors"

	"aaa2ppp/teams-tasks/internal/db"
	"aaa2ppp/teams-tasks/internal/lib/auth"
	"aaa2ppp/teams-tasks/internal/model"
)

type DBCreateReq struct {
	Name      string
	CreatedBy model.UserID
}

type DBGetByIDReq struct {
	TeamID    model.TeamID
	CurUserID model.UserID
}

type DBAddMemberReq struct {
	TeamID     model.TeamID
	UserID     model.UserID
	UserEmail  string
	MemberRole model.Role
	CurUserID  model.UserID
}

type DBListReq struct {
	CurUserID model.UserID
}

type DBGetByIDsReq struct {
	TeamIDs []model.TeamID
}

type DBGenReportReq struct {
	TeamID    model.TeamID
	CurUserID model.UserID
}

type Storage interface {
	WithTx(tx db.DBTX) Storage

	Create(ctx context.Context, req DBCreateReq) (model.TeamID, error)
	GetByID(ctx context.Context, req DBGetByIDReq) (model.Team, error)
	AddMember(ctx context.Context, req DBAddMemberReq) error

	// List ДОЛЖЕН возвращать список отсортированный по ID
	List(ctx context.Context, req DBListReq) ([]model.Team, error)
	// GetMembers ДОЛЖЕН возвращать список отсортированный по TeamID
	GetMembers(ctx context.Context, req DBGetByIDsReq) ([]model.TeamMember, error)
	// GetTasks ДОЛЖЕН возвращать список отсортированный по TeamID
	GetTasks(ctx context.Context, req DBGetByIDsReq) ([]model.Task, error)

	GenReport(ctx context.Context, req DBGenReportReq) ([]Metric, error)
}

type Transactor interface {
	InTx(context.Context, func(context.Context, db.DBTX) error) error
}

type service struct {
	storage    Storage
	transactor Transactor
}

var _ Service = &service{}

func NewService(storage Storage, transactor Transactor) *service {
	return &service{storage: storage, transactor: transactor}
}

func (s *service) Create(ctx context.Context, req SvcCreateReq) (model.Team, error) {
	curUser, err := auth.GetCurrentUser(ctx)
	if err != nil {
		return model.Team{}, err
	}

	var team model.Team
	if err := s.transactor.InTx(ctx, func(ctx context.Context, tx db.DBTX) (err error) {
		storage := s.storage.WithTx(tx)
		teamID, err := storage.Create(ctx, DBCreateReq{
			Name:      req.Name,
			CreatedBy: curUser.ID,
		})
		if err != nil {
			return err
		}
		err = storage.AddMember(ctx, DBAddMemberReq{
			TeamID:     teamID,
			UserID:     curUser.ID,
			MemberRole: model.RoleOwner,
		})
		if err != nil {
			return err
		}
		team, err = storage.GetByID(ctx, DBGetByIDReq{
			TeamID:    teamID,
			CurUserID: curUser.ID,
		})
		return err
	}); err != nil {
		return model.Team{}, err
	}

	return team, nil
}

func (s *service) AddMember(ctx context.Context, req SvcAddMemberReq) error {
	if req.MemberRole == model.RoleOwner {
		return model.ErrForbidden
	}

	curUser, err := auth.GetCurrentUser(ctx)
	if err != nil {
		return err
	}

	if role := curUser.Roles[req.TeamID.String()]; role != model.RoleOwner && role != model.RoleAdmin {
		return model.ErrForbidden
	}

	if err = s.storage.AddMember(ctx, DBAddMemberReq{
		TeamID:     req.TeamID,
		UserID:     req.UserID,
		UserEmail:  req.UserEmail,
		MemberRole: req.MemberRole,
		CurUserID:  curUser.ID,
	}); err != nil {
		if errors.Is(err, model.ErrNoRowsAffected) {
			return model.ErrForbidden
		}
		return err
	}

	return nil
}

func (s *service) List(ctx context.Context, req SvcListReq) ([]model.Team, error) {
	curUser, err := auth.GetCurrentUser(ctx)
	if err != nil {
		return nil, err
	}

	teams, err := s.storage.List(ctx, DBListReq{
		CurUserID: curUser.ID,
	})
	if err != nil {
		return nil, err
	}

	if len(teams) == 0 || (!req.WithMembers && !req.WithTasks) {
		return teams, nil
	}

	teamIDs := make([]model.TeamID, len(teams))
	for i := range len(teams) {
		teamIDs[i] = teams[i].ID
	}

	if req.WithMembers {
		members, err := s.storage.GetMembers(ctx, DBGetByIDsReq{
			TeamIDs: teamIDs,
		})
		if err != nil {
			return nil, err
		}
		collectMembers(teams, members)
	}

	if req.WithTasks {
		tasks, err := s.storage.GetTasks(ctx, DBGetByIDsReq{
			TeamIDs: teamIDs,
		})
		if err != nil {
			return nil, err
		}
		collectTasks(teams, tasks)
	}

	return teams, nil
}

func collectMembers(teams []model.Team, members []model.TeamMember) {
	i, l := 0, 0
	for l < len(members) {
		id := members[l].TeamID
		for i < len(teams) && teams[i].ID < id {
			i++
		}
		if i == len(teams) {
			break
		}
		r := l + 1
		for r < len(members) && members[r].TeamID == id {
			r++
		}
		if teams[i].ID == id {
			teams[i].Members = members[l : r : r-l]
			i++
		}
		l = r
	}
}

func collectTasks(teams []model.Team, tasks []model.Task) {
	i, l := 0, 0
	for l < len(tasks) {
		id := tasks[l].TeamID
		for i < len(teams) && teams[i].ID < id {
			i++
		}
		if i == len(teams) {
			break
		}
		r := l + 1
		for r < len(tasks) && tasks[r].TeamID == id {
			r++
		}
		if teams[i].ID == tasks[l].TeamID {
			teams[i].Tasks = tasks[l : r : r-l]
			i++
		}
		l = r
	}
}

func (s *service) GenReport(ctx context.Context, teamID model.TeamID) ([]Metric, error) {
	curUser, err := auth.GetCurrentUser(ctx)
	if err != nil {
		return nil, err
	}
	if role := curUser.Roles[teamID.String()]; role != model.RoleOwner && role != model.RoleAdmin {
		return nil, model.ErrForbidden
	}
	return s.storage.GenReport(ctx, DBGenReportReq{
		TeamID:    teamID,
		CurUserID: curUser.ID,
	})
}
