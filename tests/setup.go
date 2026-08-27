//go:build test

package tests

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"testing"
	"time"

	database "aaa2ppp/teams-tasks/internal/db"
	"aaa2ppp/teams-tasks/internal/lib/auth"
	"aaa2ppp/teams-tasks/internal/model"

	"github.com/aaa2ppp/be"
	_ "github.com/go-sql-driver/mysql"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// ---- Конструкторы для Nullable ----

func Val[T any](v T) model.Nullable[T] {
	return model.Nullable[T]{
		Null:    sql.Null[T]{Valid: true, V: v},
		Defined: true,
	}
}

func Null[T any]() model.Nullable[T] {
	return model.Nullable[T]{
		Defined: true,
	}
}

func Undef[T any]() model.Nullable[T] {
	return model.Nullable[T]{}
}

// ---- Запуск тестовой БД ----

// StartTestDatabase поднимает контейнер MariaDB, применяет миграции и возвращает *sql.DB
// и функцию для остановки/очистки.
func StartTestDatabase(t *testing.T) (*database.DB, func()) {
	ctx := context.Background()
	containerLogger := log.New(io.Discard, "", 0)

	req := testcontainers.ContainerRequest{
		Image:        "mariadb:12.3.2-noble",
		ExposedPorts: []string{"3306/tcp"},
		Env: map[string]string{
			"MYSQL_ROOT_PASSWORD": "testroot",
			"MYSQL_DATABASE":      "testdb",
			"MYSQL_USER":          "testuser",
			"MYSQL_PASSWORD":      "testpass",
		},
		WaitingFor: wait.ForLog("ready for connections").WithOccurrence(2),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
		Logger:           containerLogger,
	})
	be.Err(t, err, nil)

	host, err := container.Host(ctx)
	be.Err(t, err, nil)
	port, err := container.MappedPort(ctx, "3306/tcp")
	be.Err(t, err, nil)

	db, err := database.Open(ctx, database.Config{
		Addr:     fmt.Sprintf("%s:%s", host, port.Port()),
		DBName:   "testdb",
		User:     "testuser",
		Password: "testpass",
	})
	be.Err(t, err, nil)

	// Миграции
	be.Err(t, goose.SetDialect("mysql"), nil)
	be.Err(t, goose.Up(db.DB(), "../migrations"), nil)

	cleanup := func() {
		db.Close()
		container.Terminate(ctx)
	}

	return db, cleanup
}

// ---- Вспомогательные функции для вставки данных ----

func InsertUser(t *testing.T, db *database.DB, email, name, pass string) model.UserID {
	t.Helper()
	ctx := context.Background()
	res, err := db.ExecContext(ctx,
		"INSERT INTO users (email, name, password_hash) VALUES (?, ?, ?)",
		email, name, pass)
	be.Err(t, err, nil)
	id, _ := res.LastInsertId()
	return model.UserID(id)
}

func InsertTeam(t *testing.T, db *database.DB, name string, createdBy model.UserID) model.TeamID {
	t.Helper()
	ctx := context.Background()
	res, err := db.ExecContext(ctx,
		"INSERT INTO teams (name, created_by) VALUES (?, ?)",
		name, createdBy)
	be.Err(t, err, nil)
	id, _ := res.LastInsertId()
	return model.TeamID(id)
}

func AddMember(t *testing.T, db *database.DB, teamID model.TeamID, userID model.UserID, role model.Role) {
	t.Helper()
	ctx := context.Background()
	_, err := db.ExecContext(ctx,
		"INSERT INTO team_members (team_id, user_id, role) VALUES (?, ?, ?)",
		teamID, userID, role.String())
	be.Err(t, err, nil)
}

func CreateTask(t *testing.T, db *database.DB, teamID model.TeamID, title, desc string,
	status model.Status, createdBy model.UserID, assignee *model.UserID, createdAt time.Time, closedAt *time.Time) model.TaskID {
	t.Helper()
	ctx := context.Background()
	res, err := db.ExecContext(ctx,
		`INSERT INTO tasks (team_id, title, description, status, created_by, assignee_id, created_at, closed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		teamID, title, desc, status, createdBy, assignee, createdAt, closedAt)
	be.Err(t, err, nil)
	id, _ := res.LastInsertId()
	return model.TaskID(id)
}

func AddComment(t *testing.T, db *database.DB, taskID model.TaskID, userID model.UserID,
	content string, createdAt *time.Time) {
	t.Helper()
	ctx := context.Background()
	_, err := db.ExecContext(ctx,
		"INSERT INTO task_comments (task_id, user_id, content, created_at) VALUES (?, ?, ?, ?)",
		taskID, userID, content, createdAt)
	be.Err(t, err, nil)
}

// ---- Заглушка для кеша ----

type NoopCache struct{}

func (c *NoopCache) Get(ctx context.Context, key, field string, val any) error {
	return model.ErrNotFound
}
func (c *NoopCache) Put(ctx context.Context, key, field string, val any) error {
	return nil
}
func (c *NoopCache) Del(ctx context.Context, key string) error {
	return nil
}

// ---- Управление контекстом ----

var contextWithUser = auth.ContextWithUserForTest
