package auth

import (
	"context"

	"aaa2ppp/teams-tasks/internal/model"
)

type userContextKey struct{}

func GetCurrentUser(ctx context.Context) (model.User, error) {
	p := ctx.Value(userContextKey{})
	if user, ok := p.(*model.User); ok {
		return user.Clone(), nil
	}
	return model.User{}, model.ErrUnauthorized
}

func contextWithUser(ctx context.Context, user model.User) context.Context {
	return context.WithValue(ctx, userContextKey{}, &user)
}
