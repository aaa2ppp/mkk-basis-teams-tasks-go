package tasks

import (
	"aaa2ppp/teams-tasks/internal/db"
	"aaa2ppp/teams-tasks/internal/model"
	"context"
)

type breaker struct {
	cb  *db.CircuitBreaker
	svc Service
}

// AddComment implements [Service].
func (b *breaker) AddComment(ctx context.Context, req SvcAddCommentReq) (resp model.TaskComment, err error) {
	err = b.cb.Execute(func() error {
		resp, err = b.svc.AddComment(ctx, req)
		return err
	})
	return
}

// Create implements [Service].
func (b *breaker) Create(ctx context.Context, req SvcCreateReq) (resp model.Task, err error) {
	err = b.cb.Execute(func() error {
		resp, err = b.svc.Create(ctx, req)
		return err
	})
	return
}

// Get implements [Service].
func (b *breaker) Get(ctx context.Context, req SvcGetReq) (resp model.Task, err error) {
	err = b.cb.Execute(func() error {
		resp, err = b.svc.Get(ctx, req)
		return err
	})
	return
}

// List implements [Service].
func (b *breaker) List(ctx context.Context, req SvcListReq) (resp []model.Task, err error) {
	err = b.cb.Execute(func() error {
		resp, err = b.svc.List(ctx, req)
		return err
	})
	return
}

// Update implements [Service].
func (b *breaker) Update(ctx context.Context, req SvcUpdateReq) (resp model.Task, err error) {
	err = b.cb.Execute(func() error {
		resp, err = b.svc.Update(ctx, req)
		return err
	})
	return
}

func NewBreaker(cb *db.CircuitBreaker, svc Service) *breaker {
	return &breaker{cb: cb, svc: svc}
}

var _ Service = &breaker{}
