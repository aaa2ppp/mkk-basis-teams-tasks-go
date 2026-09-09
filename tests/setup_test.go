package tests

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"math/rand"
	"strings"
	"testing"
	"time"

	database "aaa2ppp/teams-tasks/internal/db"
	"aaa2ppp/teams-tasks/internal/model"

	"github.com/aaa2ppp/be"
	_ "github.com/go-sql-driver/mysql"
	"github.com/pressly/goose/v3"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	mariadbImage       = "mariadb:12.3.2-noble"
	redisImage         = "redis:8.10.1-alpine"
	dbContainerName    = "teams-tasks-mariadb-test-container"
	redisContainerName = "teams-tasks-redis-test-container"
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

type DBContainer struct {
	Container testcontainers.Container
	Host      string
	Port      string
	DB        *database.DB
}

func StartTestDBContainer(t *testing.T) (*DBContainer, func()) {
	ctx := context.Background()
	containerLogger := log.New(io.Discard, "", 0)

	req := testcontainers.ContainerRequest{
		Image:        mariadbImage,
		Name:         dbContainerName,
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
		Reuse:            true,
	})
	be.Err(t, err, nil)

	host, err := container.Host(ctx)
	be.Err(t, err, nil)
	port, err := container.MappedPort(ctx, "3306/tcp")
	be.Err(t, err, nil)

	db, err := database.Open(ctx, database.Config{
		Addr:     fmt.Sprintf("%s:%s", host, port.Port()),
		DBName:   "testdb",
		User:     "root",
		Password: "testroot",
	})
	be.Err(t, err, nil)

	cleanup := func() {
		if err := db.Close(); err != nil {
			t.Log(err)
		}
		if err := container.Terminate(ctx); err != nil {
			t.Log(err)
		}
	}

	return &DBContainer{
		Container: container,
		Host:      host,
		Port:      port.Port(),
		DB:        db,
	}, cleanup
}

// ---- Запуск тестовой БД ----

func uniqueDBName(t *testing.T) string {
	base := strings.TrimPrefix(t.Name(), TestMainPreffix)
	base = strings.ReplaceAll(base, "/", "_")
	base = strings.ReplaceAll(base, " ", "_")
	return base + "_" + randomString(8)
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// StartTestDatabase поднимает контейнер MariaDB, применяет миграции и возвращает *sql.DB
// и функцию для остановки/очистки.
func (c *DBContainer) StartTestDatabase(t *testing.T) (*database.DB, func()) {
	ctx := context.Background()

	dbName := uniqueDBName(t)
	_, err := c.DB.ExecContext(ctx, "CREATE DATABASE "+dbName)
	be.Err(t, err, nil)

	db, err := database.Open(ctx, database.Config{
		Addr:     fmt.Sprintf("%s:%s", c.Host, c.Port),
		DBName:   dbName,
		User:     "root",
		Password: "testroot",
	})
	be.Err(t, err, nil)

	// Миграции
	be.Err(t, goose.SetDialect("mysql"), nil)
	be.Err(t, goose.Up(db.DB(), "../migrations"), nil)

	cleanup := func() {
		if err := db.Close(); err != nil {
			t.Log(err)
		}
		if _, err := c.DB.ExecContext(ctx, "DROP DATABASE "+dbName); err != nil {
			t.Log(err)
		}
	}

	return db, cleanup
}

func (c *DBContainer) Run(t *testing.T, name string, fn func(t *testing.T, db *database.DB)) {
	t.Run(name, func(t *testing.T) {
		t.Parallel()
		db, cleanup := c.StartTestDatabase(t)
		t.Cleanup(cleanup)
		fn(t, db)
	})
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

// StartTestRedis поднимает контейнер Redis и возвращает клиент и функцию очистки.
func StartTestRedis(t *testing.T) (*redis.Client, func()) {
	ctx := context.Background()
	containerLogger := log.New(io.Discard, "", 0)

	req := testcontainers.ContainerRequest{
		Image:        redisImage,
		Name:         redisContainerName,
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForLog("Ready to accept connections"),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
		Logger:           containerLogger,
		Reuse:            true,
	})
	be.Err(t, err, nil)

	host, err := container.Host(ctx)
	be.Err(t, err, nil)
	port, err := container.MappedPort(ctx, "6379/tcp")
	be.Err(t, err, nil)

	client := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%s", host, port.Port()),
	})
	err = client.Ping(ctx).Err()
	be.Err(t, err, nil)

	cleanup := func() {
		if err := client.Close(); err != nil {
			t.Log(err)
		}
		if err := container.Terminate(ctx); err != nil {
			t.Log(err)
		}
	}

	return client, cleanup
}
