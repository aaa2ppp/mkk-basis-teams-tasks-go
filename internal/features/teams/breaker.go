package teams

import (
	"context"

	"aaa2ppp/teams-tasks/internal/db"
	"aaa2ppp/teams-tasks/internal/model"
)

type breaker struct {
	cb  *db.CircuitBreaker
	svc Service
}

// AddMember implements [Service].
func (b *breaker) AddMember(ctx context.Context, req SvcAddMemberReq) (err error) {
	return b.cb.Execute(func() error {
		return b.svc.AddMember(ctx, req)
	})
}

// Create implements [Service].
func (b *breaker) Create(ctx context.Context, req SvcCreateReq) (resp model.Team, err error) {
	err = b.cb.Execute(func() error {
		resp, err = b.svc.Create(ctx, req)
		return err
	})
	return
}

// GenReport implements [Service].
func (b *breaker) GenReport(ctx context.Context, teamID model.TeamID) (resp []Metric, err error) {
	err = b.cb.Execute(func() error {
		resp, err = b.svc.GenReport(ctx, teamID)
		return err
	})
	return
}

// List implements [Service].
func (b *breaker) List(ctx context.Context, req SvcListReq) (resp []model.Team, err error) {
	err = b.cb.Execute(func() error {
		resp, err = b.svc.List(ctx, req)
		return err
	})
	return
}

func NewBreaker(cb *db.CircuitBreaker, svc Service) *breaker {
	return &breaker{cb: cb, svc: svc}
}

var _ Service = &breaker{}
