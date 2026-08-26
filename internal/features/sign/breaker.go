package sign

import (
	"context"

	"aaa2ppp/teams-tasks/internal/db"
)

type breaker struct {
	cb  *db.CircuitBreaker
	svc Service
}

// Login implements [Service].
func (b *breaker) Login(ctx context.Context, req LoginReq) (resp LoginResp, err error) {
	err = b.cb.Execute(func() error {
		resp, err = b.svc.Login(ctx, req)
		return err
	})
	return
}

// Register implements [Service].
func (b *breaker) Register(ctx context.Context, req RegisterReq) (resp LoginResp, err error) {
	err = b.cb.Execute(func() error {
		resp, err = b.svc.Register(ctx, req)
		return err
	})
	return
}

func NewBreaker(cb *db.CircuitBreaker, svc Service) *breaker {
	return &breaker{cb: cb, svc: svc}
}

var _ Service = &breaker{}
