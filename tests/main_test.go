package tests

import (
	"sync"
	"testing"

	"github.com/redis/go-redis/v9"
)

const TestMainPreffix = "TestMain/"

func TestMain(t *testing.T) {
	var dbc *DBContainer
	var rdb *redis.Client
	var dbcCleanup, rdbCleanup func()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		dbc, dbcCleanup = StartTestDBContainer(t)
	}()
	go func() {
		defer wg.Done()
		rdb, rdbCleanup = StartTestRedis(t)
	}()
	wg.Wait()

	t.Cleanup(func() {
		dbcCleanup()
		rdbCleanup()
	})

	dbc.Run(t, "SQLReport", testSQLReport)
	dbc.Run(t, "TaskAddComment", testTaskAddComment)
	dbc.Run(t, "TaskCreate", testTaskCreate)
	dbc.Run(t, "TaskGet", testTaskGet)
	dbc.Run(t, "TaskListAccess", testTaskListAccess)
	dbc.Run(t, "TaskPagination", testTaskPagination)
	dbc.Run(t, "TaskUpdateExtended", testTaskUpdateExtended)
	dbc.Run(t, "TaskUpdate", testTaskUpdate)
	dbc.Run(t, "TaskUpdateVersionZero", testTaskUpdateVersionZero)
	dbc.Run(t, "TeamCreate", testTeamCreate)
	dbc.Run(t, "TeamList", testTeamList)
	dbc.Run(t, "TeamAddMember", testTeamAddMember)

	t.Run("Cache", func(t *testing.T) {
		t.Parallel()
		db, dbCleanup := dbc.StartTestDatabase(t)
		t.Cleanup(dbCleanup)
		testCache(t, db, rdb)
	})
}
