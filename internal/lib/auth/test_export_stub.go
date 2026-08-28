//go:build !test

package auth

import (
	"context"

	"aaa2ppp/teams-tasks/internal/model"
)

var ContextWithUserForTest = func(ctx context.Context, user model.User) context.Context {
	panic("ContextWithUserForTest is only available in test builds (use -tags=test)")
}
