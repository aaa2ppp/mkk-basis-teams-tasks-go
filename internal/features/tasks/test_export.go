//go:build test

package tasks

import (
	"context"

	"aaa2ppp/teams-tasks/internal/model"
)

// InvalidateTeamCacheForTest
//
// **FOR TEST ONY**
//
// Позволяет тестам принудительно очистить кеш для команды. Доступно только при использовании
// тега сборки `-tags=test`, иначе вызывает панику.
func InvalidateTeamCacheForTest(svc *service, ctx context.Context, teamID model.TeamID) error {
	return svc.invalidateCache(ctx, teamID)
}
